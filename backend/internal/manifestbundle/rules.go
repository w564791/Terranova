package manifestbundle

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Bundle limits. The editor enforces the same values on every draft write
// (manifest_files_handler), so a draft that the editor accepted only fails
// publish for the content rules (denylist, secret scan).
const (
	MaxPathLen    = 256              // bytes, POSIX relative path
	MaxFileSize   = 1 * 1024 * 1024  // bytes per file
	MaxBundleSize = 50 * 1024 * 1024 // bytes, sum of all file contents
)

// Rule names. A Problem carries only a rule name and a path; it never carries
// file content or matched text, so it is safe for API responses, the
// bundle_invalid_reason column and logs.
const (
	RulePathInvalid       = "path_invalid"
	RulePathTooLong       = "path_too_long"
	RulePathNotNFC        = "path_not_nfc"
	RulePathDuplicate     = "path_duplicate"
	RulePathCaseDuplicate = "path_case_duplicate"
	RuleDenylistedFile    = "denylisted_file"
	RuleFileTooLarge      = "file_too_large"
	RuleBundleTooLarge    = "bundle_too_large"
	RuleSecretScanPrefix  = "secret_scan:"
)

// Problem one rule violation, the 422 publish shape {file, line?, rule,
// message}: File is the offending path (empty for bundle-wide rules), Line
// the 1-based line of a secret-scan hit (omitted for path / size / denylist
// rules), Message a fixed text per rule. Never extend this with content or
// matched text.
type Problem struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// String renders "rule @ file" (file quoted when it is not printable, so a
// hostile path cannot inject log lines). It is the bundle_invalid_reason /
// log form and carries no line or message.
func (p Problem) String() string {
	if p.File == "" {
		return p.Rule
	}
	return p.Rule + " @ " + displayPath(p.File)
}

// secretKindNames human names of the secret-scan kinds (for messages).
var secretKindNames = map[string]string{
	"aws_access_key": "AWS access key",
	"private_key":    "private key",
	"github_token":   "GitHub token",
	"slack_token":    "Slack token",
}

// ruleMessage fixed, content-free message of a rule.
func ruleMessage(rule string) string {
	switch rule {
	case RulePathInvalid:
		return "invalid path: must be a relative POSIX path without empty, '.' or '..' segments, backslashes or control characters"
	case RulePathTooLong:
		return fmt.Sprintf("path is longer than %d bytes", MaxPathLen)
	case RulePathNotNFC:
		return "path is not Unicode NFC normalized"
	case RulePathDuplicate:
		return "duplicate path"
	case RulePathCaseDuplicate:
		return "path differs from another path only by letter case"
	case RuleDenylistedFile:
		return "file type is not allowed in a bundle (variable values, state, git metadata, env or private key files)"
	case RuleFileTooLarge:
		return fmt.Sprintf("file is larger than %d MB", MaxFileSize/(1024*1024))
	case RuleBundleTooLarge:
		return fmt.Sprintf("bundle is larger than %d MB", MaxBundleSize/(1024*1024))
	}
	if kind, ok := strings.CutPrefix(rule, RuleSecretScanPrefix); ok {
		name := secretKindNames[kind]
		if name == "" {
			name = "credential"
		}
		return "possible " + name + " found; remove it and use a variable or secret store instead"
	}
	return rule
}

func displayPath(p string) string {
	if !utf8.ValidString(p) {
		return strconv.QuoteToASCII(p)
	}
	for _, r := range p {
		if !unicode.IsPrint(r) {
			return strconv.QuoteToASCII(p)
		}
	}
	return p
}

// maxReasonProblems caps how many problems a reason string lists.
const maxReasonProblems = 20

// Reason joins problems into the bundle_invalid_reason / log form:
// "rule @ path; rule @ path; ... (+N more)". Empty for no problems.
func Reason(problems []Problem) string {
	if len(problems) == 0 {
		return ""
	}
	n := len(problems)
	if n > maxReasonProblems {
		n = maxReasonProblems
	}
	parts := make([]string, 0, n+1)
	for _, p := range problems[:n] {
		parts = append(parts, p.String())
	}
	if extra := len(problems) - n; extra > 0 {
		parts = append(parts, fmt.Sprintf("(+%d more)", extra))
	}
	return strings.Join(parts, "; ")
}

