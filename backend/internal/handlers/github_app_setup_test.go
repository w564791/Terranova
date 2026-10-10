package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/gitsource"
	"iac-platform/internal/keys"
	"iac-platform/internal/models"
)

// fakeOAuth a mocked GitHub OAuth server: codes map to GitHub users, org
// memberships per org login.
type fakeOAuth struct {
	mu        sync.Mutex
	codes     map[string]gitsource.GitHubUser
	tokens    map[string]gitsource.GitHubUser
	orgRoles  map[string]map[int64]string // org login -> user id -> role
	exchanged int
	revoked   []string
}

func newFakeOAuth() *fakeOAuth {
	return &fakeOAuth{codes: map[string]gitsource.GitHubUser{}, tokens: map[string]gitsource.GitHubUser{}, orgRoles: map[string]map[int64]string{}}
}

func (f *fakeOAuth) ExchangeCode(_ context.Context, code string) (*gitsource.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.codes[code]
	if !ok {
		return nil, gitsource.ErrOAuthCode
	}
	delete(f.codes, code) // codes are single use on GitHub too
	f.exchanged++
	tok := "ghu_" + code
	f.tokens[tok] = u
	return gitsource.NewToken(tok, time.Now().Add(8*time.Hour)), nil
}

func (f *fakeOAuth) AuthenticatedUser(_ context.Context, t *gitsource.Token) (*gitsource.GitHubUser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.tokens[t.Secret()]
	if !ok {
		return nil, gitsource.ErrAuth
	}
	return &u, nil
}

func (f *fakeOAuth) OrgMembership(_ context.Context, t *gitsource.Token, org string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.tokens[t.Secret()]
	if !ok {
		return "", "", gitsource.ErrAuth
	}
	role := f.orgRoles[org][u.ID]
	if role == "" {
		return "", "", gitsource.ErrNotFound
	}
	return "active", role, nil
}

func (f *fakeOAuth) RevokeUserToken(_ context.Context, t *gitsource.Token) {
	f.mu.Lock()
	f.revoked = append(f.revoked, t.Secret())
	f.mu.Unlock()
}

// orgAdminChecker the IAM re-check of the callback.
type orgAdminChecker struct {
	allowAllPermChecker // the other interface methods
	mu                  sync.Mutex
	allow               bool
	reqs                []service.CheckPermissionRequest
}

func (c *orgAdminChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqs = append(c.reqs, *req)
	return &service.CheckPermissionResult{IsAllowed: c.allow}, nil
}

func setSigningRoot(t *testing.T) {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "production")
	t.Setenv("SIGNING_ROOT_KEY", base64.StdEncoding.EncodeToString(b))
	t.Setenv("SIGNING_ROOT_KEY_VERSION", "")
	t.Setenv("SIGNING_ROOT_KEY_PREVIOUS", "")
	t.Setenv("SIGNING_ROOT_KEY_PREVIOUS_VERSION", "")
}

// withCallerOrg like withCaller, in another organization.
func withCallerOrg(org uint, user string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", user)
		c.Set("auth_org_id", org)
		c.Next()
	}
}

type setupEnv struct {
	db    *gorm.DB
	r     *gin.Engine
	app   *fakeGitApp
	oauth *fakeOAuth
	perm  *orgAdminChecker
	e     gitsource.Endpoints
}

func newSetupEnv(t *testing.T) *setupEnv {
	t.Helper()
	setSigningRoot(t)
	db := setupGitDB(t)
	app := newFakeApp()
	app.accounts[55] = "newco"
	app.accounts[88] = "victim"
	app.accounts[99] = "alice"
	app.types = map[int64]string{99: "User"}
	fx := newGitFixture(t)
	e := useGitFixture(t, app, fx, "")
	oauth := newFakeOAuth()
	gitDeps.OAuth = func() (GitHubOAuth, error) { return oauth, nil }
	perm := &orgAdminChecker{allow: true}
	h := NewGitHubAppHandler(db).WithPermissionChecker(perm)
	r := gitRouter(db)
	r.POST("/organizations/:org_id/github-app/connect", withCaller(valueobject.PermissionLevelAdmin), h.Connect)
	r.GET("/github-app/setup/callback", h.SetupCallback)
	r.GET("/organizations/:org_id/github-app/available-installations", withCaller(valueobject.PermissionLevelWrite), h.ListUsableInstallations)
	r.GET("/organizations/:org_id/github-app/available-installations/:installation_id", withCaller(valueobject.PermissionLevelWrite), h.GetUsableInstallation)
	r.GET("/organizations/:org_id/github-app/installations/:installation_id/repositories", withCaller(valueobject.PermissionLevelWrite), h.ListInstallationRepositories)
	r.GET("/org2/github-app/available-installations/:installation_id", withCallerOrg(2, "u2"), h.GetUsableInstallation)
	r.GET("/org2/github-app/installations/:installation_id/repositories", withCallerOrg(2, "u2"), h.ListInstallationRepositories)
	t.Cleanup(func() { githubSetupNow = time.Now })
	return &setupEnv{db: db, r: r, app: app, oauth: oauth, perm: perm, e: e}
}

