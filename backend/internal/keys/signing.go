package keys

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/hkdf"
)

// Purpose of a signing key. Each purpose has its own key, derived with HKDF
// from SIGNING_ROOT_KEY (info "terranova/jwt/<purpose>"), so a token of one
// purpose never verifies as another.
type Purpose string

const (
	PurposeUser    Purpose = "user"    // login, user API and team API tokens
	PurposeState   Purpose = "state"   // Terraform HTTP state backend tokens
	PurposeAgent   Purpose = "agent"   // per-agent tokens
	PurposeRun     Purpose = "run"     // manifest sandbox run tokens
	PurposeRunTask Purpose = "runtask" // Run Task callback tokens
)

// ErrLegacyTokenRejected a token without kid outside the legacy window.
var ErrLegacyTokenRejected = errors.New("token without kid is no longer accepted")

// DeriveSigningKey HKDF-SHA256(root, salt = nil, info = "terranova/jwt/<p>").
func DeriveSigningKey(root []byte, p Purpose) []byte {
	out := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, root, nil, []byte("terranova/jwt/"+string(p))), out); err != nil {
		panic(err) // cannot happen for 32 bytes
	}
	return out
}

// Kid the JWT kid of purpose p at root version v: "<purpose>-v<version>".
func Kid(p Purpose, version int) string {
	return string(p) + "-v" + strconv.Itoa(version)
}

func parseKid(kid string, p Purpose) (int, bool) {
	rest, ok := strings.CutPrefix(kid, string(p)+"-v")
	if !ok {
		return 0, false
	}
	v, err := strconv.Atoi(rest)
	return v, err == nil && v >= 1
}

// Sign signs claims for purpose p with the current key (HS256, header kid).
// In legacy signing mode (development without SIGNING_ROOT_KEY) the token is
// signed with legacy (no kid), as before; legacy nil means the purpose has no
// legacy scheme and SIGNING_ROOT_KEY is required.
func Sign(p Purpose, claims jwt.Claims, legacy []byte) (string, error) {
	if LegacySigningMode() {
		if legacy == nil {
			return "", fmt.Errorf("SIGNING_ROOT_KEY is required to sign %s tokens", p)
		}
		return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(legacy)
	}
	root, err := SigningRoots.Current()
	if err != nil {
		return "", fmt.Errorf("signing %s token: %w", p, err)
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	t.Header["kid"] = Kid(p, root.Version)
	return t.SignedString(DeriveSigningKey(root.Material, p))
}

// Keyfunc verifies tokens of purpose p:
//   - with kid "<p>-v<n>": the key derived from SIGNING_ROOT_KEY version n
//     (current or previous, so tokens survive a root rotation); a kid of
//     another purpose is refused;
//   - without kid: legacy (JWT_SECRET based) only while the legacy window is
//     open (LegacyTokenAllowed), or always in legacy signing mode
//     (development without SIGNING_ROOT_KEY).
//
// Only HMAC algorithms are accepted; callers should also pass
// jwt.WithValidMethods([]string{"HS256"}).
func Keyfunc(p Purpose, legacy []byte) jwt.Keyfunc {
	return func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		if kidRaw, present := t.Header["kid"]; present {
			kid, _ := kidRaw.(string)
			version, ok := parseKid(kid, p)
			if !ok {
				return nil, fmt.Errorf("token kid %q is not a %s key", kid, p)
			}
			root, err := SigningRoots.ByVersion(version)
			if err != nil {
				return nil, err
			}
			return DeriveSigningKey(root.Material, p), nil
		}
		if legacy == nil {
			return nil, ErrLegacyTokenRejected
		}
		if LegacySigningMode() {
			return legacy, nil
		}
		var iat *jwt.NumericDate
		if t.Claims != nil {
			iat, _ = t.Claims.GetIssuedAt()
		}
		if err := LegacyTokenAllowed(iat, time.Now()); err != nil {
			return nil, err
		}
		return legacy, nil
	}
}

// LegacyTokenAllowed the legacy window for tokens without kid:
//   - LEGACY_TOKEN_CUTOFF (RFC3339; deploy time + the longest token lifetime)
//     is the control: unset, or now after it, rejects every no-kid token
//     regardless of its exp;
//   - the token must carry iat and iat must not be after
//     LEGACY_TOKEN_ISSUED_BEFORE (RFC3339, the deploy time; defaults to this
//     process's start). iat is chosen by whoever signed the token, so this only
//     catches tokens minted with the old secret after the switch; the cutoff
//     is the real limit.
//
// TODO(2026-11-15): remove the legacy path once every deployment's
// LEGACY_TOKEN_CUTOFF has passed, then retire JWT_SECRET (see
// docs/security/signing-and-encryption-keys.md).
func LegacyTokenAllowed(iat *jwt.NumericDate, now time.Time) error {
	cutoff, err := legacyCutoff()
	if err != nil {
		return err
	}
	if cutoff.IsZero() {
		return fmt.Errorf("%w (LEGACY_TOKEN_CUTOFF not set)", ErrLegacyTokenRejected)
	}
	if now.After(cutoff) {
		return fmt.Errorf("%w (LEGACY_TOKEN_CUTOFF %s passed)", ErrLegacyTokenRejected, cutoff.UTC().Format(time.RFC3339))
	}
	if iat == nil {
		return fmt.Errorf("%w (no iat)", ErrLegacyTokenRejected)
	}
	issuedBefore, err := legacyIssuedBefore()
	if err != nil {
		return err
	}
	if iat.Time.After(issuedBefore) {
		return fmt.Errorf("%w (issued after the key switch)", ErrLegacyTokenRejected)
	}
	return nil
}

var processStart = time.Now()

func legacyCutoff() (time.Time, error) {
	return envTime("LEGACY_TOKEN_CUTOFF")
}

func legacyIssuedBefore() (time.Time, error) {
	t, err := envTime("LEGACY_TOKEN_ISSUED_BEFORE")
	if err == nil && t.IsZero() {
		return processStart, nil
	}
	return t, err
}

func envTime(name string) (time.Time, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC3339: %w", name, err)
	}
	return t, nil
}

// LegacyJWTSecret the legacy key of user / runtask tokens (raw JWT_SECRET).
// nil when JWT_SECRET is unset.
func LegacyJWTSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return nil
}

// LegacyStateSecret the legacy key of state tokens ("state:" + JWT_SECRET).
func LegacyStateSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte("state:" + s)
	}
	return nil
}

// Fingerprint a non-secret identifier of key material (for logs / tests).
func Fingerprint(material []byte) string {
	m := hmac.New(sha256.New, []byte("terranova/key-fingerprint"))
	m.Write(material)
	return fmt.Sprintf("%x", m.Sum(nil)[:6])
}

// LegacySecret []byte(s), nil for "" (no legacy key).
func LegacySecret(s string) []byte {
	if s == "" {
		return nil
	}
	return []byte(s)
}
