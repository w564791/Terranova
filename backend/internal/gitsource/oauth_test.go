package gitsource

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testClientID     = "Iv1.testclient"
	testClientSecret = "test-client-secret-value"
	testUserToken    = "ghu_TestUserToken0123456789"
	testMetaToken    = "ghs_TestMetadataToken0123456789"
)

// fakeGitHub the OAuth / user / installation-repositories endpoints.
type fakeGitHub struct {
	*httptest.Server
	mu        sync.Mutex
	requests  []string // method path?query
	exchange  []string // form bodies (must carry the secret, never the URL)
	revoked   []string
	role      string // membership role of the user in "acme"
	userID    int64
	metaPerms map[string]string
	mintBody  []map[string]any
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{role: "admin", userID: 500, metaPerms: map[string]string{"metadata": "read"}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		auth := r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login/oauth/access_token":
			_ = r.ParseForm()
			f.exchange = append(f.exchange, r.PostForm.Encode())
			if r.PostForm.Get("client_secret") != testClientSecret || r.PostForm.Get("code") != "good-code" {
				_, _ = w.Write([]byte(`{"error":"bad_verification_code"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"` + testUserToken + `","token_type":"bearer","expires_in":28800}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/user":
			if auth != "Bearer "+testUserToken {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			_, _ = fmt.Fprintf(w, `{"id":%d,"login":"octo"}`, f.userID)
		case r.Method == http.MethodGet && r.URL.Path == "/api/user/memberships/orgs/acme":
			if auth != "Bearer "+testUserToken || f.role == "" {
				http.NotFound(w, r)
				return
			}
			_, _ = fmt.Fprintf(w, `{"state":"active","role":%q}`, f.role)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/applications/"+testClientID+"/token":
			u, p, ok := r.BasicAuth()
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if ok && u == testClientID && p == testClientSecret {
				f.revoked = append(f.revoked, body["access_token"])
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/app/installations/42/access_tokens":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mintBody = append(f.mintBody, body)
			_ = json.NewEncoder(w).Encode(map[string]any{"token": testMetaToken, "expires_at": time.Now().Add(time.Hour), "permissions": f.metaPerms})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/installation/token":
			f.revoked = append(f.revoked, strings.TrimPrefix(auth, "Bearer "))
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/installation/repositories":
			if auth != "Bearer "+testMetaToken {
				http.Error(w, "no", http.StatusUnauthorized)
				return
			}
			// html_url / clone_url point elsewhere: the client must ignore them
			_, _ = w.Write([]byte(`{"total_count":3,"repositories":[
				{"full_name":"acme/infra","default_branch":"main","private":true,"html_url":"https://evil.example.com/acme/infra","clone_url":"https://evil.example.com/x.git"},
				{"full_name":"acme/../etc","default_branch":"main"},
				{"full_name":"acme/docs","default_branch":"trunk","private":false}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/app":
			_, _ = w.Write([]byte(`{"slug":"terranova","html_url":"` + f.URL + `/apps/terranova"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeGitHub) endpoints() Endpoints { return Endpoints{WebURL: f.URL, APIURL: f.URL + "/api"} }

func TestOAuthApp_ExchangeVerifyRevoke(t *testing.T) {
	gh := newFakeGitHub(t)
	o := &OAuthApp{Creds: NewOAuthClientCredentials(testClientID, testClientSecret), Endpoints: gh.endpoints()}
	ctx := context.Background()

	if _, err := o.ExchangeCode(ctx, "bad-code"); !errors.Is(err, ErrOAuthCode) {
		t.Fatalf("bad code: %v", err)
	}
	tok, err := o.ExchangeCode(ctx, "good-code")
	if err != nil || tok.Secret() != testUserToken {
		t.Fatalf("exchange: %v", err)
	}
	// secret and code in the form body, never in a URL
	for _, req := range gh.requests {
		if strings.Contains(req, testClientSecret) || strings.Contains(req, "good-code") || strings.Contains(req, testUserToken) {
			t.Fatalf("secret in URL: %s", req)
		}
	}
	if !strings.Contains(gh.exchange[1], "client_secret="+testClientSecret) {
		t.Fatalf("exchange body %q", gh.exchange[1])
	}
	if s := fmt.Sprintf("%v %+v %#v %s", tok, tok, tok, o.Creds); strings.Contains(s, testUserToken) || strings.Contains(s, testClientSecret) {
		t.Fatalf("formatting leaks: %s", s)
	}

	org := &Installation{ID: 42, AccountID: 9000, AccountLogin: "acme", AccountType: "Organization"}
	if u, err := VerifyInstallationAdmin(ctx, o, tok, org); err != nil || u.Login != "octo" {
		t.Fatalf("org admin: %v %v", u, err)
	}
	gh.role = "member"
	if _, err := VerifyInstallationAdmin(ctx, o, tok, org); !errors.Is(err, ErrNotInstallationAdmin) {
		t.Fatalf("member: %v", err)
	}
	gh.role = ""
	if _, err := VerifyInstallationAdmin(ctx, o, tok, org); !errors.Is(err, ErrNotInstallationAdmin) {
		t.Fatalf("not a member: %v", err)
	}
	user := &Installation{ID: 42, AccountID: 500, AccountLogin: "octo", AccountType: "User"}
	if _, err := VerifyInstallationAdmin(ctx, o, tok, user); err != nil {
		t.Fatalf("same user: %v", err)
	}
	user.AccountID = 501
	if _, err := VerifyInstallationAdmin(ctx, o, tok, user); !errors.Is(err, ErrNotInstallationAdmin) {
		t.Fatalf("other user: %v", err)
	}
	if _, err := VerifyInstallationAdmin(ctx, o, tok, &Installation{ID: 42, AccountID: 1, AccountLogin: "ent", AccountType: "Enterprise"}); !errors.Is(err, ErrNotInstallationAdmin) {
		t.Fatalf("enterprise: %v", err)
	}

	o.RevokeUserToken(ctx, tok)
	if len(gh.revoked) != 1 || gh.revoked[0] != testUserToken {
		t.Fatalf("revoked %v", gh.revoked)
	}
}

func TestOAuthCredentialsFromEnv(t *testing.T) {
	t.Setenv("GITHUB_APP_CLIENT_ID", "")
	t.Setenv("GITHUB_APP_CLIENT_SECRET", "x")
	if _, err := OAuthCredentialsFromEnv(); !errors.Is(err, ErrOAuthDisabled) {
		t.Fatalf("missing id: %v", err)
	}
	t.Setenv("GITHUB_APP_CLIENT_ID", "Iv1.x")
	if c, err := OAuthCredentialsFromEnv(); err != nil || c.ClientID != "Iv1.x" {
		t.Fatalf("configured: %v", err)
	}
}

func metadataApp(t *testing.T, gh *fakeGitHub) *GitHubApp {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &GitHubApp{Creds: staticCreds{creds: AppCredentials{AppID: 1234, PrivateKey: key}}, Endpoints: gh.endpoints()}
}

// The repositories list uses a metadata-only token and builds html_url from
// the configured GITHUB_URL, ignoring hosts in the API response.
func TestGitHubApp_ListInstallationReposMetadataOnly(t *testing.T) {
	gh := newFakeGitHub(t)
	app := metadataApp(t, gh)
	ctx := context.Background()
	tok, err := app.MintMetadataToken(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(gh.mintBody[0]); string(b) != `{"permissions":{"metadata":"read"}}` {
		t.Fatalf("mint body %s", b)
	}
	repos, total, err := app.ListInstallationRepos(ctx, tok, 2, 50)
	if err != nil || total != 3 || len(repos) != 2 {
		t.Fatalf("repos %v %d %v", repos, total, err)
	}
	if repos[0] != (RepoInfo{FullName: "acme/infra", DefaultBranch: "main", Private: true, HTMLURL: gh.URL + "/acme/infra"}) || repos[1].HTMLURL != gh.URL+"/acme/docs" {
		t.Fatalf("repos %+v", repos)
	}
	last := gh.requests[len(gh.requests)-1]
	if last != "GET /api/installation/repositories?per_page=50&page=2" {
		t.Fatalf("request %s", last)
	}
	// a token granting more than metadata:read is revoked and refused
	gh.metaPerms = map[string]string{"metadata": "read", "contents": "read"}
	if _, err := app.MintMetadataToken(ctx, 42); err == nil || len(gh.revoked) != 1 || gh.revoked[0] != testMetaToken {
		t.Fatalf("over-broad token: %v revoked %v", err, gh.revoked)
	}
	for _, req := range gh.requests {
		if strings.Contains(req, "evil.example.com") {
			t.Fatalf("contacted a host from the response: %s", req)
		}
	}
}

func TestInstallURLAndRepoFullName(t *testing.T) {
	gh := newFakeGitHub(t)
	app := metadataApp(t, gh)
	info, err := app.GetApp(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	u, err := InstallURL(info, gh.endpoints(), "a.b+c")
	if err != nil || u != gh.URL+"/apps/terranova/installations/new?state=a.b%2Bc" {
		t.Fatalf("install url %q %v", u, err)
	}
	if _, err := InstallURL(&AppInfo{HTMLURL: "https://evil.example.com/apps/x"}, gh.endpoints(), "s"); err == nil {
		t.Fatal("install URL on another host accepted")
	}
	for _, ok := range []string{"acme/infra", "a-b/c.d_e"} {
		if _, err := ParseRepoFullName(ok); err != nil {
			t.Fatalf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "acme", "evil.com/acme/infra", "https://evil.com/acme/infra", "acme/infra.git", "acme/..", "acme/infra/x", "-x/y", "acme/in fra", "//evil.com/x"} {
		if _, err := ParseRepoFullName(bad); !errors.Is(err, ErrInvalidRepoName) {
			t.Fatalf("%q accepted", bad)
		}
	}
}
