package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iac-platform/internal/gitsource"
	"iac-platform/internal/models"
)

// connect returns github_url from GITHUB_URL only; request headers / query
// parameters naming another host are ignored.
func TestGitHubAppConnectReturnsConfiguredGitHubURL(t *testing.T) {
	s := newSetupEnv(t)
	req := httptest.NewRequest("POST", "/organizations/1/github-app/connect?github_url=https://evil.example.com", strings.NewReader(`{"github_url":"https://evil.example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Host", "evil.example.com")
	req.Host = "evil.example.com"
	w := httptest.NewRecorder()
	s.r.ServeHTTP(w, req)
	var resp struct {
		InstallURL string `json:"install_url"`
		GitHubURL  string `json:"github_url"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusOK || resp.GitHubURL != s.e.WebURL || !strings.HasPrefix(resp.InstallURL, s.e.WebURL+"/") {
		t.Fatalf("connect: %d %s (want github_url %s)", w.Code, w.Body.String(), s.e.WebURL)
	}
	if strings.Contains(w.Body.String(), "evil") {
		t.Fatal("request-supplied host reflected")
	}
}

type repoPage struct {
	Repositories []gitsource.RepoInfo `json:"repositories"`
	TotalCount   int                  `json:"total_count"`
	Page         int                  `json:"page"`
	PerPage      int                  `json:"per_page"`
	Q            string               `json:"q"`
	Truncated    bool                 `json:"truncated"`
}

func getRepos(t *testing.T, s *setupEnv, query string) (int, repoPage, string) {
	t.Helper()
	w := doJSON(s.r, "GET", "/organizations/1/github-app/installations/42/repositories?"+query, "")
	var p repoPage
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	return w.Code, p, w.Body.String()
}

func names(p repoPage) []string {
	var out []string
	for _, r := range p.Repositories {
		out = append(out, r.FullName)
	}
	return out
}

func TestGitHubAppRepositorySearch(t *testing.T) {
	s := newSetupEnv(t)
	for _, n := range []string{"acme/Infra-Live", "acme/web", "acme/infra-modules", "acme/docs"} {
		s.app.repos[n] = true
	}

	// case-insensitive substring of full_name, trimmed, paged over matches
	minted, revoked := len(s.app.minted), s.app.revoked
	code, p, body := getRepos(t, s, "q=%20%20INFRA%20&per_page=2&page=1")
	if code != http.StatusOK || p.TotalCount != 3 || p.Q != "INFRA" || p.Truncated ||
		strings.Join(names(p), ",") != "acme/Infra-Live,acme/infra" {
		t.Fatalf("search page 1: %d %s", code, body)
	}
	if got := s.app.minted[minted:]; len(got) != 1 || got[0] != "42 metadata" || s.app.revoked != revoked+1 {
		t.Fatalf("want one metadata token minted and revoked: %v revoked %d", got, s.app.revoked-revoked)
	}
	if _, p, body = getRepos(t, s, "q=infra&per_page=2&page=2"); strings.Join(names(p), ",") != "acme/infra-modules" || p.TotalCount != 3 {
		t.Fatalf("search page 2: %s", body)
	}
	if _, p, body = getRepos(t, s, "q=infra&per_page=2&page=9"); len(p.Repositories) != 0 || p.TotalCount != 3 || !strings.Contains(body, `"repositories":[]`) {
		t.Fatalf("search past the end: %s", body)
	}
	if _, p, body = getRepos(t, s, "q=nomatch"); len(p.Repositories) != 0 || p.TotalCount != 0 {
		t.Fatalf("no match: %s", body)
	}
	// blank q = plain listing
	if _, p, body = getRepos(t, s, "q=%20%20&per_page=100"); p.TotalCount != 5 || p.Q != "" {
		t.Fatalf("blank q: %s", body)
	}

	// length limit: 100 characters after trimming (runes, not bytes)
	ok100 := strings.Repeat("é", 100)
	if code, _, body := getRepos(t, s, "q=%20"+ok100+"%20"); code != http.StatusOK {
		t.Fatalf("100 characters: %d %s", code, body)
	}
	minted = len(s.app.minted)
	if code, _, body := getRepos(t, s, "q="+strings.Repeat("a", 101)); code != http.StatusBadRequest || !strings.Contains(body, "repo_query_too_long") {
		t.Fatalf("101 characters: %d %s", code, body)
	}
	if len(s.app.minted) != minted {
		t.Fatal("token minted for a rejected query")
	}
	// another org's installation stays 404 with q
	if w := doJSON(s.r, "GET", "/org2/github-app/installations/42/repositories?q=infra", ""); w.Code != http.StatusNotFound {
		t.Fatalf("cross-org search: %d", w.Code)
	}
}

func TestGitHubAppRepositorySearchScanCap(t *testing.T) {
	s := newSetupEnv(t)
	for i := 0; i < 1050; i++ {
		s.app.repos[fmt.Sprintf("acme/r%04d", i)] = true
	}
	// sorted: acme/infra, acme/r0000..acme/r1049; the first 1000 (up to r0998) are scanned
	code, p, body := getRepos(t, s, "q=r09&per_page=100")
	if code != http.StatusOK || !p.Truncated || p.TotalCount != 99 {
		t.Fatalf("within cap: %d truncated=%v total=%d", code, p.Truncated, p.TotalCount)
	}
	if _, p, _ = getRepos(t, s, "q=r104"); !p.Truncated || p.TotalCount != 0 {
		t.Fatalf("beyond cap must not be scanned: %s", body)
	}
	s2 := newSetupEnv(t)
	if _, p, _ = getRepos(t, s2, "q=infra"); p.Truncated {
		t.Fatal("small installation reported truncated")
	}
}

func TestGitManifestSubpathValidationAndImmutable(t *testing.T) {
	s := newSetupEnv(t)
	n := 0
	create := func(subpath string) *httptest.ResponseRecorder {
		n++
		raw, _ := json.Marshal(map[string]any{"name": fmt.Sprintf("sp-%d", n), "source_type": "git",
			"git_repo": "acme/infra", "github_installation_id": 42, "git_subpath": subpath})
		return doJSON(s.r, "POST", "/organizations/1/manifests", string(raw))
	}
	minted := len(s.app.minted)
	for _, bad := range []string{"/envs/prod", "../envs", "envs/../../x", "envs/./prod", "./envs", "envs//prod",
		`envs\prod`, "e\u0301nvs", "envs/\x01", "/"} {
		w := create(bad)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), gitCodeSubpathInvalid) {
			t.Fatalf("subpath %q: %d %s", bad, w.Code, w.Body.String())
		}
	}
	if len(s.app.minted) != minted {
		t.Fatal("token minted for an invalid subpath")
	}
	for in, want := range map[string]string{"": "", ".": "", "  envs/prod/ ": "envs/prod", "envs/\u00e9t\u00e9": "envs/\u00e9t\u00e9"} {
		w := create(in)
		var m models.Manifest
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		got := ""
		if m.GitSubpath != nil {
			got = *m.GitSubpath
		}
		if w.Code != http.StatusCreated || got != want {
			t.Fatalf("subpath %q: %d %s (want %q)", in, w.Code, w.Body.String(), want)
		}
	}

	w := create("envs/prod")
	var m models.Manifest
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	put := func(id, body string) *httptest.ResponseRecorder {
		return doJSON(s.r, "PUT", "/organizations/1/manifests/"+id, body)
	}
	for _, tc := range []struct {
		body string
		code int
		want string
	}{
		{`{"git_subpath":"envs/other"}`, http.StatusConflict, gitCodeSourceImmutable},
		{`{"git_subpath":""}`, http.StatusConflict, gitCodeSourceImmutable},
		{`{"git_subpath":"../envs"}`, http.StatusBadRequest, gitCodeSubpathInvalid},
		{`{"git_repo":"acme/other"}`, http.StatusConflict, gitCodeSourceImmutable},
		{`{"git_repo_url":"https://evil.example.com/acme/infra"}`, http.StatusConflict, gitCodeSourceImmutable},
		{`{"github_installation_id":77}`, http.StatusConflict, gitCodeSourceImmutable},
		{`{"name":"renamed","git_subpath":"envs/prod/"}`, http.StatusOK, ""}, // echo (normalized) is fine
		{fmt.Sprintf(`{"git_repo":"ACME/infra","git_repo_url":%q,"github_installation_id":42}`, s.e.WebURL+"/acme/infra"), http.StatusOK, ""},
	} {
		w := put(m.ID, tc.body)
		if w.Code != tc.code || (tc.want != "" && !strings.Contains(w.Body.String(), tc.want)) {
			t.Fatalf("PUT %s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	var stored models.Manifest
	if err := s.db.Where("id = ?", m.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.GitSubpath == nil || *stored.GitSubpath != "envs/prod" || *stored.GitRepoURL != s.e.WebURL+"/acme/infra" ||
		*stored.GitHubInstallationID != 42 || stored.Name != "renamed" {
		t.Fatalf("stored git source changed: %+v", stored)
	}

	// native manifests have no git source to set
	w = doJSON(s.r, "POST", "/organizations/1/manifests", `{"name":"native-1"}`)
	var nm models.Manifest
	_ = json.Unmarshal(w.Body.Bytes(), &nm)
	if w.Code != http.StatusCreated {
		t.Fatalf("native create: %d %s", w.Code, w.Body.String())
	}
	if w := put(nm.ID, `{"git_subpath":"envs"}`); w.Code != http.StatusConflict {
		t.Fatalf("native git_subpath: %d %s", w.Code, w.Body.String())
	}
	if w := put(nm.ID, `{"git_repo":"acme/infra"}`); w.Code != http.StatusConflict {
		t.Fatalf("native git_repo: %d %s", w.Code, w.Body.String())
	}
	if w := put(nm.ID, `{"git_subpath":"","description":"d"}`); w.Code != http.StatusOK {
		t.Fatalf("native empty echo: %d %s", w.Code, w.Body.String())
	}
}