// ValidatePath checks one stored bundle path (already normalized: POSIX
// separators, no leading slash). It returns the violated rule name or "".
// The editor additionally restricts drafts to an ASCII charset; bundle rules
// accept Unicode (git trees) but require NFC.
func ValidatePath(p string) string {
	switch {
	case p == "", strings.HasPrefix(p, "/"), strings.HasSuffix(p, "/"),
		strings.ContainsRune(p, '\\'), !utf8.ValidString(p):
		return RulePathInvalid
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return RulePathInvalid
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return RulePathInvalid
		}
	}
	if len(p) > MaxPathLen {
		return RulePathTooLong
	}
	if !norm.NFC.IsNormalString(p) {
		return RulePathNotNFC
	}
	return ""
}

// ErrPathInvalid is returned by CheckPath for editor callers.
var ErrPathInvalid = errors.New("invalid path")

// CheckPath is ValidatePath as an error (editor convenience).
func CheckPath(p string) error {
	if rule := ValidatePath(p); rule != "" {
		return fmt.Errorf("%w: %s", ErrPathInvalid, rule)
	}
	return nil
}

// denylisted reports files that must never be part of a bundle: variable
// value files (values come from varsets / overrides, never from the bundle),
// state files, the local provider cache, CLI credential config, git metadata
// (.git/ at any depth, or a .git file), dotenv files (.env, .env.*) and
// private key / keystore files by name (*.pem, *.key, *.p12, *.pfx, id_rsa,
// id_dsa, id_ecdsa, id_ed25519; public keys such as id_rsa.pub are allowed).
// Names match case-insensitively.
// .terraform.lock.hcl is allowed (init verifies it).
func denylisted(p string) bool {
	segs := strings.Split(p, "/")
	for _, s := range segs[:len(segs)-1] {
		if s = strings.ToLower(s); s == ".terraform" || s == ".git" {
			return true
		}
	}
	base := strings.ToLower(segs[len(segs)-1])
	switch base {
	case ".terraformrc", "terraform.rc", ".git", ".env",
		"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519":
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		return true
	}
	for _, suffix := range []string{".tfvars", ".tfvars.json", ".tfstate", ".tfstate.backup",
		".pem", ".key", ".p12", ".pfx"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

// secretPatterns high-confidence credential formats. Only the kind name is
// ever reported; the matched bytes are discarded.
var secretPatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"aws_access_key", regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"private_key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |ENCRYPTED |PGP )?PRIVATE KEY(?: BLOCK)?-----`)},
	{"github_token", regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{82})\b`)},
	{"slack_token", regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`)},
}

// Validate applies every bundle rule and returns the problems sorted by
// (file, rule). nil means the file set is a valid bundle.
func Validate(files []File) []Problem {
	var out []Problem
	add := func(rule, path string) {
		out = append(out, Problem{File: path, Rule: rule, Message: ruleMessage(rule)})
	}

	seen := make(map[string]bool, len(files))
	folded := make(map[string]string, len(files))
	total := 0
	for _, f := range files {
		total += len(f.Content)
		if rule := ValidatePath(f.Path); rule != "" {
			add(rule, f.Path)
		}
		if seen[f.Path] {
			add(RulePathDuplicate, f.Path)
		} else {
			seen[f.Path] = true
			key := strings.ToLower(norm.NFC.String(f.Path))
			if other, ok := folded[key]; ok {
				add(RulePathCaseDuplicate, f.Path)
				add(RulePathCaseDuplicate, other)
			} else {
				folded[key] = f.Path
			}
		}
		if denylisted(f.Path) {
			add(RuleDenylistedFile, f.Path)
		}
		if len(f.Content) > MaxFileSize {
			add(RuleFileTooLarge, f.Path)
		}
		for _, sp := range secretPatterns {
			if loc := sp.re.FindIndex(f.Content); loc != nil {
				rule := RuleSecretScanPrefix + sp.kind
				line := bytes.Count(f.Content[:loc[0]], []byte{'\n'}) + 1 // 1-based line of the first hit
				out = append(out, Problem{File: f.Path, Line: line, Rule: rule, Message: ruleMessage(rule)})
			}
		}
	}
	if total > MaxBundleSize {
		add(RuleBundleTooLarge, "")
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Rule < out[j].Rule
	})
	// a path can collide case-wise with several others; report it once per rule
	var dedup []Problem
	for i, p := range out {
		if i > 0 && out[i-1] == p {
			continue
		}
		dedup = append(dedup, p)
	}
	if len(dedup) == 0 {
		return nil
	}
	return dedup
}
