// Package tlstrust centralises TLS trust for Terranova's outbound clients
// (agent -> API, agent C&C WebSocket, Terraform HTTP state backend, run task
// and other server-side HTTP clients).
//
// There is deliberately no way to disable certificate verification. Options
// commonly used for that (TLS_INSECURE, SKIP_TLS_VERIFY, GIT_SSL_NO_VERIFY,
// ...) are detected at startup: in production (ENV=production, the same
// switch as the key checks) the process refuses to start; in development it
// starts with a loud warning naming each option. Private CAs are supported
// through IAC_CA_FILE (alias AGENT_CA_FILE): a PEM bundle added to the system
// roots.
package tlstrust

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gorilla/websocket"

	"iac-platform/internal/keys"
)

// CA bundle environment variables (both may be set; all files are added).
const (
	EnvCAFile      = "IAC_CA_FILE"
	EnvAgentCAFile = "AGENT_CA_FILE"
)

type bypassOption struct {
	name    string
	enabled func(value string) bool
	effect  string
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "y", "t":
		return true
	}
	return false
}

func present(string) bool { return true }

func equals(want string) func(string) bool {
	return func(v string) bool { return strings.TrimSpace(v) == want }
}

const notHonored = "not honored by Terranova (there is no verification bypass); use " + EnvCAFile + " for private CAs"

// bypassOptions are environment variables that request skipping TLS
// verification, either in Terranova's own naming space or honored by tools
// the server/agent spawns (git, node, python).
var bypassOptions = []bypassOption{
	{"IAC_TLS_INSECURE_SKIP_VERIFY", truthy, notHonored},
	{"IAC_INSECURE_SKIP_VERIFY", truthy, notHonored},
	{"IAC_AGENT_TLS_INSECURE", truthy, notHonored},
	{"AGENT_TLS_INSECURE", truthy, notHonored},
	{"AGENT_INSECURE_SKIP_VERIFY", truthy, notHonored},
	{"TLS_INSECURE", truthy, notHonored},
	{"TLS_INSECURE_SKIP_VERIFY", truthy, notHonored},
	{"TLS_SKIP_VERIFY", truthy, notHonored},
	{"INSECURE_SKIP_VERIFY", truthy, notHonored},
	{"SKIP_TLS_VERIFY", truthy, notHonored},
	// Honored by child processes (git fetches Terraform modules, etc.).
	{"GIT_SSL_NO_VERIFY", present, "git skips certificate verification for every fetch (Terraform module sources)"},
	{"NODE_TLS_REJECT_UNAUTHORIZED", equals("0"), "Node.js child processes skip certificate verification"},
	{"PYTHONHTTPSVERIFY", equals("0"), "Python child processes skip certificate verification"},
}

// EnabledBypassOptions returns the names of bypass options set in the
// environment, sorted.
func EnabledBypassOptions() []string {
	var out []string
	for _, o := range bypassOptions {
		if v, ok := os.LookupEnv(o.name); ok && o.enabled(v) {
			out = append(out, o.name)
		}
	}
	sort.Strings(out)
	return out
}

func effectOf(name string) string {
	for _, o := range bypassOptions {
		if o.name == name {
			return o.effect
		}
	}
	return ""
}

// CheckStartup validates the TLS trust configuration for component
// ("server", "agent"). Any enabled bypass option is an error in production
// and a warning otherwise. An unreadable or invalid CA bundle is always an
// error.
func CheckStartup(component string) (warnings []string, err error) {
	var errs []error
	for _, name := range EnabledBypassOptions() {
		if keys.IsProduction() {
			errs = append(errs, fmt.Errorf("%s: TLS verification bypass option %s is set; refused in production (ENV=production), use %s for private CAs", component, name, EnvCAFile))
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"!!! %s: TLS verification bypass option %s is set (%s). Allowed only because ENV is not production; the %s will refuse to start with it in production !!!",
			component, name, effectOf(name), component))
	}
	if _, e := loadCAFiles(); e != nil {
		errs = append(errs, e)
	}
	return warnings, errors.Join(errs...)
}

func caFiles() []string {
	var files []string
	seen := map[string]bool{}
	for _, env := range []string{EnvCAFile, EnvAgentCAFile} {
		if f := strings.TrimSpace(os.Getenv(env)); f != "" && !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	return files
}

// loadCAFiles reads and validates the configured CA bundles; nil when none.
func loadCAFiles() ([][]byte, error) {
	var out [][]byte
	for _, f := range caFiles() {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: cannot read CA bundle: %w", EnvCAFile, EnvAgentCAFile, err)
		}
		if err := validateBundle(data); err != nil {
			return nil, fmt.Errorf("CA bundle %s: %w", f, err)
		}
		out = append(out, data)
	}
	return out, nil
}

func validateBundle(data []byte) error {
	n := 0
	for rest := data; ; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if strings.Contains(b.Type, "PRIVATE KEY") {
			return errors.New("contains a private key; a CA bundle must only contain certificates")
		}
		if b.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(b.Bytes); err != nil {
			return fmt.Errorf("invalid certificate: %w", err)
		}
		n++
	}
	if n == 0 {
		return errors.New("no PEM certificates found")
	}
	return nil
}

// RootCAs returns the system roots plus the configured CA bundles, or nil
// (meaning: use the system roots) when no CA bundle is configured.
func RootCAs() (*x509.CertPool, error) {
	bundles, err := loadCAFiles()
	if err != nil || len(bundles) == 0 {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	for _, b := range bundles {
		pool.AppendCertsFromPEM(b)
	}
	return pool, nil
}

// ClientConfig returns the TLS client configuration for internal clients:
// TLS 1.2+, verification always on, system roots plus IAC_CA_FILE.
func ClientConfig() (*tls.Config, error) {
	roots, err := RootCAs()
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, nil
}

// InstallDefaults applies ClientConfig to http.DefaultTransport (used by
// http.DefaultClient and every http.Client without its own Transport) and
// websocket.DefaultDialer. Call once at startup, after CheckStartup.
func InstallDefaults() error {
	cfg, err := ClientConfig()
	if err != nil {
		return err
	}
	if t, ok := http.DefaultTransport.(*http.Transport); ok {
		t.TLSClientConfig = cfg.Clone()
	}
	websocket.DefaultDialer.TLSClientConfig = cfg.Clone()
	return nil
}

// systemBundleFiles are the usual system CA bundle locations (Debian/Alpine,
// RHEL/Amazon Linux, openSUSE, macOS/Homebrew openssl).
var systemBundleFiles = []string{
	"/etc/ssl/certs/ca-certificates.crt",
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/ssl/ca-bundle.pem",
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
	"/etc/ssl/cert.pem",
}

// CABundlePEM returns a PEM bundle of the system roots plus the configured
// CA bundles, for child processes that take PEM content (Terraform's
// TF_HTTP_CLIENT_CA_CERTIFICATE_PEM replaces the system roots, so they are
// included). Empty when no CA bundle is configured.
func CABundlePEM() (string, error) {
	bundles, err := loadCAFiles()
	if err != nil || len(bundles) == 0 {
		return "", err
	}
	var b strings.Builder
	for _, f := range systemBundleFiles {
		if data, err := os.ReadFile(f); err == nil && validateBundle(data) == nil {
			b.Write(data)
			if !strings.HasSuffix(string(data), "\n") {
				b.WriteByte('\n')
			}
			break
		}
	}
	for _, data := range bundles {
		b.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}
