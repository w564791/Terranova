package gitsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token a GitHub App installation token: contents:read on one repo, about
// one hour. It is never stored or logged: String / GoString / MarshalJSON
// print "***"; only Secret() returns the value, for the GIT_ASKPASS env var
// and the Authorization header.
type Token struct {
	value     string
	ExpiresAt time.Time
}

// NewToken wraps a token value (minters and tests).
func NewToken(value string, expiresAt time.Time) *Token {
	return &Token{value: value, ExpiresAt: expiresAt}
}

// Secret the raw value. Only for the git process environment and the
// Authorization header; never for URLs, argv, logs or errors.
func (t *Token) Secret() string {
	if t == nil {
		return ""
	}
	return t.value
}

func (Token) String() string               { return "***" }
func (Token) GoString() string             { return "***" }
func (Token) MarshalJSON() ([]byte, error) { return []byte(`"***"`), nil }

// Format keeps every verb (%v, %+v, %#v, %s, %q, ...) from printing the value.
func (Token) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte("***")) }

// TokenMinter mints a per-operation installation token for one repo.
type TokenMinter interface {
	MintRepoToken(ctx context.Context, installationID int64, repo Repo) (*Token, error)
}

// Installation what GitHub says about an installation.
type Installation struct {
	ID           int64  `json:"id"`
	AccountID    int64  `json:"account_id"`
	AccountLogin string `json:"account_login"`
	AccountType  string `json:"account_type"` // "Organization" or "User"
}

// InstallationLookup resolves an installation id with the App JWT (used when
// an org admin registers an installation).
type InstallationLookup interface {
	GetInstallation(ctx context.Context, installationID int64) (*Installation, error)
}

// GitHubApp mints installation tokens with the App credentials. HTTP may be
// nil (a client with a 30s timeout and no redirects is used).
type GitHubApp struct {
	Creds     CredentialsProvider
	Endpoints Endpoints
	HTTP      *http.Client
	Now       func() time.Time
}

// NewGitHubApp from the environment; ErrDisabled when the App is not
// configured.
func NewGitHubApp() (*GitHubApp, error) {
	if _, err := Credentials.AppCredentials(); err != nil {
		return nil, err
	}
	e, err := EndpointsFromEnv()
	if err != nil {
		return nil, err
	}
	return &GitHubApp{Creds: Credentials, Endpoints: e}, nil
}

func noRedirectClient() *http.Client {
	return &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (a *GitHubApp) client() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return noRedirectClient()
}

func (a *GitHubApp) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// appJWT the App's own short-lived JWT (RS256, 9 minutes, backdated 60s).
func (a *GitHubApp) appJWT() (string, error) {
	c, err := a.Creds.AppCredentials()
	if err != nil {
		return "", err
	}
	now := a.now()
	return jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    fmt.Sprint(c.AppID),
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}).SignedString(c.PrivateKey)
}

// APIError a GitHub API call failed. The message never contains a token.
type APIError struct {
	Op     string
	Status int
}

func (e *APIError) Error() string { return fmt.Sprintf("github %s: HTTP %d", e.Op, e.Status) }

// ErrNotFound the installation / repo / ref does not exist or is not visible
// to the App.
var ErrNotFound = errors.New("not found or not accessible to the GitHub App")

