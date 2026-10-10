// Package reservedenv is the single source of truth for platform-controlled
// environment variable names that users must not set (workspace variables,
// variable sets, manifest deployment env overrides). Matching is
// case-insensitive. The executor strips reserved keys from user-supplied env
// at run time; write APIs reject them with 422 reserved_env_var.
package reservedenv

import (
	"strings"
)

// Match how a pattern matches a key.
type Match string

const (
	MatchExact  Match = "exact"
	MatchPrefix Match = "prefix"
)

// Pattern a reserved name or prefix.
type Pattern struct {
	Pattern string `json:"pattern"`
	Match   Match  `json:"match"`
}

// ErrorCode returned on save rejection.
const ErrorCode = "reserved_env_var"

// patterns is the ordered list returned by the read-only API and used for
// matching. Keep in sync with executor strip and docs.
var patterns = []Pattern{
	// Terraform CLI / install / registry / HTTP backend
	{Pattern: "TF_CLI_ARGS", Match: MatchPrefix}, // TF_CLI_ARGS and TF_CLI_ARGS_*
	{Pattern: "TF_CLI_CONFIG_FILE", Match: MatchExact},
	{Pattern: "TF_PLUGIN_CACHE", Match: MatchPrefix},
	{Pattern: "TF_HTTP_", Match: MatchPrefix},
	{Pattern: "TF_TOKEN_", Match: MatchPrefix},
	{Pattern: "TF_REGISTRY_", Match: MatchPrefix},
	// Platform-set Terraform automation flags
	{Pattern: "TF_IN_AUTOMATION", Match: MatchExact},
	{Pattern: "TF_INPUT", Match: MatchExact},
	// TLS / proxy / git credential surfaces
	{Pattern: "GIT_SSL_", Match: MatchPrefix},
	{Pattern: "GIT_ASKPASS", Match: MatchExact},
	{Pattern: "GIT_CONFIG", Match: MatchPrefix},
	{Pattern: "SSL_CERT_", Match: MatchPrefix},
	{Pattern: "CURL_CA_BUNDLE", Match: MatchExact},
	{Pattern: "NODE_TLS_REJECT_UNAUTHORIZED", Match: MatchExact},
	{Pattern: "HTTP_PROXY", Match: MatchExact},
	{Pattern: "HTTPS_PROXY", Match: MatchExact},
	{Pattern: "NO_PROXY", Match: MatchExact},
	// Internal / platform-injected
	{Pattern: "_TERRANOVA_", Match: MatchPrefix},
	{Pattern: "IAC_", Match: MatchPrefix},
}

// Patterns returns a copy of the reserved list for the API.
func Patterns() []Pattern {
	out := make([]Pattern, len(patterns))
	copy(out, patterns)
	return out
}

// Prefixes returns the reserved patterns as plain strings (exact names and
// prefixes alike) for GET /system/reserved-env-prefixes.
func Prefixes() []string {
	out := make([]string, len(patterns))
	for i, p := range patterns {
		out[i] = p.Pattern
	}
	return out
}

// Hit describes one reserved key that matched.
type Hit struct {
	Key            string `json:"key"`
	ReservedPrefix string `json:"reserved_prefix"`
	Match          Match  `json:"match"`
}

// MatchKey returns the first matching pattern for key, or nil.
func MatchKey(key string) *Hit {
	k := strings.ToUpper(strings.TrimSpace(key))
	if k == "" {
		return nil
	}
	for _, p := range patterns {
		pat := strings.ToUpper(p.Pattern)
		switch p.Match {
		case MatchExact:
			if k == pat {
				return &Hit{Key: key, ReservedPrefix: p.Pattern, Match: p.Match}
			}
		case MatchPrefix:
			if strings.HasPrefix(k, pat) {
				return &Hit{Key: key, ReservedPrefix: p.Pattern, Match: p.Match}
			}
		}
	}
	return nil
}

// FindHits returns every reserved key in keys (preserving input order).
func FindHits(keys []string) []Hit {
	var hits []Hit
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			continue
		}
		if h := MatchKey(key); h != nil {
			hits = append(hits, *h)
			seen[key] = true
		}
	}
	return hits
}

// IsEnvironmentCategory reports whether category is injected as process env
// (as opposed to TF_VAR_* terraform variables).
func IsEnvironmentCategory(category string) bool {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "environment", "env":
		return true
	default:
		return false
	}
}

// SanitizePairs drops KEY=VALUE entries whose key is reserved. Platform code
// must re-append its own values afterwards.
func SanitizePairs(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		if MatchKey(key) != nil {
			continue
		}
		out = append(out, kv)
	}
	return out
}
