package gitsource

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/manifestbundle"
)

// Fetcher pulls the tree of one pinned commit with the git CLI, platform
// side, at publish time. Nothing is checked out: the tree is listed with
// ls-tree and blobs are read with cat-file, so no hooks, filters (LFS
// smudge), symlinks or submodules are ever materialized.
type Fetcher struct {
	Endpoints Endpoints
	GitPath   string        // default "git"
	Timeout   time.Duration // default 2 minutes
}

// Tree the files of a commit (under the manifest's subpath, paths relative
// to it) plus problems found before content was read.
type Tree struct {
	SHA     string
	Subject string // first line of the commit message
	// Files regular files (mode normalized by ModeFromGit); Mime / IsBinary
	// are left to the caller.
	Files []manifestbundle.File
	// Problems tree entries rejected without reading them: git_symlink,
	// git_submodule, file_too_large, bundle_too_large, too_many_files. The
	// caller merges them with ValidateForPublish(Files).
	Problems []manifestbundle.Problem
}

var (
	// ErrCommitNotFound the SHA is not in the repository (or not reachable).
	ErrCommitNotFound = errors.New("commit not found in the repository")
	// ErrAuth the repository rejected the installation token.
	ErrAuth = errors.New("repository access denied for the GitHub App installation")
	// ErrSubpathNotFound git_subpath is not a directory of the commit.
	ErrSubpathNotFound = errors.New("git_subpath does not exist in the commit")
)

// GitError a git command failed; Detail is git's stderr with the token
// scrubbed (and truncated).
type GitError struct {
	Op     string
	Detail string
}

func (e *GitError) Error() string { return "git " + e.Op + " failed: " + e.Detail }

// askpassScript prints the user name / the token from the environment of the
// git process (never argv, never a file holding the token).
const askpassScript = `#!/bin/sh
case "$1" in
  Username*|username*) printf '%s\n' x-access-token ;;
  *) printf '%s\n' "$TERRANOVA_GIT_TOKEN" ;;
esac
`

type gitRunner struct {
	git   string
	dir   string
	env   []string
	token string
}

// Scrub replaces every occurrence of secret in s (and its URL-encoded form)
// with "***".
func Scrub(s, secret string) string {
	if secret == "" {
		return s
	}
	s = strings.ReplaceAll(s, secret, "***")
	return strings.ReplaceAll(s, urlQueryEscape(secret), "***")
}

func (g *gitRunner) run(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, g.git, args...)
	cmd.Dir = g.dir
	cmd.Env = g.env
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedBuffer{buf: &stderr, max: 64 << 10}
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(Scrub(stderr.String(), g.token))
		if ctx.Err() != nil {
			detail = "timed out"
		}
		if len(detail) > 500 {
			detail = detail[:500] + "..."
		}
		if detail == "" {
			detail = Scrub(err.Error(), g.token)
		}
		op := args[0]
		for _, a := range args {
			if !strings.HasPrefix(a, "-") && !strings.Contains(a, "=") {
				op = a
				break
			}
		}
		return nil, &GitError{Op: op, Detail: detail}
	}
	return stdout.Bytes(), nil
}

type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

