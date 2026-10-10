package gitsource

import (
	"bytes"
	"context"
	"errors"
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

	"iac-platform/internal/manifestbundle"
)

const testToken = "ghs_TestInstallationToken0123456789abcdef"

// fakeGitServer serves bare repos under root with git http-backend (smart
// HTTP), requiring HTTP basic auth x-access-token:<token>, and records every
// request URL and Authorization header.
type fakeGitServer struct {
	*httptest.Server
	mu    sync.Mutex
	urls  []string
	token string
}

func gitExecPath(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func newFakeGitServer(t *testing.T, root, token string) *fakeGitServer {
	t.Helper()
	backend := &cgi.Handler{
		Path: filepath.Join(gitExecPath(t), "git-http-backend"),
		Env:  []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "GIT_CONFIG_NOSYSTEM=1"},
	}
	s := &fakeGitServer{token: token}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.urls = append(s.urls, r.URL.String())
		s.mu.Unlock()
		user, pass, ok := r.BasicAuth()
		if !ok || user != "x-access-token" || pass != s.token {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "auth required", http.StatusUnauthorized)
			return
		}
		backend.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func gitT(t *testing.T, dir string, args ...string) string {
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

type testRepo struct {
	work, bare string
}

// newTestRepo: <root>/acme/infra.git bare repo plus a work tree pushing to it.
func newTestRepo(t *testing.T, root string) *testRepo {
	t.Helper()
	bare := filepath.Join(root, "acme", "infra.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, bare, "init", "-q", "--bare", "-b", "main")
	gitT(t, bare, "config", "uploadpack.allowAnySHA1InWant", "true")
	gitT(t, bare, "config", "http.receivepack", "false")
	work := t.TempDir()
	gitT(t, work, "init", "-q", "-b", "main")
	return &testRepo{work: work, bare: bare}
}

func (r *testRepo) write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(r.work, path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func (r *testRepo) commit(t *testing.T, msg string) string {
	t.Helper()
	gitT(t, r.work, "add", "-A")
	return r.commitIndex(t, msg)
}

func (r *testRepo) commitIndex(t *testing.T, msg string) string {
	t.Helper()
	gitT(t, r.work, "commit", "-q", "--allow-empty", "-m", msg)
	gitT(t, r.work, "push", "-q", r.bare, "HEAD:refs/heads/main")
	return gitT(t, r.work, "rev-parse", "HEAD")
}

func setupFetch(t *testing.T) (*Fetcher, *fakeGitServer, *testRepo, Repo) {
	t.Helper()
	root := t.TempDir()
	srv := newFakeGitServer(t, root, testToken)
	repo := newTestRepo(t, root)
	f := &Fetcher{Endpoints: Endpoints{WebURL: srv.URL, APIURL: srv.URL + "/api"}, Timeout: time.Minute}
	return f, srv, repo, Repo{Owner: "acme", Name: "infra"}
}

func files(tree *Tree) map[string]string {
	m := map[string]string{}
	for _, f := range tree.Files {
		m[f.Path] = string(f.Content)
	}
	return m
}

func TestFetch_PinsToSHA(t *testing.T) {
	f, srv, repo, r := setupFetch(t)
	repo.write(t, "main.tf", "# v1\n", 0o644)
	sha1 := repo.commit(t, "first\n\nbody")
	repo.write(t, "main.tf", "# v2\n", 0o644)
	repo.write(t, "run.sh", "#!/bin/sh\n", 0o755)
	sha2 := repo.commit(t, "second")

	tree, err := f.Fetch(context.Background(), r, sha1, "", NewToken(testToken, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if tree.SHA != sha1 || tree.Subject != "first" || len(tree.Problems) != 0 {
		t.Fatalf("tree = %+v", tree)
	}
	if got := files(tree); len(got) != 1 || got["main.tf"] != "# v1\n" {
		t.Fatalf("files at sha1 = %v", got)
	}
	tree, err = f.Fetch(context.Background(), r, sha2, "", NewToken(testToken, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	got := files(tree)
	if got["main.tf"] != "# v2\n" || got["run.sh"] != "#!/bin/sh\n" {
		t.Fatalf("files at sha2 = %v", got)
	}
	for _, fl := range tree.Files {
		if fl.Path == "run.sh" && fl.Mode != manifestbundle.ModeExecutable {
			t.Fatalf("run.sh mode = %o", fl.Mode)
		}
	}
	// the bundle hash is a function of the pinned commit only
	h1, _ := manifestbundle.Hash(tree.Files)
	tree2, err := f.Fetch(context.Background(), r, sha2, "", NewToken(testToken, time.Now().Add(time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if h2, _ := manifestbundle.Hash(tree2.Files); h1 != h2 {
		t.Fatal("same SHA, different bundle hash")
	}
	for _, u := range srv.urls {
		if strings.Contains(u, testToken) || strings.Contains(u, "x-access-token") {
			t.Fatalf("token in request URL %s", u)
		}
	}

	// abbreviated / ref names are refused: only full SHAs pin
	for _, bad := range []string{"main", sha1[:12], strings.ToUpper(sha1), "HEAD"} {
		if _, err := f.Fetch(context.Background(), r, bad, "", NewToken(testToken, time.Now())); err == nil {
			t.Fatalf("Fetch(%q) accepted", bad)
		}
	}
}

func TestFetch_UnknownCommitAndBadToken(t *testing.T) {
	f, _, repo, r := setupFetch(t)
	repo.write(t, "main.tf", "x\n", 0o644)
	sha := repo.commit(t, "c")

	_, err := f.Fetch(context.Background(), r, strings.Repeat("a", 40), "", NewToken(testToken, time.Now()))
	if !errors.Is(err, ErrCommitNotFound) {
		t.Fatalf("unknown sha err = %v", err)
	}
	wrong := "ghs_WrongToken_DoNotLeak_0123456789"
	_, err = f.Fetch(context.Background(), r, sha, "", NewToken(wrong, time.Now()))
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("bad token err = %v", err)
	}
	if strings.Contains(err.Error(), wrong) {
		t.Fatalf("token in error: %v", err)
	}
}

func TestFetch_SubpathSymlinkSubmodule(t *testing.T) {
	f, _, repo, r := setupFetch(t)
	repo.write(t, "envs/prod/main.tf", "# prod\n", 0o644)
	repo.write(t, "envs/prod/modules/net/main.tf", "# net\n", 0o644)
	repo.write(t, "README.md", "top\n", 0o644)
	if err := os.Symlink("main.tf", filepath.Join(repo.work, "envs/prod/link.tf")); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo.work, "add", "-A")
	gitT(t, repo.work, "update-index", "--add", "--cacheinfo", "160000,"+strings.Repeat("b", 40)+",envs/prod/vendor/sub")
	sha := repo.commitIndex(t, "tree with specials")

	tree, err := f.Fetch(context.Background(), r, sha, "envs/prod", NewToken(testToken, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	got := files(tree)
	if len(got) != 2 || got["main.tf"] != "# prod\n" || got["modules/net/main.tf"] != "# net\n" {
		t.Fatalf("files = %v", got)
	}
	var rules []string
	for _, p := range tree.Problems {
		rules = append(rules, p.Rule+"@"+p.File)
	}
	if strings.Join(rules, ",") != "git_symlink@link.tf,git_submodule@vendor/sub" {
		t.Fatalf("problems = %v", rules)
	}

	if _, err := f.Fetch(context.Background(), r, sha, "envs/dev", NewToken(testToken, time.Now())); !errors.Is(err, ErrSubpathNotFound) {
		t.Fatalf("missing subpath err = %v", err)
	}
	for _, bad := range []string{"../x", "/abs", "a/../b", "-c"} {
		if _, err := f.Fetch(context.Background(), r, sha, bad, NewToken(testToken, time.Now())); !errors.Is(err, ErrInvalidSubpath) {
			t.Fatalf("subpath %q err = %v", bad, err)
		}
	}
}

// The token reaches git only through the environment (GIT_ASKPASS): never
// argv, and it is scrubbed from every error and never logged.
func TestFetch_TokenNeverInArgvLogsOrErrors(t *testing.T) {
	f, _, repo, r := setupFetch(t)
	repo.write(t, "main.tf", "x\n", 0o644)
	sha := repo.commit(t, "c")

	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	wrapper := filepath.Join(dir, "git-wrapper.sh")
	realGit, _ := exec.LookPath("git")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argvLog+"\nexec "+realGit+" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.GitPath = wrapper

	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	if _, err := f.Fetch(context.Background(), r, sha, "", NewToken(testToken, time.Now())); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(argvLog)
	if !strings.Contains(string(argv), "fetch") {
		t.Fatalf("wrapper not used: %s", argv)
	}
	if strings.Contains(string(argv), testToken) || strings.Contains(string(argv), "x-access-token") {
		t.Fatalf("token in argv:\n%s", argv)
	}
	if strings.Contains(logs.String(), testToken) {
		t.Fatal("token logged")
	}

	// a git that echoes the token on failure: the error is scrubbed
	evil := filepath.Join(dir, "git-evil.sh")
	if err := os.WriteFile(evil, []byte("#!/bin/sh\necho \"fatal: bad credentials $TERRANOVA_GIT_TOKEN\" >&2\nexit 128\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.GitPath = evil
	_, err := f.Fetch(context.Background(), r, sha, "", NewToken(testToken, time.Now()))
	if err == nil || strings.Contains(err.Error(), testToken) || !strings.Contains(err.Error(), "***") {
		t.Fatalf("err = %v", err)
	}

	tok := NewToken(testToken, time.Now())
	for _, s := range []string{tok.String(), tok.GoString(), sprintf("%v %+v %s %#v %q", tok, tok, tok, tok, tok), sprintf("%v", *tok)} {
		if strings.Contains(s, testToken) {
			t.Fatalf("token formatted: %s", s)
		}
	}
}
