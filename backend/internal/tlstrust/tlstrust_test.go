package tlstrust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// clean unsets every option this package reads (restored after the test).
func clean(t *testing.T) {
	t.Helper()
	names := []string{"ENV", EnvCAFile, EnvAgentCAFile}
	for _, o := range bypassOptions {
		names = append(names, o.name)
	}
	for _, n := range names {
		t.Setenv(n, "")
		os.Unsetenv(n)
	}
}

func writeFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// serverCAFile writes the httptest server's (runtime-generated) certificate
// as a CA bundle.
func serverCAFile(t *testing.T, srv *httptest.Server) string {
	return writeFile(t, "ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
}

func TestNoOptionsNoWarnings(t *testing.T) {
	clean(t)
	t.Setenv("ENV", "production")
	w, err := CheckStartup("agent")
	if err != nil || len(w) != 0 {
		t.Fatalf("clean production config: warnings=%v err=%v", w, err)
	}
}

func TestBypassOptionsRefusedInProduction(t *testing.T) {
	for _, o := range bypassOptions {
		t.Run(o.name, func(t *testing.T) {
			clean(t)
			t.Setenv("ENV", "production")
			v := "true"
			if o.name == "NODE_TLS_REJECT_UNAUTHORIZED" || o.name == "PYTHONHTTPSVERIFY" {
				v = "0"
			}
			t.Setenv(o.name, v)
			_, err := CheckStartup("agent")
			if err == nil || !strings.Contains(err.Error(), o.name) || !strings.Contains(err.Error(), "refused in production") {
				t.Fatalf("production with %s=%s: err=%v", o.name, v, err)
			}
		})
	}
}

func TestBypassOptionsWarnInDevelopment(t *testing.T) {
	clean(t)
	t.Setenv("TLS_INSECURE", "1")
	t.Setenv("GIT_SSL_NO_VERIFY", "") // git honors mere presence
	w, err := CheckStartup("agent")
	if err != nil {
		t.Fatalf("development must start: %v", err)
	}
	joined := strings.Join(w, "\n")
	for _, name := range []string{"TLS_INSECURE", "GIT_SSL_NO_VERIFY"} {
		if !strings.Contains(joined, name) {
			t.Fatalf("warning does not name %s: %q", name, joined)
		}
	}
	if len(w) != 2 || !strings.Contains(joined, "refuse to start with it in production") {
		t.Fatalf("warnings: %q", joined)
	}
}

func TestDisabledValuesAreNotBypass(t *testing.T) {
	clean(t)
	t.Setenv("ENV", "production")
	t.Setenv("TLS_INSECURE", "false")
	t.Setenv("SKIP_TLS_VERIFY", "0")
	t.Setenv("NODE_TLS_REJECT_UNAUTHORIZED", "1")
	t.Setenv("PYTHONHTTPSVERIFY", "1")
	if got := EnabledBypassOptions(); len(got) != 0 {
		t.Fatalf("enabled: %v", got)
	}
	if _, err := CheckStartup("server"); err != nil {
		t.Fatal(err)
	}
}

func TestCAFileTrustsPrivateCA(t *testing.T) {
	clean(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()

	get := func() error {
		cfg, err := ClientConfig()
		if err != nil {
			return err
		}
		if cfg.InsecureSkipVerify || cfg.MinVersion < tls.VersionTLS12 {
			t.Fatalf("unsafe client config: %+v", cfg)
		}
		resp, err := (&http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}).Get(srv.URL)
		if err == nil {
			resp.Body.Close()
		}
		return err
	}
	if err := get(); err == nil {
		t.Fatal("untrusted server certificate accepted without a CA bundle")
	}
	for _, env := range []string{EnvCAFile, EnvAgentCAFile} {
		clean(t)
		t.Setenv(env, serverCAFile(t, srv))
		if _, err := CheckStartup("agent"); err != nil {
			t.Fatalf("%s: %v", env, err)
		}
		if err := get(); err != nil {
			t.Fatalf("%s: private CA not trusted: %v", env, err)
		}
	}
}

func TestInstallDefaults(t *testing.T) {
	clean(t)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	dt := http.DefaultTransport.(*http.Transport)
	prevHTTP, prevWS := dt.TLSClientConfig, websocket.DefaultDialer.TLSClientConfig
	t.Cleanup(func() {
		dt.TLSClientConfig, websocket.DefaultDialer.TLSClientConfig = prevHTTP, prevWS
		dt.CloseIdleConnections()
	})

	t.Setenv(EnvCAFile, serverCAFile(t, srv))
	if err := InstallDefaults(); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("default client does not trust IAC_CA_FILE: %v", err)
	}
	resp.Body.Close()
	if websocket.DefaultDialer.TLSClientConfig == nil || websocket.DefaultDialer.TLSClientConfig.RootCAs == nil ||
		websocket.DefaultDialer.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("websocket default dialer not configured")
	}
}

func TestInvalidCABundleAlwaysRefused(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	cases := map[string]string{
		"missing":     filepath.Join(t.TempDir(), "nope.pem"),
		"empty":       writeFile(t, "empty.pem", nil),
		"not pem":     writeFile(t, "junk.pem", []byte("hello")),
		"bad cert":    writeFile(t, "bad.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("x")})),
		"private key": writeFile(t, "withkey.pem", append(append([]byte{}, certPEM...), keyPEM...)),
	}
	for name, path := range cases {
		for _, env := range []string{"", "production"} {
			clean(t)
			t.Setenv("ENV", env)
			t.Setenv(EnvCAFile, path)
			if _, err := CheckStartup("agent"); err == nil {
				t.Fatalf("%s (ENV=%q): accepted", name, env)
			}
			if _, err := ClientConfig(); err == nil {
				t.Fatalf("%s: ClientConfig accepted", name)
			}
		}
	}
}

func TestCABundlePEM(t *testing.T) {
	clean(t)
	if b, err := CABundlePEM(); err != nil || b != "" {
		t.Fatalf("no CA configured: %q %v", b, err)
	}
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	f := serverCAFile(t, srv)
	t.Setenv(EnvCAFile, f)
	b, err := CABundlePEM()
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := os.ReadFile(f)
	if !strings.Contains(b, string(ca)) {
		t.Fatal("bundle lacks the configured CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(b)) {
		t.Fatal("bundle is not valid PEM")
	}
}

// TestNoHardcodedInsecureSkipVerify guards the whole backend module: no
// non-test source may disable certificate verification.
func TestNoHardcodedInsecureSkipVerify(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	re := regexp.MustCompile(`InsecureSkipVerify\s*[:=]\s*[^f\s/]`)
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			if re.MatchString(line) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", p, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) > 0 {
		t.Fatalf("TLS verification disabled in source (use IAC_CA_FILE for private CAs):\n%s", strings.Join(hits, "\n"))
	}
}
