package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/gitsource"
	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const fakeRepoToken = "ghs_FakeInstallationToken_0123456789abcdefXYZ"

// disableGitSource: no GitHub App configured.
func disableGitSource(t *testing.T) {
	t.Helper()
	prev := gitDeps
	t.Cleanup(func() { gitDeps = prev })
	gitDeps = GitSourceDeps{
		App:           func() (GitApp, error) { return nil, gitsource.ErrDisabled },
		Fetcher:       func() (*gitsource.Fetcher, error) { return nil, gitsource.ErrDisabled },
		Endpoints:     func() (gitsource.Endpoints, error) { return gitsource.Endpoints{WebURL: "https://github.com"}, nil },
		WebhookSecret: func() ([]byte, error) { return nil, gitsource.ErrWebhookDisabled },
	}
}

// fakeGitApp a mocked token minter / GitHub API.
type fakeGitApp struct {
	mu       sync.Mutex
	minted   []string // "installation repo"
	revoked  int
	accounts map[int64]string
	repos    map[string]bool // full names the installation can read
}

func (f *fakeGitApp) MintRepoToken(_ context.Context, inst int64, repo gitsource.Repo) (*gitsource.Token, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.minted = append(f.minted, sprintInt(inst)+" "+repo.FullName())
	if f.accounts[inst] == "" || !f.repos[repo.FullName()] {
		return nil, gitsource.ErrNotFound
	}
	return gitsource.NewToken(fakeRepoToken, time.Now().Add(time.Hour)), nil
}
func (f *fakeGitApp) GetInstallation(_ context.Context, inst int64) (*gitsource.Installation, error) {
	if f.accounts[inst] == "" {
		return nil, gitsource.ErrNotFound
	}
	return &gitsource.Installation{ID: inst, AccountLogin: f.accounts[inst]}, nil
}
func (f *fakeGitApp) ListBranches(_ context.Context, _ gitsource.Repo, t *gitsource.Token) ([]gitsource.Branch, error) {
	if t.Secret() != fakeRepoToken {
		return nil, gitsource.ErrAuth
	}
	return []gitsource.Branch{{Name: "main", SHA: strings.Repeat("a", 40)}}, nil
}
func (f *fakeGitApp) ListCommits(_ context.Context, _ gitsource.Repo, ref string, _ int, t *gitsource.Token) ([]gitsource.Commit, error) {
	if t.Secret() != fakeRepoToken {
		return nil, gitsource.ErrAuth
	}
	return []gitsource.Commit{{SHA: strings.Repeat("b", 40), Subject: "on " + ref}}, nil
}
func (f *fakeGitApp) Revoke(context.Context, *gitsource.Token) {
	f.mu.Lock()
	f.revoked++
	f.mu.Unlock()
}

func sprintInt(i int64) string { b, _ := json.Marshal(i); return string(b) }

// --- fake git server (git http-backend behind basic auth) -----------------

