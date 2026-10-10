// Package gitsource is the platform side of git-sourced manifests (sandbox
// spec §9 step 8): a GitHub App mints a short-lived installation token
// (contents:read, the single repo), the platform fetches the tree of one
// pinned commit SHA with the git CLI and hands it to the bundle rules. Runs
// never fetch git; they use the stored bundle.
//
// Token hygiene: the token lives in memory only (never stored, never logged,
// never in a URL or argv). git receives it through GIT_ASKPASS, a script that
// prints an environment variable of the git process; every git error is
// scrubbed of the token before it is returned.
package gitsource

import (
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// ErrDisabled the GitHub App is not configured: git sources are disabled.
var ErrDisabled = errors.New("git source is disabled: GitHub App credentials are not configured (GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY)")

// ErrWebhookDisabled no webhook secret is configured.
var ErrWebhookDisabled = errors.New("GitHub webhook is disabled: GITHUB_WEBHOOK_SECRET is not configured")

// AppCredentials the GitHub App identity (never logged; String hides the key).
type AppCredentials struct {
	AppID      int64
	PrivateKey *rsa.PrivateKey
}

func (AppCredentials) String() string { return "gitsource.AppCredentials{***}" }

// GoString hides the key from %#v.
func (c AppCredentials) GoString() string { return c.String() }

// CredentialsProvider supplies the App credentials, like keys.KeyProvider
// does for root keys: EnvCredentials by default, a KMS / secret-manager
// backed provider can replace Credentials.
type CredentialsProvider interface {
	// AppCredentials ErrDisabled when not configured.
	AppCredentials() (AppCredentials, error)
	// WebhookSecret ErrWebhookDisabled when not configured.
	WebhookSecret() ([]byte, error)
}

// EnvCredentials reads
//
//	GITHUB_APP_ID               numeric App id
//	GITHUB_APP_PRIVATE_KEY      PEM private key (or base64 of the PEM)
//	GITHUB_APP_PRIVATE_KEY_FILE path of the PEM (alternative)
//	GITHUB_WEBHOOK_SECRET       webhook secret (optional; webhooks off without it)
//
// on every call (no caching), so rotation is a restart.
type EnvCredentials struct{}

// Credentials is the provider in use.
var Credentials CredentialsProvider = EnvCredentials{}

func (EnvCredentials) AppCredentials() (AppCredentials, error) {
	idStr := strings.TrimSpace(os.Getenv("GITHUB_APP_ID"))
	pemStr := strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY"))
	if pemStr == "" {
		if path := strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE")); path != "" {
			b, err := os.ReadFile(path)
			if err != nil {
				return AppCredentials{}, fmt.Errorf("GITHUB_APP_PRIVATE_KEY_FILE: %w", err)
			}
			pemStr = strings.TrimSpace(string(b))
		}
	}
	if idStr == "" || pemStr == "" {
		return AppCredentials{}, ErrDisabled
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return AppCredentials{}, errors.New("GITHUB_APP_ID must be a positive integer")
	}
	key, err := ParsePrivateKey(pemStr)
	if err != nil {
		return AppCredentials{}, err
	}
	return AppCredentials{AppID: id, PrivateKey: key}, nil
}

func (EnvCredentials) WebhookSecret() ([]byte, error) {
	s := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if strings.TrimSpace(s) == "" {
		return nil, ErrWebhookDisabled
	}
	return []byte(s), nil
}

// ParsePrivateKey a PEM RSA key, or the base64 of one. The error never
// contains key material.
func ParsePrivateKey(s string) (*rsa.PrivateKey, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "-----BEGIN") {
		if b, err := base64.StdEncoding.DecodeString(s); err == nil {
			s = string(b)
		}
	}
	s = strings.ReplaceAll(s, `\n`, "\n") // single-line env form
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(s))
	if err != nil {
		return nil, errors.New("GITHUB_APP_PRIVATE_KEY is not a valid PEM RSA private key")
	}
	return key, nil
}

// Endpoints where GitHub lives.
//
//	GITHUB_URL      web / git base, default https://github.com (GitHub
//	                Enterprise Server: https://ghe.example.com)
//	GITHUB_API_URL  REST base, default https://api.github.com for
//	                github.com, <GITHUB_URL>/api/v3 otherwise
type Endpoints struct {
	WebURL string // no trailing slash
	APIURL string // no trailing slash
}

// EndpointsFromEnv the configured endpoints.
func EndpointsFromEnv() (Endpoints, error) {
	web := strings.TrimRight(strings.TrimSpace(os.Getenv("GITHUB_URL")), "/")
	if web == "" {
		web = "https://github.com"
	}
	api := strings.TrimRight(strings.TrimSpace(os.Getenv("GITHUB_API_URL")), "/")
	if api == "" {
		if web == "https://github.com" {
			api = "https://api.github.com"
		} else {
			api = web + "/api/v3"
		}
	}
	for name, v := range map[string]string{"GITHUB_URL": web, "GITHUB_API_URL": api} {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" {
			return Endpoints{}, fmt.Errorf("%s must be an http(s) URL without credentials or query", name)
		}
	}
	return Endpoints{WebURL: web, APIURL: api}, nil
}
