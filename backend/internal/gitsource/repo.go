package gitsource

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// Repo one GitHub repository on the configured host.
type Repo struct {
	Owner string
	Name  string
}

// FullName owner/name.
func (r Repo) FullName() string { return r.Owner + "/" + r.Name }

// URL the canonical web URL (also the stored manifests.git_repo_url).
func (r Repo) URL(e Endpoints) string { return e.WebURL + "/" + r.Owner + "/" + r.Name }

// CloneURL the https clone URL. It never carries credentials.
func (r Repo) CloneURL(e Endpoints) string { return r.URL(e) + ".git" }

var (
	ownerRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	nameRe  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	shaRe   = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

// ErrInvalidRepoURL the repo URL is not https://<GITHUB_URL host>/<owner>/<repo>.
var ErrInvalidRepoURL = errors.New("git_repo_url must be <GITHUB_URL>/<owner>/<repo> (https, no credentials, query or fragment)")

// ParseRepoURL accepts <web>/<owner>/<repo>[.git][/] on the configured host
// only, without credentials, query or fragment.
func ParseRepoURL(raw string, e Endpoints) (Repo, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	base, berr := url.Parse(e.WebURL)
	if err != nil || berr != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" ||
		!strings.EqualFold(u.Scheme, base.Scheme) || !strings.EqualFold(u.Host, base.Host) {
		return Repo{}, ErrInvalidRepoURL
	}
	p := strings.TrimSuffix(strings.TrimPrefix(u.Path, strings.TrimRight(base.Path, "/")), "/")
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(parts) != 2 {
		return Repo{}, ErrInvalidRepoURL
	}
	owner, name := parts[0], strings.TrimSuffix(parts[1], ".git")
	if !ownerRe.MatchString(owner) || !nameRe.MatchString(name) || name == "." || name == ".." {
		return Repo{}, ErrInvalidRepoURL
	}
	return Repo{Owner: owner, Name: name}, nil
}

// ErrInvalidRepoName the repo is not "<owner>/<name>".
var ErrInvalidRepoName = errors.New("git_repo must be <owner>/<repo> (a repository of the configured GitHub host)")

// ParseRepoFullName "<owner>/<name>" of a repository on the configured host.
// A host, scheme, port or extra path segment is rejected: the host always
// comes from GITHUB_URL / GITHUB_API_URL, never from a request.
func ParseRepoFullName(s string) (Repo, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 2 {
		return Repo{}, ErrInvalidRepoName
	}
	owner, name := parts[0], parts[1]
	if !ownerRe.MatchString(owner) || !nameRe.MatchString(name) || name == "." || name == ".." || strings.HasSuffix(name, ".git") {
		return Repo{}, ErrInvalidRepoName
	}
	return Repo{Owner: owner, Name: name}, nil
}

// IsCommitSHA a full lowercase SHA-1 / SHA-256 commit id.
func IsCommitSHA(s string) bool { return shaRe.MatchString(s) }

// ErrInvalidSubpath the subpath is not a clean relative directory.
var ErrInvalidSubpath = errors.New("git_subpath must be a relative directory without '.', '..' or empty segments")

// CleanSubpath "" (repo root) or a clean relative POSIX directory.
func CleanSubpath(s string) (string, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" || s == "." {
		return "", nil
	}
	if strings.HasPrefix(s, "/") {
		return "", ErrInvalidSubpath
	}
	if len(s) > 512 || strings.ContainsAny(s, "\\\x00:") || path.Clean(s) != s {
		return "", ErrInvalidSubpath
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == "" || seg == "." || seg == ".." || strings.HasPrefix(seg, "-") {
			return "", ErrInvalidSubpath
		}
		for _, c := range seg {
			if c < 0x20 || c == 0x7f {
				return "", ErrInvalidSubpath
			}
		}
	}
	return s, nil
}
