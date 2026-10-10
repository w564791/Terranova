package gitsource

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type staticCreds struct {
	creds  AppCredentials
	secret []byte
}

func (s staticCreds) AppCredentials() (AppCredentials, error) {
	if s.creds.PrivateKey == nil {
		return AppCredentials{}, ErrDisabled
	}
	return s.creds, nil
}
func (s staticCreds) WebhookSecret() ([]byte, error) {
	if len(s.secret) == 0 {
		return nil, ErrWebhookDisabled
	}
	return s.secret, nil
}

type fakeAPI struct {
	*httptest.Server
	mu          sync.Mutex
	key         *rsa.PrivateKey
	grant       map[string]string
	grantRepos  []string
	revoked     []string
	mintBodies  []map[string]any
	authHeaders []string
}

func newFakeAPI(t *testing.T, key *rsa.PrivateKey) *fakeAPI {
	t.Helper()
	f := &fakeAPI{key: key, grant: map[string]string{"contents": "read", "metadata": "read"}, grantRepos: []string{"acme/infra"}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		auth := r.Header.Get("Authorization")
		f.authHeaders = append(f.authHeaders, auth)
		appAuth := func() bool {
			tok, err := jwt.Parse(strings.TrimPrefix(auth, "Bearer "), func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
				jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer("1234"))
			return err == nil && tok.Valid
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/42/access_tokens":
			if !appAuth() {
				http.Error(w, "bad app jwt", http.StatusUnauthorized)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mintBodies = append(f.mintBodies, body)
			var repos []map[string]string
			for _, n := range f.grantRepos {
				repos = append(repos, map[string]string{"full_name": n})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"token": testToken, "expires_at": time.Now().Add(time.Hour), "permissions": f.grant, "repositories": repos})
		case r.Method == http.MethodGet && r.URL.Path == "/app/installations/42":
			if !appAuth() {
				http.Error(w, "bad app jwt", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "account": map[string]string{"login": "acme", "type": "Organization"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/installation/token":
			f.revoked = append(f.revoked, auth)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/infra/branches":
			if auth != "Bearer "+testToken {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`[{"name":"main","commit":{"sha":"` + strings.Repeat("a", 40) + `"}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/infra/commits":
			if auth != "Bearer "+testToken || r.URL.Query().Get("sha") != "main" {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`[{"sha":"` + strings.Repeat("b", 40) + `","commit":{"message":"fix: thing\n\nbody","author":{"name":"Ann","date":"2026-10-01T00:00:00Z"}}}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func testApp(t *testing.T) (*GitHubApp, *fakeAPI) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	api := newFakeAPI(t, key)
	return &GitHubApp{Creds: staticCreds{creds: AppCredentials{AppID: 1234, PrivateKey: key}}, Endpoints: Endpoints{WebURL: "https://github.com", APIURL: api.URL}}, api
}

func TestGitHubApp_MintScopedToken(t *testing.T) {
	app, api := testApp(t)
	tok, err := app.MintRepoToken(context.Background(), 42, Repo{Owner: "acme", Name: "infra"})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Secret() != testToken || time.Until(tok.ExpiresAt) < 50*time.Minute {
		t.Fatalf("token %v expires %v", tok, tok.ExpiresAt)
	}
	body := api.mintBodies[0]
	if b, _ := json.Marshal(body); string(b) != `{"permissions":{"contents":"read"},"repositories":["infra"]}` {
		t.Fatalf("mint body = %s", b)
	}

	branches, err := app.ListBranches(context.Background(), Repo{Owner: "acme", Name: "infra"}, tok)
	if err != nil || len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("branches = %v %v", branches, err)
	}
	commits, err := app.ListCommits(context.Background(), Repo{Owner: "acme", Name: "infra"}, "main", 10, tok)
	if err != nil || len(commits) != 1 || commits[0].Subject != "fix: thing" || commits[0].AuthorName != "Ann" {
		t.Fatalf("commits = %v %v", commits, err)
	}
	app.Revoke(context.Background(), tok)
	if len(api.revoked) != 1 || api.revoked[0] != "Bearer "+testToken {
		t.Fatalf("revoked = %v", api.revoked)
	}
	inst, err := app.GetInstallation(context.Background(), 42)
	if err != nil || inst.AccountLogin != "acme" {
		t.Fatalf("installation = %v %v", inst, err)
	}
	if _, err := app.GetInstallation(context.Background(), 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown installation err = %v", err)
	}
}

func TestGitHubApp_RefusesOverbroadToken(t *testing.T) {
	app, api := testApp(t)
	api.grant = map[string]string{"contents": "write"}
	if _, err := app.MintRepoToken(context.Background(), 42, Repo{Owner: "acme", Name: "infra"}); err == nil || strings.Contains(err.Error(), testToken) {
		t.Fatalf("write token accepted: %v", err)
	}
	api.grant = map[string]string{"contents": "read"}
	api.grantRepos = []string{"acme/infra", "acme/other"}
	if _, err := app.MintRepoToken(context.Background(), 42, Repo{Owner: "acme", Name: "infra"}); err == nil {
		t.Fatal("multi-repo token accepted")
	}
	if len(api.revoked) != 2 {
		t.Fatalf("over-broad tokens must be revoked: %v", api.revoked)
	}
}

func TestGitHubApp_Disabled(t *testing.T) {
	app := &GitHubApp{Creds: staticCreds{}, Endpoints: Endpoints{APIURL: "http://127.0.0.1:1"}}
	if _, err := app.MintRepoToken(context.Background(), 1, Repo{Owner: "a", Name: "b"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v", err)
	}
	t.Setenv("GITHUB_APP_ID", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", "")
	if _, err := NewGitHubApp(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("NewGitHubApp err = %v", err)
	}
}

func sign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	body := `{"ref":"refs/heads/main"}`
	good := sign("s3cret", body)
	if !VerifySignature([]byte("s3cret"), []byte(body), good) {
		t.Fatal("valid signature rejected")
	}
	for name, h := range map[string]string{
		"unsigned":    "",
		"sha1 header": "sha1=" + strings.Repeat("0", 40),
		"wrong":       sign("other", body),
		"truncated":   good[:len(good)-2],
		"not hex":     "sha256=zz",
		"tampered":    sign("s3cret", body+" "),
	} {
		if VerifySignature([]byte("s3cret"), []byte(body), h) {
			t.Fatalf("%s accepted", name)
		}
	}
	if VerifySignature(nil, []byte(body), sign("", body)) {
		t.Fatal("empty secret must reject")
	}
}

func TestParseRepoURL(t *testing.T) {
	e := Endpoints{WebURL: "https://github.com"}
	for _, ok := range []string{"https://github.com/acme/infra", "https://github.com/acme/infra.git", "https://GitHub.com/acme/infra/"} {
		r, err := ParseRepoURL(ok, e)
		if err != nil || r.FullName() != "acme/infra" {
			t.Fatalf("%s: %v %v", ok, r, err)
		}
	}
	for _, bad := range []string{
		"http://github.com/acme/infra", "https://x-access-token:t@github.com/acme/infra", "https://github.com/acme/infra?x=1",
		"https://gitlab.com/acme/infra", "https://github.com/acme", "https://github.com/acme/infra/tree/main",
		"git@github.com:acme/infra.git", "https://github.com/../infra", "https://github.com/acme/..", "file:///tmp/x",
	} {
		if _, err := ParseRepoURL(bad, e); err == nil {
			t.Fatalf("%s accepted", bad)
		}
	}
}