type gitFixture struct {
	srvURL string
	work   string
	bare   string
	urls   []string
	mu     sync.Mutex
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newGitFixture(t *testing.T) *gitFixture {
	t.Helper()
	execPath, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	root := t.TempDir()
	f := &gitFixture{bare: filepath.Join(root, "acme", "infra.git"), work: t.TempDir()}
	if err := os.MkdirAll(f.bare, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, f.bare, "init", "-q", "--bare", "-b", "main")
	runGit(t, f.bare, "config", "uploadpack.allowAnySHA1InWant", "true")
	runGit(t, f.work, "init", "-q", "-b", "main")
	backend := &cgi.Handler{
		Path: filepath.Join(strings.TrimSpace(string(execPath)), "git-http-backend"),
		Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.urls = append(f.urls, r.URL.String())
		f.mu.Unlock()
		if u, p, ok := r.BasicAuth(); !ok || u != "x-access-token" || p != fakeRepoToken {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	f.srvURL = srv.URL
	return f
}

func (f *gitFixture) commit(t *testing.T, files map[string]string, msg string) string {
	t.Helper()
	for p, c := range files {
		full := filepath.Join(f.work, p)
		if c == "" {
			_ = os.Remove(full)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, f.work, "add", "-A")
	runGit(t, f.work, "commit", "-q", "--allow-empty", "-m", msg)
	runGit(t, f.work, "push", "-q", f.bare, "HEAD:refs/heads/main")
	return runGit(t, f.work, "rev-parse", "HEAD")
}

// useGitFixture wires gitDeps to the fake App and the fake git server.
func useGitFixture(t *testing.T, app *fakeGitApp, fx *gitFixture, secret string) gitsource.Endpoints {
	t.Helper()
	prev := gitDeps
	t.Cleanup(func() { gitDeps = prev })
	e := gitsource.Endpoints{WebURL: fx.srvURL, APIURL: fx.srvURL + "/api"}
	gitDeps = GitSourceDeps{
		App:       func() (GitApp, error) { return app, nil },
		Fetcher:   func() (*gitsource.Fetcher, error) { return &gitsource.Fetcher{Endpoints: e, Timeout: time.Minute}, nil },
		Endpoints: func() (gitsource.Endpoints, error) { return e, nil },
		WebhookSecret: func() ([]byte, error) {
			if secret == "" {
				return nil, gitsource.ErrWebhookDisabled
			}
			return []byte(secret), nil
		},
	}
	return e
}

func setupGitDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openSQLiteWithNow(t)
	for _, stmt := range []string{
		`CREATE TABLE manifests (id TEXT PRIMARY KEY, organization_id INTEGER, name TEXT, description TEXT, status TEXT, source_type TEXT NOT NULL DEFAULT 'native', git_repo_url TEXT, git_subpath TEXT, github_installation_id INTEGER, git_latest_sha TEXT, git_latest_ref TEXT, git_latest_at DATETIME, created_by TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, variables TEXT, changelog TEXT, bundle_hash TEXT, bundle_invalid_reason TEXT, source_ref TEXT, created_by TEXT, created_at DATETIME)`,
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY AUTOINCREMENT, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary INTEGER, mode INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE github_app_installations (id INTEGER PRIMARY KEY AUTOINCREMENT, organization_id INTEGER NOT NULL, installation_id INTEGER NOT NULL UNIQUE, account_login TEXT NOT NULL, created_by TEXT NOT NULL, created_at DATETIME)`,
		`CREATE TABLE modules (id INTEGER PRIMARY KEY, status TEXT, module_source TEXT)`,
		`CREATE TABLE module_versions (id INTEGER PRIMARY KEY, module_id INTEGER, module_source TEXT)`,
		`INSERT INTO modules (id, status, module_source) VALUES (1, 'active', 'git::https://github.com/acme/mods.git')`,
		`INSERT INTO github_app_installations (organization_id, installation_id, account_login, created_by) VALUES (1, 42, 'acme', 'admin'), (2, 77, 'other', 'admin')`,
		`INSERT INTO manifests (id, organization_id, name, status, created_by) VALUES ('mf-native', 1, 'native', 'draft', 'u1')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	return db
}

func gitRouter(db *gorm.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mh := NewManifestHandler(db, nil)
	vh := NewManifestVersionsHandler(db)
	gh := NewManifestGitHandler(db)
	fh := NewManifestFilesHandler(db)
	r.POST("/organizations/:org_id/manifests", withCaller(valueobject.PermissionLevelWrite), mh.CreateManifest)
	r.POST("/organizations/:org_id/manifests/:id/v2/versions", withCaller(valueobject.PermissionLevelWrite), vh.PublishVersion)
	r.GET("/organizations/:org_id/manifests/:id/git/branches", withCaller(valueobject.PermissionLevelWrite), gh.ListBranches)
	r.GET("/organizations/:org_id/manifests/:id/git/commits", withCaller(valueobject.PermissionLevelWrite), gh.ListCommits)
	r.PUT("/organizations/:org_id/manifests/:id/files/*path", withCaller(valueobject.PermissionLevelWrite), ManifestNativeOnly(db), fh.PutFile)
	r.POST("/webhooks/github", NewGitHubWebhookHandler(db).Receive)
	return r
}

func createGitManifest(t *testing.T, r http.Handler, e gitsource.Endpoints, name, subpath string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"name": name, "source_type": "git", "git_repo_url": e.WebURL + "/acme/infra.git", "git_subpath": subpath, "github_installation_id": 42})
	w := doJSON(r, "POST", "/organizations/1/manifests", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("create git manifest: %d %s", w.Code, w.Body.String())
	}
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if m["source_type"] != "git" || m["git_repo_url"] != e.WebURL+"/acme/infra" || m["github_installation_id"] != float64(42) {
		t.Fatalf("created = %v", m)
	}
	return m["id"].(string)
}

func newFakeApp() *fakeGitApp {
	return &fakeGitApp{accounts: map[int64]string{42: "acme", 77: "other"}, repos: map[string]bool{"acme/infra": true}}
}

func TestCreateGitManifest(t *testing.T) {
	db := setupGitDB(t)
	fx := newGitFixture(t)
	app := newFakeApp()
	e := useGitFixture(t, app, fx, "")
	r := gitRouter(db)

	createGitManifest(t, r, e, "g1", "")
	for name, tc := range map[string]struct {
		body string
		code int
		want string
	}{
		"bad url":             {`{"name":"x1","source_type":"git","git_repo_url":"https://evil.example/acme/infra","github_installation_id":42}`, 400, "git_repo_url"},
		"token in url":        {`{"name":"x2","source_type":"git","git_repo_url":"` + strings.Replace(e.WebURL, "://", "://x-access-token:abc@", 1) + `/acme/infra","github_installation_id":42}`, 400, "git_repo_url"},
		"no installation":     {`{"name":"x3","source_type":"git","git_repo_url":"` + e.WebURL + `/acme/infra"}`, 400, "github_installation_id"},
		"other org's install": {`{"name":"x4","source_type":"git","git_repo_url":"` + e.WebURL + `/other/infra","github_installation_id":77}`, 422, "github_installation_not_registered"},
		"owner mismatch":      {`{"name":"x5","source_type":"git","git_repo_url":"` + e.WebURL + `/someone/infra","github_installation_id":42}`, 422, "git_repo_not_accessible"},
		"repo not accessible": {`{"name":"x6","source_type":"git","git_repo_url":"` + e.WebURL + `/acme/secret","github_installation_id":42}`, 422, "git_repo_not_accessible"},
		"bad subpath":         {`{"name":"x7","source_type":"git","git_repo_url":"` + e.WebURL + `/acme/infra","git_subpath":"../up","github_installation_id":42}`, 400, "git_subpath"},
	} {
		w := doJSON(r, "POST", "/organizations/1/manifests", tc.body)
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	var n int64
	db.Model(&models.Manifest{}).Where("source_type = 'git'").Count(&n)
	if n != 1 {
		t.Fatalf("git manifests = %d, want 1", n)
	}
	if app.revoked != len(app.minted)-1 { // the failed mint (not accessible) returns no token
		t.Fatalf("minted %d, revoked %d", len(app.minted), app.revoked)
	}
}

func TestPublishGitManifest(t *testing.T) {
	db := setupGitDB(t)
	fx := newGitFixture(t)
	app := newFakeApp()
	e := useGitFixture(t, app, fx, "")
	r := gitRouter(db)
	id := createGitManifest(t, r, e, "g1", "stack")

	sha := "0123456789abcdef0123456789abcdef01234567"
	sha1 := fx.commit(t, map[string]string{
		"README.md":          "outside the subpath\n",
		"stack/main.tf":      "variable \"region\" {}\nmodule \"net\" {\n  source = \"git::https://github.com/acme/mods.git//net?ref=" + sha + "\"\n}\n",
		"stack/modules/a.tf": "# local\n",
	}, "feat: first stack\n\nlong body")

	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	// commit_sha is required, full SHA only
	for _, body := range []string{`{"version":"v1.0.0"}`, `{"version":"v1.0.0","commit_sha":"main"}`, `{"version":"v1.0.0","commit_sha":"` + sha1[:12] + `"}`} {
		if w := doJSON(r, "POST", "/organizations/1/manifests/"+id+"/v2/versions", body); w.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	w := doJSON(r, "POST", "/organizations/1/manifests/"+id+"/v2/versions", `{"version":"v1.0.0","commit_sha":"`+sha1+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	var v models.ManifestVersion
	if err := db.First(&v, "id = ?", resp["id"]).Error; err != nil {
		t.Fatal(err)
	}
	if v.SourceRef == nil || *v.SourceRef != sha1 || resp["source_ref"] != sha1 || v.Changelog != "feat: first stack" {
		t.Fatalf("version = %+v resp = %v", v, resp)
	}
	b, err := manifestbundle.OpenVersion(context.Background(), db, id, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, f := range b.Files {
		paths = append(paths, f.Path)
	}
	if strings.Join(paths, ",") != "main.tf,modules/a.tf" {
		t.Fatalf("bundle paths = %v (subpath is the bundle root)", paths)
	}
	if err := b.Verify(); err != nil || b.Hash != resp["bundle_hash"] {
		t.Fatalf("bundle verify: %v (%s vs %v)", err, b.Hash, resp["bundle_hash"])
	}

	// the branch moves on; the version stays pinned to sha1
	fx.commit(t, map[string]string{"stack/main.tf": "variable \"changed\" {}\n"}, "change")
	b2, _ := manifestbundle.OpenVersion(context.Background(), db, id, v.ID)
	if b2.Hash != b.Hash {
		t.Fatal("published version changed")
	}

	// bundle rules apply to the git tree
	for name, tc := range map[string]struct {
		files map[string]string
		rule  string
	}{
		"unpinned module": {map[string]string{"stack/main.tf": "module \"n\" {\n  source = \"git::https://github.com/acme/mods.git//net?ref=main\"\n}\n"}, manifestbundle.RuleHCLModuleUnpinned},
		"tfvars":          {map[string]string{"stack/main.tf": "# ok\n", "stack/prod.tfvars": "x = 1\n"}, manifestbundle.RuleDenylistedFile},
		"provisioner":     {map[string]string{"stack/prod.tfvars": "", "stack/main.tf": "resource \"null_resource\" \"x\" {\n  provisioner \"local-exec\" {\n    command = \"id\"\n  }\n}\n"}, manifestbundle.RuleHCLProvisioner},
	} {
		s := fx.commit(t, tc.files, name)
		w := doJSON(r, "POST", "/organizations/1/manifests/"+id+"/v2/versions", `{"version":"v9.9.9","commit_sha":"`+s+`"}`)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"rule":"`+tc.rule+`"`) {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	// symlink in the tree
	fx.commit(t, map[string]string{"stack/main.tf": "# ok\n"}, "ok")
	if err := os.Symlink("main.tf", filepath.Join(fx.work, "stack", "link.tf")); err != nil {
		t.Fatal(err)
	}
	s := fx.commit(t, nil, "symlink")
	if w := doJSON(r, "POST", "/organizations/1/manifests/"+id+"/v2/versions", `{"version":"v9.9.9","commit_sha":"`+s+`"}`); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"rule":"git_symlink"`) {
		t.Fatalf("symlink: %d %s", w.Code, w.Body.String())
	}
	// unknown commit
	if w := doJSON(r, "POST", "/organizations/1/manifests/"+id+"/v2/versions", `{"version":"v9.9.9","commit_sha":"`+strings.Repeat("c", 40)+`"}`); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "git_commit_not_found") {
		t.Fatalf("unknown commit: %d %s", w.Code, w.Body.String())
	}
	var nVersions int64
	db.Model(&models.ManifestVersion{}).Where("manifest_id = ?", id).Count(&nVersions)
	if nVersions != 1 {
		t.Fatalf("versions = %d, rejected publishes must not store anything", nVersions)
	}

	// native manifests refuse commit_sha
	if w := doJSON(r, "POST", "/organizations/1/manifests/mf-native/v2/versions", `{"version":"v1.0.0","commit_sha":"`+sha1+`"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("native with commit_sha: %d", w.Code)
	}

	// token hygiene: every minted token was revoked, never in a URL, never logged or returned
	if app.revoked != len(app.minted) {
		t.Fatalf("minted %d, revoked %d", len(app.minted), app.revoked)
	}
	for _, u := range fx.urls {
		if strings.Contains(u, fakeRepoToken) {
			t.Fatalf("token in URL %s", u)
		}
	}
	if strings.Contains(logs.String(), fakeRepoToken) {
		t.Fatal("token logged")
	}
}

func TestGitManifestEditorReadOnlyAndCommitPicker(t *testing.T) {
	db := setupGitDB(t)
	fx := newGitFixture(t)
	app := newFakeApp()
	e := useGitFixture(t, app, fx, "")
	r := gitRouter(db)
	id := createGitManifest(t, r, e, "g1", "")

	if w := doJSON(r, "PUT", "/organizations/1/manifests/"+id+"/files/main.tf", `{"content":"# x"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "git_source_read_only") {
		t.Fatalf("draft write on git manifest: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(r, "GET", "/organizations/1/manifests/"+id+"/git/branches", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"main"`) {
		t.Fatalf("branches: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(r, "GET", "/organizations/1/manifests/"+id+"/git/commits?ref=main", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"subject":"on main"`) {
		t.Fatalf("commits: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(r, "GET", "/organizations/1/manifests/mf-native/git/branches", ""); w.Code != http.StatusConflict {
		t.Fatalf("branches of native: %d", w.Code)
	}
}

func signBody(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestGitHubWebhook(t *testing.T) {
	db := setupGitDB(t)
	fx := newGitFixture(t)
	app := newFakeApp()
	e := useGitFixture(t, app, fx, "whsec")
	r := gitRouter(db)
	id := createGitManifest(t, r, e, "g1", "")
	minted := len(app.minted)

	newSHA := strings.Repeat("d", 40)
	push := []byte(`{"ref":"refs/heads/main","after":"` + newSHA + `","repository":{"html_url":"` + e.WebURL + `/acme/infra"},"installation":{"id":42}}`)
	send := func(body []byte, sig, event string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
		if sig != "" {
			req.Header.Set("X-Hub-Signature-256", sig)
		}
		req.Header.Set("X-GitHub-Event", event)
		r.ServeHTTP(w, req)
		return w
	}
	if w := send(push, "", "push"); w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", w.Code)
	}
	if w := send(push, signBody("wrong", push), "push"); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", w.Code)
	}
	if w := send(push, "sha256=", "push"); w.Code != http.StatusUnauthorized {
		t.Fatalf("empty signature: %d", w.Code)
	}
	if w := send([]byte(`{}`), signBody("whsec", []byte(`{}`)), "ping"); w.Code != http.StatusOK {
		t.Fatalf("ping: %d", w.Code)
	}
	w := send(push, signBody("whsec", push), "push")
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"matched":1`) {
		t.Fatalf("push: %d %s", w.Code, w.Body.String())
	}
	var m models.Manifest
	db.First(&m, "id = ?", id)
	if m.GitLatestSHA == nil || *m.GitLatestSHA != newSHA || m.GitLatestRef == nil || *m.GitLatestRef != "refs/heads/main" || m.GitLatestAt == nil {
		t.Fatalf("latest = %v %v %v", m.GitLatestSHA, m.GitLatestRef, m.GitLatestAt)
	}
	// notification only: nothing published, nothing fetched
	var n int64
	db.Model(&models.ManifestVersion{}).Count(&n)
	if n != 0 || len(app.minted) != minted || len(fx.urls) != 0 {
		t.Fatalf("webhook must not publish or fetch: versions=%d minted=%d urls=%v", n, len(app.minted)-minted, fx.urls)
	}
	// another installation's push for the same repo URL matches nothing
	other := bytes.Replace(push, []byte(`"id":42`), []byte(`"id":77`), 1)
	if w := send(other, signBody("whsec", other), "push"); !strings.Contains(w.Body.String(), `"matched":0`) {
		t.Fatalf("other installation: %s", w.Body.String())
	}

	// no secret configured: refused
	useGitFixture(t, app, fx, "")
	if w := send(push, signBody("whsec", push), "push"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no secret: %d", w.Code)
	}
}

func TestGitHubAppInstallations(t *testing.T) {
	db := setupGitDB(t)
	app := newFakeApp()
	app.accounts[55] = "newco"
	fx := newGitFixture(t)
	e := useGitFixture(t, app, fx, "")
	h := NewGitHubAppHandler(db)
	r := gitRouter(db)
	createGitManifest(t, r, e, "g1", "")
	r.GET("/organizations/:org_id/github-app/installations", withCaller(valueobject.PermissionLevelAdmin), h.ListInstallations)
	r.POST("/organizations/:org_id/github-app/installations", withCaller(valueobject.PermissionLevelAdmin), h.RegisterInstallation)
	r.DELETE("/organizations/:org_id/github-app/installations/:installation_id", withCaller(valueobject.PermissionLevelAdmin), h.DeleteInstallation)

	if w := doJSON(r, "POST", "/organizations/1/github-app/installations", `{"installation_id":55}`); w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"account_login":"newco"`) {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(r, "POST", "/organizations/1/github-app/installations", `{"installation_id":55}`); w.Code != http.StatusOK {
		t.Fatalf("re-register: %d", w.Code)
	}
	if w := doJSON(r, "POST", "/organizations/1/github-app/installations", `{"installation_id":77}`); w.Code != http.StatusConflict {
		t.Fatalf("another org's installation: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(r, "POST", "/organizations/1/github-app/installations", `{"installation_id":999}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown installation: %d", w.Code)
	}
	if w := doJSON(r, "GET", "/organizations/1/github-app/installations", ""); !strings.Contains(w.Body.String(), `"installation_id":42`) || strings.Contains(w.Body.String(), `"installation_id":77`) {
		t.Fatalf("list: %s", w.Body.String())
	}
	if w := doJSON(r, "DELETE", "/organizations/1/github-app/installations/42", ""); w.Code != http.StatusConflict {
		t.Fatalf("delete in use: %d", w.Code)
	}
	if w := doJSON(r, "DELETE", "/organizations/1/github-app/installations/55", ""); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := doJSON(r, "DELETE", "/organizations/1/github-app/installations/77", ""); w.Code != http.StatusNotFound {
		t.Fatalf("delete other org's: %d", w.Code)
	}
}