func (a *GitHubApp) do(ctx context.Context, op, method, url, bearer string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return fmt.Errorf("github %s: %w", op, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+bearer)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client().Do(req)
	if err != nil {
		// url.Error carries only the URL (no credentials there)
		return fmt.Errorf("github %s: request failed", op)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("github %s: %w", op, ErrNotFound)
	}
	if resp.StatusCode/100 != 2 {
		return &APIError{Op: op, Status: resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("github %s: invalid response", op)
	}
	return nil
}

// MintRepoToken POST /app/installations/{id}/access_tokens restricted to the
// one repository and contents:read. The response must not grant more.
func (a *GitHubApp) MintRepoToken(ctx context.Context, installationID int64, repo Repo) (*Token, error) {
	appTok, err := a.appJWT()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Token        string            `json:"token"`
		ExpiresAt    time.Time         `json:"expires_at"`
		Permissions  map[string]string `json:"permissions"`
		Repositories []struct {
			FullName string `json:"full_name"`
		} `json:"repositories"`
	}
	err = a.do(ctx, "mint installation token", http.MethodPost,
		fmt.Sprintf("%s/app/installations/%d/access_tokens", a.Endpoints.APIURL, installationID), appTok,
		map[string]any{"repositories": []string{repo.Name}, "permissions": map[string]string{"contents": "read"}}, &resp)
	if err != nil {
		return nil, err
	}
	tok := NewToken(resp.Token, resp.ExpiresAt)
	if resp.Token == "" {
		return nil, errors.New("github mint installation token: empty token")
	}
	for p, level := range resp.Permissions {
		if level != "read" || (p != "contents" && p != "metadata") {
			a.revoke(ctx, tok)
			return nil, fmt.Errorf("github mint installation token: unexpected permission %s:%s", p, level)
		}
	}
	if len(resp.Repositories) != 1 || !strings.EqualFold(resp.Repositories[0].FullName, repo.FullName()) {
		a.revoke(ctx, tok)
		return nil, errors.New("github mint installation token: token is not scoped to the single repository")
	}
	return tok, nil
}

// Revoke DELETE /installation/token (best effort; the token expires anyway).
func (a *GitHubApp) Revoke(ctx context.Context, t *Token) { a.revoke(ctx, t) }

func (a *GitHubApp) revoke(ctx context.Context, t *Token) {
	if t == nil || t.value == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = a.do(ctx, "revoke installation token", http.MethodDelete, a.Endpoints.APIURL+"/installation/token", t.value, nil, nil)
}

// GetInstallation GET /app/installations/{id} (App JWT).
func (a *GitHubApp) GetInstallation(ctx context.Context, installationID int64) (*Installation, error) {
	appTok, err := a.appJWT()
	if err != nil {
		return nil, err
	}
	var resp struct {
		ID      int64 `json:"id"`
		Account struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
	}
	if err := a.do(ctx, "get installation", http.MethodGet,
		fmt.Sprintf("%s/app/installations/%d", a.Endpoints.APIURL, installationID), appTok, nil, &resp); err != nil {
		return nil, err
	}
	return &Installation{ID: resp.ID, AccountID: resp.Account.ID, AccountLogin: resp.Account.Login, AccountType: resp.Account.Type}, nil
}

// AppInfo the App itself (GET /app with the App JWT).
type AppInfo struct {
	Slug    string `json:"slug"`
	HTMLURL string `json:"html_url"`
}

// GetApp GET /app (App JWT).
func (a *GitHubApp) GetApp(ctx context.Context) (*AppInfo, error) {
	appTok, err := a.appJWT()
	if err != nil {
		return nil, err
	}
	var info AppInfo
	if err := a.do(ctx, "get app", http.MethodGet, a.Endpoints.APIURL+"/app", appTok, nil, &info); err != nil {
		return nil, err
	}
	if info.HTMLURL == "" {
		return nil, errors.New("github get app: invalid response")
	}
	return &info, nil
}

// InstallURL the page that installs the App, carrying state (GitHub hands it
// back to the callback). The App's html_url must be on the web host.
func InstallURL(info *AppInfo, e Endpoints, state string) (string, error) {
	u, err := url.Parse(info.HTMLURL)
	w, werr := url.Parse(e.WebURL)
	if err != nil || werr != nil || u.Scheme != w.Scheme || !strings.EqualFold(u.Host, w.Host) || u.User != nil || u.RawQuery != "" {
		return "", errors.New("github get app: html_url is not on the GitHub host")
	}
	return strings.TrimRight(info.HTMLURL, "/") + "/installations/new?state=" + url.QueryEscape(state), nil
}