// Fetch the commit sha of repo with token (may be nil for a public repo) and
// return its tree under subpath.
func (f *Fetcher) Fetch(ctx context.Context, repo Repo, sha, subpath string, token *Token) (*Tree, error) {
	if !IsCommitSHA(sha) {
		return nil, errors.New("commit_sha must be a full lowercase 40 (SHA-1) or 64 (SHA-256) hex commit id")
	}
	subpath, err := CleanSubpath(subpath)
	if err != nil {
		return nil, err
	}
	timeout := f.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tmp, err := os.MkdirTemp("", "manifest-git-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	askpass := filepath.Join(tmp, "askpass.sh")
	if err := os.WriteFile(askpass, []byte(askpassScript), 0o700); err != nil {
		return nil, err
	}
	repoDir := filepath.Join(tmp, "repo.git")

	protocols := "https"
	if strings.HasPrefix(f.Endpoints.WebURL, "http://") {
		protocols = "http" // explicitly configured plain-http server (tests / lab)
	}
	git := f.GitPath
	if git == "" {
		git = "git"
	}
	g := &gitRunner{git: git, dir: tmp, token: token.Secret(), env: []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + tmp,
		"LC_ALL=C",
		"GIT_ASKPASS=" + askpass,
		"TERRANOVA_GIT_TOKEN=" + token.Secret(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_ALLOW_PROTOCOL=" + protocols,
		"GIT_LFS_SKIP_SMUDGE=1",
	}}
	safety := []string{
		"-c", "credential.helper=",
		"-c", "core.hooksPath=/dev/null",
		"-c", "http.followRedirects=false",
		"-c", "protocol.allow=never",
		"-c", "protocol." + protocols + ".allow=always",
		"-c", "fetch.recurseSubmodules=false",
		"-c", "core.fsmonitor=false",
	}
	if _, err := g.run(ctx, nil, append(safety, "init", "-q", "--bare", repoDir)...); err != nil {
		return nil, err
	}
	g.dir = repoDir
	if _, err := g.run(ctx, nil, append(safety, "fetch", "-q", "--no-tags", "--no-recurse-submodules",
		"--depth=1", repo.CloneURL(f.Endpoints), sha)...); err != nil {
		var ge *GitError
		if errors.As(err, &ge) {
			d := strings.ToLower(ge.Detail)
			switch {
			case strings.Contains(d, "authentication failed") || strings.Contains(d, "could not read username") ||
				strings.Contains(d, "403") || strings.Contains(d, "401"):
				return nil, fmt.Errorf("%w: %s", ErrAuth, ge.Detail)
			case strings.Contains(d, "not our ref") || strings.Contains(d, "couldn't find remote ref") ||
				strings.Contains(d, "unadvertised object") || strings.Contains(d, "no such remote ref") ||
				strings.Contains(d, "not found"):
				return nil, fmt.Errorf("%w: %s", ErrCommitNotFound, ge.Detail)
			}
		}
		return nil, err
	}
	got, err := g.run(ctx, nil, "rev-parse", "--verify", "--end-of-options", sha+"^{commit}")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCommitNotFound, err)
	}
	if strings.TrimSpace(string(got)) != sha {
		return nil, fmt.Errorf("%w: fetched object is not commit %s", ErrCommitNotFound, sha)
	}
	msg, err := g.run(ctx, nil, "log", "-1", "--format=%B", sha)
	if err != nil {
		return nil, err
	}
	listing, err := g.run(ctx, nil, "ls-tree", "-r", "-z", "-l", "--full-tree", sha)
	if err != nil {
		return nil, err
	}
	tree := &Tree{SHA: sha, Subject: Subject(string(msg))}

	type entry struct {
		path, oid string
		mode      int
	}
	var (
		entries []entry
		total   int64
		found   = subpath == ""
	)
	type raw struct{ mode, typ, oid, size, path string }
	var rows []raw
	for _, rec := range bytes.Split(listing, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		meta, p, ok := bytes.Cut(rec, []byte{'\t'})
		if !ok {
			return nil, errors.New("git ls-tree: malformed output")
		}
		fields := strings.Fields(string(meta))
		if len(fields) != 4 {
			return nil, errors.New("git ls-tree: malformed output")
		}
		rel := string(p)
		if subpath != "" {
			var in bool
			rel, in = strings.CutPrefix(rel, subpath+"/")
			if !in {
				continue
			}
			found = true
		}
		rows = append(rows, raw{fields[0], fields[1], fields[2], fields[3], rel})
	}
	if !found {
		return nil, ErrSubpathNotFound
	}
	if len(rows) > manifestbundle.MaxFiles {
		tree.Problems = []manifestbundle.Problem{manifestbundle.NewProblem("", manifestbundle.RuleTooManyFiles)}
		return tree, nil
	}
	for _, r := range rows {
		switch {
		case r.mode == "120000":
			tree.Problems = append(tree.Problems, manifestbundle.NewProblem(r.path, manifestbundle.RuleGitSymlink))
			continue
		case r.mode == "160000" || r.typ == "commit":
			tree.Problems = append(tree.Problems, manifestbundle.NewProblem(r.path, manifestbundle.RuleGitSubmodule))
			continue
		}
		mode, err := manifestbundle.ModeFromGit(r.mode)
		if err != nil || r.typ != "blob" {
			tree.Problems = append(tree.Problems, manifestbundle.NewProblem(r.path, manifestbundle.RuleGitSymlink))
			continue
		}
		size, err := strconv.ParseInt(r.size, 10, 64)
		if err != nil {
			return nil, errors.New("git ls-tree: malformed size")
		}
		if size > manifestbundle.MaxFileSize {
			tree.Problems = append(tree.Problems, manifestbundle.NewProblem(r.path, manifestbundle.RuleFileTooLarge))
			continue
		}
		total += size
		entries = append(entries, entry{path: r.path, oid: r.oid, mode: mode})
	}
	if total > manifestbundle.MaxBundleSize {
		tree.Problems = append(tree.Problems, manifestbundle.NewProblem("", manifestbundle.RuleBundleTooLarge))
		tree.Problems = manifestbundle.SortProblems(tree.Problems)
		return tree, nil // content is not read
	}
	tree.Problems = manifestbundle.SortProblems(tree.Problems)
	if len(entries) == 0 {
		return tree, nil
	}

	var in strings.Builder
	for _, e := range entries {
		in.WriteString(e.oid)
		in.WriteByte('\n')
	}
	out, err := g.run(ctx, strings.NewReader(in.String()), "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	rd := bufio.NewReader(bytes.NewReader(out))
	for _, e := range entries {
		header, err := rd.ReadString('\n')
		if err != nil {
			return nil, errors.New("git cat-file: truncated output")
		}
		h := strings.Fields(header)
		if len(h) != 3 || h[0] != e.oid || h[1] != "blob" {
			return nil, errors.New("git cat-file: unexpected object")
		}
		n, err := strconv.Atoi(h[2])
		if err != nil || n < 0 || n > manifestbundle.MaxFileSize {
			return nil, errors.New("git cat-file: unexpected size")
		}
		content := make([]byte, n)
		if _, err := io.ReadFull(rd, content); err != nil {
			return nil, errors.New("git cat-file: truncated output")
		}
		if b, err := rd.ReadByte(); err != nil || b != '\n' {
			return nil, errors.New("git cat-file: malformed output")
		}
		tree.Files = append(tree.Files, manifestbundle.File{Path: e.path, Content: content, Mode: e.mode})
	}
	return tree, nil
}