// connect POST connect as org 1 / u1, returns the state of the install URL.
func (s *setupEnv) connect(t *testing.T) string {
	t.Helper()
	w := doJSON(s.r, "POST", "/organizations/1/github-app/connect", "")
	if w.Code != http.StatusOK {
		t.Fatalf("connect: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		InstallURL string    `json:"install_url"`
		ExpiresAt  time.Time `json:"expires_at"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	prefix := s.e.WebURL + "/apps/terranova-test/installations/new?state="
	if !strings.HasPrefix(resp.InstallURL, prefix) {
		t.Fatalf("install url %q", resp.InstallURL)
	}
	if d := time.Until(resp.ExpiresAt); d < 9*time.Minute || d > 10*time.Minute+time.Second {
		t.Fatalf("expires in %v", d)
	}
	u, _ := url.Parse(resp.InstallURL)
	return u.Query().Get("state")
}

// callback GitHub's redirect to the callback; returns github_app, reason.
func (s *setupEnv) callback(t *testing.T, q url.Values) (string, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.r.ServeHTTP(w, httptest.NewRequest("GET", "/github-app/setup/callback?"+q.Encode(), nil))
	if w.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", w.Code, w.Body.String())
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	lq := loc.Query()
	if loc.Path != githubSetupResultPath || lq.Has("code") || lq.Has("state") || strings.Contains(loc.RawQuery, "ghu_") || strings.Contains(loc.RawQuery, ".ey") {
		t.Fatalf("redirect %q", w.Header().Get("Location"))
	}
	return loc.Query().Get("github_app"), loc.Query().Get("reason")
}

func cbQuery(state, code string, inst int64) url.Values {
	q := url.Values{"state": {state}, "setup_action": {"install"}, "installation_id": {sprintInt(inst)}}
	if code != "" {
		q.Set("code", code)
	}
	return q
}

func (s *setupEnv) binding(t *testing.T, inst int64) *models.GitHubAppInstallation {
	t.Helper()
	var row models.GitHubAppInstallation
	if err := s.db.Where("installation_id = ?", inst).Take(&row).Error; err != nil {
		return nil
	}
	return &row
}

func TestGitHubAppSetupCallbackBindsWithOAuthProof(t *testing.T) {
	s := newSetupEnv(t)
	s.oauth.codes["c-ok"] = gitsource.GitHubUser{ID: 500, Login: "newco-owner"}
	s.oauth.orgRoles["newco"] = map[int64]string{500: "admin"}

	state := s.connect(t)
	if res, reason := s.callback(t, cbQuery(state, "c-ok", 55)); res != setupResultConnected || reason != "" {
		t.Fatalf("callback: %s %s", res, reason)
	}
	row := s.binding(t, 55)
	if row == nil || row.OrganizationID != 1 || row.VerifiedAt == nil || row.VerifiedGitHubLogin == nil || *row.VerifiedGitHubLogin != "newco-owner" ||
		row.AccountID == nil || *row.AccountID != fakeAccountID(55) || row.AccountType == nil || *row.AccountType != "Organization" || row.CreatedBy != "u1" {
		t.Fatalf("binding %+v", row)
	}
	// the user token was revoked and is stored nowhere
	if len(s.oauth.revoked) != 1 || s.oauth.revoked[0] != "ghu_c-ok" {
		t.Fatalf("revoked %v", s.oauth.revoked)
	}
	for _, table := range []string{"github_app_installations", "github_app_setup_nonces", "audit_logs"} {
		var rows []map[string]interface{}
		s.db.Table(table).Find(&rows)
		raw, _ := json.Marshal(rows)
		if strings.Contains(string(raw), "ghu_") || strings.Contains(string(raw), "c-ok") {
			t.Fatalf("%s holds the user token / code: %s", table, raw)
		}
	}
	// the initiating user was re-checked as org ADMIN
	if len(s.perm.reqs) != 1 || s.perm.reqs[0].UserID != "u1" || s.perm.reqs[0].ScopeID != 1 || s.perm.reqs[0].RequiredLevel != valueobject.PermissionLevelAdmin {
		t.Fatalf("permission re-check %+v", s.perm.reqs)
	}

	// user account: the same GitHub user as the account
	s.oauth.codes["c-alice"] = gitsource.GitHubUser{ID: fakeAccountID(99), Login: "alice"}
	if res, _ := s.callback(t, cbQuery(s.connect(t), "c-alice", 99)); res != setupResultConnected {
		t.Fatalf("user account: %s", res)
	}
	s.oauth.codes["c-mallory"] = gitsource.GitHubUser{ID: 4242, Login: "mallory"}
	s.db.Exec(`DELETE FROM github_app_installations WHERE installation_id = 99`)
	if res, reason := s.callback(t, cbQuery(s.connect(t), "c-mallory", 99)); res != setupResultError || reason != setupReasonNotAdmin {
		t.Fatalf("other user on a user account: %s %s", res, reason)
	}
}

// An admin of platform org 1 cannot bind another GitHub account's
// installation by putting its id into the callback: without OAuth proof that
// they administer that account nothing is written.
func TestGitHubAppSetupCrossOrgInstallationNeedsProof(t *testing.T) {
	s := newSetupEnv(t)
	s.oauth.codes["c-member"] = gitsource.GitHubUser{ID: 501, Login: "attacker"}
	s.oauth.orgRoles["victim"] = map[int64]string{501: "member"} // member, not admin
	s.oauth.codes["c-outsider"] = gitsource.GitHubUser{ID: 502, Login: "outsider"}

	for _, tc := range []struct{ code, reason string }{
		{"", setupReasonOAuthMissing},       // no OAuth proof at all
		{"forged", setupReasonOAuthFailed},  // code GitHub does not know
		{"c-member", setupReasonNotAdmin},   // org member, not admin
		{"c-outsider", setupReasonNotAdmin}, // not a member
	} {
		if res, reason := s.callback(t, cbQuery(s.connect(t), tc.code, 88)); res != setupResultError || reason != tc.reason {
			t.Fatalf("code %q: %s %s, want %s", tc.code, res, reason, tc.reason)
		}
		if s.binding(t, 88) != nil {
			t.Fatalf("code %q bound the installation", tc.code)
		}
	}
	// the manual endpoint cannot be used instead
	h := NewGitHubAppHandler(s.db)
	s.r.POST("/organizations/:org_id/github-app/installations", withCaller(valueobject.PermissionLevelAdmin), h.RegisterInstallation)
	if w := doJSON(s.r, "POST", "/organizations/1/github-app/installations", `{"installation_id":88}`); w.Code != http.StatusGone || s.binding(t, 88) != nil {
		t.Fatalf("manual register: %d", w.Code)
	}
	// the platform user lost org ADMIN meanwhile
	s.oauth.codes["c-victim-admin"] = gitsource.GitHubUser{ID: 503, Login: "victim-admin"}
	s.oauth.orgRoles["victim"][503] = "admin"
	state := s.connect(t)
	s.perm.allow = false
	if res, reason := s.callback(t, cbQuery(state, "c-victim-admin", 88)); reason != setupReasonForbidden || res != setupResultError || s.binding(t, 88) != nil {
		t.Fatalf("no longer admin: %s %s", res, reason)
	}
}

func TestGitHubAppSetupStateExpiredReplayedForged(t *testing.T) {
	s := newSetupEnv(t)
	s.oauth.orgRoles["newco"] = map[int64]string{500: "admin"}
	code := func(c string) string {
		s.oauth.codes[c] = gitsource.GitHubUser{ID: 500, Login: "newco-owner"}
		return c
	}

	// expired: 10 minutes
	state := s.connect(t)
	githubSetupNow = func() time.Time { return time.Now().Add(10*time.Minute + 5*time.Second) }
	if res, reason := s.callback(t, cbQuery(state, code("c1"), 55)); res != setupResultError || reason != setupReasonStateExpired || s.binding(t, 55) != nil {
		t.Fatalf("expired: %s %s", res, reason)
	}
	githubSetupNow = time.Now

	// replay: the state is single use, even with a fresh valid code
	state = s.connect(t)
	if res, _ := s.callback(t, cbQuery(state, code("c2"), 55)); res != setupResultConnected {
		t.Fatalf("first use: %s", res)
	}
	s.db.Exec(`DELETE FROM github_app_installations WHERE installation_id = 55`)
	if res, reason := s.callback(t, cbQuery(state, code("c3"), 55)); res != setupResultError || reason != setupReasonStateUsed || s.binding(t, 55) != nil {
		t.Fatalf("replay: %s %s", res, reason)
	}

	// forged / tampered / wrong purpose / missing
	parts := strings.Split(s.connect(t), ".")
	claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
	evil := strings.Replace(string(claims), `"org":1`, `"org":2`, 1)
	tampered := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(evil)) + "." + parts[2]
	userPurpose, err := keys.Sign(keys.PurposeUser, githubSetupClaims{OrgID: 1, UserID: "u1", RegisteredClaims: jwt.RegisteredClaims{
		ID: "n-user", Audience: jwt.ClaimStrings{githubStateAudience}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	noKid, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, githubSetupClaims{OrgID: 1, UserID: "u1", RegisteredClaims: jwt.RegisteredClaims{
		ID: "n-nokid", Audience: jwt.ClaimStrings{githubStateAudience}, IssuedAt: jwt.NewNumericDate(time.Now()), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
	}}).SignedString([]byte("guessable"))
	for name, st := range map[string]string{"tampered": tampered, "user purpose": userPurpose, "no kid": noKid, "missing": "", "garbage": "x.y.z"} {
		if res, reason := s.callback(t, cbQuery(st, code("c-"+name), 55)); res != setupResultError || reason != setupReasonStateInvalid {
			t.Fatalf("%s: %s %s", name, res, reason)
		}
	}
	if s.binding(t, 55) != nil {
		t.Fatal("forged state bound an installation")
	}
	// setup_action=request: nothing bound, the state stays usable
	state = s.connect(t)
	q := url.Values{"state": {state}, "setup_action": {"request"}}
	if res, _ := s.callback(t, q); res != setupResultRequested || s.binding(t, 55) != nil {
		t.Fatalf("request: %s", res)
	}
	if res, _ := s.callback(t, cbQuery(state, code("c4"), 55)); res != setupResultConnected {
		t.Fatalf("after request: %s", res)
	}
	// expired nonces are cleaned up
	if err := cleanupGitHubEphemera(s.db, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var nonces int64
	s.db.Model(&models.GitHubAppSetupNonce{}).Count(&nonces)
	if nonces != 0 {
		t.Fatalf("nonces left: %d", nonces)
	}
}

func TestGitHubAppSetupDuplicateInstallationRejected(t *testing.T) {
	s := newSetupEnv(t)
	// 77 is verified for org 2 (fixture); a proven admin from org 1 is refused
	s.oauth.codes["c-other"] = gitsource.GitHubUser{ID: 600, Login: "other-owner"}
	s.oauth.orgRoles["other"] = map[int64]string{600: "admin"}
	if res, reason := s.callback(t, cbQuery(s.connect(t), "c-other", 77)); res != setupResultError || reason != setupReasonBoundElsewhere {
		t.Fatalf("duplicate: %s %s", res, reason)
	}
	if row := s.binding(t, 77); row == nil || row.OrganizationID != 2 {
		t.Fatalf("binding of org 2 changed: %+v", row)
	}
	// the database refuses a second row too
	if err := s.db.Exec(`INSERT INTO github_app_installations (organization_id, installation_id, account_login, created_by) VALUES (1, 77, 'other', 'x')`).Error; err == nil {
		t.Fatal("duplicate installation_id inserted")
	}
	// an unverified (manual, earlier build) row of another org has no standing
	s.db.Exec(`INSERT INTO github_app_installations (organization_id, installation_id, account_login, created_by) VALUES (2, 55, 'newco', 'x')`)
	s.oauth.codes["c-newco"] = gitsource.GitHubUser{ID: 500, Login: "newco-owner"}
	s.oauth.orgRoles["newco"] = map[int64]string{500: "admin"}
	if res, _ := s.callback(t, cbQuery(s.connect(t), "c-newco", 55)); res != setupResultConnected {
		t.Fatalf("replace unverified: %s", res)
	}
	if row := s.binding(t, 55); row == nil || row.OrganizationID != 1 || row.VerifiedAt == nil {
		t.Fatalf("binding %+v", row)
	}
	// re-connecting the same org's installation refreshes the proof
	s.oauth.codes["c-again"] = gitsource.GitHubUser{ID: 500, Login: "newco-owner"}
	if res, _ := s.callback(t, cbQuery(s.connect(t), "c-again", 55)); res != setupResultConnected {
		t.Fatalf("reconnect: %s", res)
	}
	var n int64
	s.db.Model(&models.GitHubAppInstallation{}).Where("installation_id = 55").Count(&n)
	if n != 1 {
		t.Fatalf("rows for 55: %d", n)
	}
}

func TestGitHubAppConnectNeedsConfiguration(t *testing.T) {
	s := newSetupEnv(t)
	gitDeps.OAuth = func() (GitHubOAuth, error) { return nil, gitsource.ErrOAuthDisabled }
	if w := doJSON(s.r, "POST", "/organizations/1/github-app/connect", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no oauth client: %d", w.Code)
	}
	gitDeps.OAuth = func() (GitHubOAuth, error) { return s.oauth, nil }
	t.Setenv("SIGNING_ROOT_KEY", "")
	t.Setenv("ENV", "development")
	if w := doJSON(s.r, "POST", "/organizations/1/github-app/connect", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no signing root: %d %s", w.Code, w.Body.String())
	}
}

// Manifest authors (MANIFESTS WRITE) see bound installations of their org
// only (id + account) and their repositories through a metadata-only token.
func TestGitHubAppUsableInstallationsAndRepositories(t *testing.T) {
	s := newSetupEnv(t)
	s.db.Exec(`INSERT INTO github_app_installations (organization_id, installation_id, account_login, created_by) VALUES (1, 55, 'newco', 'x')`) // unverified
	w := doJSON(s.r, "GET", "/organizations/1/github-app/available-installations", "")
	if w.Code != http.StatusOK || w.Body.String() != `{"installations":[{"id":42,"account":"acme"}]}` {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(s.r, "GET", "/organizations/1/github-app/available-installations/42", ""); w.Code != http.StatusOK || w.Body.String() != `{"id":42,"account":"acme"}` {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{
		"/organizations/1/github-app/available-installations/77", // org 2's
		"/organizations/1/github-app/available-installations/55", // unverified
		"/organizations/1/github-app/installations/77/repositories",
		"/organizations/1/github-app/installations/55/repositories",
		"/org2/github-app/available-installations/42", // org 1's, seen from org 2
		"/org2/github-app/installations/42/repositories",
	} {
		if w := doJSON(s.r, "GET", path, ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	minted := len(s.app.minted)
	s.app.repos["acme/zeta"] = true
	s.app.repos["acme/alpha"] = true
	w = doJSON(s.r, "GET", "/organizations/1/github-app/installations/42/repositories?per_page=2&page=1", "")
	var resp struct {
		Repositories []gitsource.RepoInfo `json:"repositories"`
		TotalCount   int                  `json:"total_count"`
		Page         int                  `json:"page"`
		PerPage      int                  `json:"per_page"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusOK || resp.TotalCount != 3 || len(resp.Repositories) != 2 || resp.Page != 1 || resp.PerPage != 2 ||
		resp.Repositories[0] != (gitsource.RepoInfo{FullName: "acme/alpha", DefaultBranch: "main", Private: true, HTMLURL: s.e.WebURL + "/acme/alpha"}) {
		t.Fatalf("repositories: %d %s", w.Code, w.Body.String())
	}
	if got := s.app.minted[minted:]; len(got) != 1 || got[0] != "42 metadata" {
		t.Fatalf("minted %v, want one metadata token", got)
	}
	if s.app.revoked == 0 {
		t.Fatal("metadata token not revoked")
	}
	if strings.Contains(w.Body.String(), fakeMetadataToken) {
		t.Fatal("token in response")
	}
	w = doJSON(s.r, "GET", "/organizations/1/github-app/installations/42/repositories?per_page=2&page=2", "")
	if !strings.Contains(w.Body.String(), `"full_name":"acme/zeta"`) || strings.Contains(w.Body.String(), "acme/alpha") {
		t.Fatalf("page 2: %s", w.Body.String())
	}
}

// Create takes installation_id + repo full name; the host is always the
// configured one; the installation must be bound to the caller's org and
// reach the repo with a scoped token. Publish re-checks the binding.
func TestGitManifestCreateRepoValidationAndHost(t *testing.T) {
	s := newSetupEnv(t)
	n := 0
	create := func(body map[string]any) *httptest.ResponseRecorder {
		body["source_type"] = "git"
		if _, ok := body["name"]; !ok {
			n++
			body["name"] = "g-" + sprintInt(int64(n))
		}
		raw, _ := json.Marshal(body)
		return doJSON(s.r, "POST", "/organizations/1/manifests", string(raw))
	}
	minted := len(s.app.minted)
	// request-supplied hosts are rejected before anything is contacted
	for _, b := range []map[string]any{
		{"git_repo_url": "https://evil.example.com/acme/infra", "github_installation_id": 42},
		{"git_repo_url": "https://user:pw@" + strings.TrimPrefix(s.e.WebURL, "http://") + "/acme/infra", "github_installation_id": 42},
		{"git_repo": "evil.example.com/acme/infra", "github_installation_id": 42},
		{"git_repo": "https://evil.example.com/acme/infra", "github_installation_id": 42},
		{"git_repo": "acme/infra.git", "github_installation_id": 42},
		{"git_repo": "acme/infra", "git_repo_url": s.e.WebURL + "/acme/infra", "github_installation_id": 42},
		{"git_repo": "acme/infra", "github_installation_id": 42, "github_api_url": "https://evil.example.com"},
	} {
		w := create(b)
		if _, extra := b["github_api_url"]; extra {
			// unknown fields are ignored: created against the configured host
			var m map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &m)
			if w.Code != http.StatusCreated || m["git_repo_url"] != s.e.WebURL+"/acme/infra" {
				t.Fatalf("%v: %d %s", b, w.Code, w.Body.String())
			}
			continue
		}
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%v: %d %s", b, w.Code, w.Body.String())
		}
	}
	if got := s.app.minted[minted:]; len(got) != 1 || got[0] != "42 acme/infra" {
		t.Fatalf("minted %v (only the configured-host create may mint)", got)
	}
	// installation of another org / unverified / not owning the repo / repo not accessible
	s.db.Exec(`INSERT INTO github_app_installations (organization_id, installation_id, account_login, created_by) VALUES (1, 55, 'newco', 'x')`)
	for _, tc := range []struct {
		body map[string]any
		code int
	}{
		{map[string]any{"git_repo": "other/infra", "github_installation_id": 77}, http.StatusUnprocessableEntity},
		{map[string]any{"git_repo": "newco/infra", "github_installation_id": 55}, http.StatusUnprocessableEntity},
		{map[string]any{"git_repo": "other/infra", "github_installation_id": 42}, http.StatusUnprocessableEntity},
		{map[string]any{"git_repo": "acme/secret", "github_installation_id": 42}, http.StatusUnprocessableEntity},
	} {
		if w := create(tc.body); w.Code != tc.code {
			t.Fatalf("%v: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	// git_repo creates with the canonical URL from config
	w := create(map[string]any{"name": "g2", "git_repo": "acme/infra", "github_installation_id": 42})
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if w.Code != http.StatusCreated || m["git_repo_url"] != s.e.WebURL+"/acme/infra" {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	id := m["id"].(string)
	// publish / branch listing re-check the binding of the manifest's org
	s.db.Exec(`DELETE FROM github_app_installations WHERE installation_id = 42`)
	before := len(s.app.minted)
	for _, req := range [][2]string{
		{"POST", "/organizations/1/manifests/" + id + "/v2/versions"},
		{"GET", "/organizations/1/manifests/" + id + "/git/branches"},
	} {
		body := ""
		if req[0] == "POST" {
			body = `{"version":"v1.0.0","commit_sha":"` + strings.Repeat("a", 40) + `"}`
		}
		if w := doJSON(s.r, req[0], req[1], body); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), gitCodeInstallationNotReg) {
			t.Fatalf("%s %s after unbinding: %d %s", req[0], req[1], w.Code, w.Body.String())
		}
	}
	if len(s.app.minted) != before {
		t.Fatal("token minted for an unbound installation")
	}
	// the installation re-bound to another org does not serve this one
	s.db.Exec(`INSERT INTO github_app_installations (organization_id, installation_id, account_login, account_id, account_type, verified_github_user_id, verified_github_login, verified_at, created_by) VALUES (2, 42, 'acme', 42000, 'Organization', 1, 'x', CURRENT_TIMESTAMP, 'x')`)
	if w := doJSON(s.r, "GET", "/organizations/1/manifests/"+id+"/git/branches", ""); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("branches with installation of org 2: %d", w.Code)
	}
}