// MintMetadataToken an installation token with only metadata:read (no
// contents), used to list the installation's repositories. The response must
// not grant more; revoke it right after use.
func (a *GitHubApp) MintMetadataToken(ctx context.Context, installationID int64) (*Token, error) {
	appTok, err := a.appJWT()
	if err != nil {
		return nil, err
	}
	var resp struct {
		Token       string            `json:"token"`
		ExpiresAt   time.Time         `json:"expires_at"`
		Permissions map[string]string `json:"permissions"`
	}
	err = a.do(ctx, "mint metadata token", http.MethodPost,
		fmt.Sprintf("%s/app/installations/%d/access_tokens", a.Endpoints.APIURL, installationID), appTok,
		map[string]any{"permissions": map[string]string{"metadata": "read"}}, &resp)
	if err != nil {
		return nil, err
	}
	if resp.Token == "" {
		return nil, errors.New("github mint metadata token: empty token")
	}
	tok := NewToken(resp.Token, resp.ExpiresAt)
	for p, level := range resp.Permissions {
		if p != "metadata" || level != "read" {
			a.revoke(ctx, tok)
			return nil, fmt.Errorf("github mint metadata token: unexpected permission %s:%s", p, level)
		}
	}
	return tok, nil
}

// RepoInfo a repository of an installation. HTMLURL is built from the
// configured GITHUB_URL, never taken from the API response.
type RepoInfo struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	HTMLURL       string `json:"html_url"`
}

// ListInstallationRepos GET /installation/repositories (installation token),
// one page. Entries whose full_name is not a valid owner/name are dropped.
func (a *GitHubApp) ListInstallationRepos(ctx context.Context, t *Token, page, perPage int) ([]RepoInfo, int, error) {
	if perPage <= 0 || perPage > 100 {
		perPage = 30
	}
	if page <= 0 {
		page = 1
	}
	var resp struct {
		TotalCount   int `json:"total_count"`
		Repositories []struct {
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
			Private       bool   `json:"private"`
		} `json:"repositories"`
	}
	if err := a.do(ctx, "list installation repositories", http.MethodGet,
		fmt.Sprintf("%s/installation/repositories?per_page=%d&page=%d", a.Endpoints.APIURL, perPage, page), t.Secret(), nil, &resp); err != nil {
		return nil, 0, err
	}
	out := make([]RepoInfo, 0, len(resp.Repositories))
	for _, r := range resp.Repositories {
		repo, err := ParseRepoFullName(r.FullName)
		if err != nil {
			continue
		}
		out = append(out, RepoInfo{FullName: repo.FullName(), DefaultBranch: r.DefaultBranch, Private: r.Private, HTMLURL: repo.URL(a.Endpoints)})
	}
	return out, resp.TotalCount, nil
}

// Branch a branch head.
type Branch struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}

// Commit a commit for the picker (subject = first line of the message).
type Commit struct {
	SHA        string    `json:"sha"`
	Subject    string    `json:"subject"`
	AuthorName string    `json:"author_name"`
	AuthorDate time.Time `json:"author_date"`
}

// ListBranches GET /repos/{o}/{r}/branches (first 100).
func (a *GitHubApp) ListBranches(ctx context.Context, repo Repo, t *Token) ([]Branch, error) {
	var resp []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := a.do(ctx, "list branches", http.MethodGet,
		fmt.Sprintf("%s/repos/%s/%s/branches?per_page=100", a.Endpoints.APIURL, repo.Owner, repo.Name), t.Secret(), nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Branch, 0, len(resp))
	for _, b := range resp {
		out = append(out, Branch{Name: b.Name, SHA: b.Commit.SHA})
	}
	return out, nil
}

// ListCommits GET /repos/{o}/{r}/commits?sha=<ref> (newest first, <= 100).
func (a *GitHubApp) ListCommits(ctx context.Context, repo Repo, ref string, perPage int, t *Token) ([]Commit, error) {
	if perPage <= 0 || perPage > 100 {
		perPage = 30
	}
	u := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d", a.Endpoints.APIURL, repo.Owner, repo.Name, perPage)
	if ref != "" {
		u += "&sha=" + urlQueryEscape(ref)
	}
	var resp []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := a.do(ctx, "list commits", http.MethodGet, u, t.Secret(), nil, &resp); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(resp))
	for _, c := range resp {
		out = append(out, Commit{SHA: c.SHA, Subject: Subject(c.Commit.Message), AuthorName: c.Commit.Author.Name, AuthorDate: c.Commit.Author.Date})
	}
	return out, nil
}

// Subject first line of a commit message, at most 200 bytes.
func Subject(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	msg = strings.TrimSpace(msg)
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}
