package keys

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const oldJWTSecret = "old-jwt-secret-value"

func randKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// setKeys a production-like configuration: signing root v1, cutoff unset.
func setKeys(t *testing.T) (root string) {
	t.Helper()
	root = randKey(t)
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", oldJWTSecret)
	t.Setenv("SIGNING_ROOT_KEY", root)
	t.Setenv("SIGNING_ROOT_KEY_VERSION", "")
	t.Setenv("SIGNING_ROOT_KEY_PREVIOUS", "")
	t.Setenv("SIGNING_ROOT_KEY_PREVIOUS_VERSION", "")
	t.Setenv("DATA_ENCRYPTION_KEY", randKey(t))
	t.Setenv("LEGACY_TOKEN_CUTOFF", "")
	t.Setenv("LEGACY_TOKEN_ISSUED_BEFORE", "")
	return root
}

func claims(iat time.Time, ttl time.Duration) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(iat), ExpiresAt: jwt.NewNumericDate(iat.Add(ttl)), Subject: "u-1"}
}

func verify(p Purpose, tok string, legacy []byte) error {
	_, err := jwt.ParseWithClaims(tok, &jwt.RegisteredClaims{}, Keyfunc(p, legacy), jwt.WithValidMethods([]string{"HS256"}))
	return err
}

func signRaw(t *testing.T, key []byte, kid string, c jwt.Claims) string {
	t.Helper()
	tk := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	if kid != "" {
		tk.Header["kid"] = kid
	}
	s, err := tk.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSign_CarriesKidAndVerifies(t *testing.T) {
	setKeys(t)
	tok, err := Sign(PurposeUser, claims(time.Now(), time.Hour), []byte(oldJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _ := jwt.NewParser().ParseUnverified(tok, &jwt.RegisteredClaims{})
	if parsed.Header["kid"] != "user-v1" {
		t.Fatalf("kid = %v, want user-v1", parsed.Header["kid"])
	}
	if err := verify(PurposeUser, tok, []byte(oldJWTSecret)); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// purpose separation
	if err := verify(PurposeState, tok, nil); err == nil {
		t.Fatal("a user token verified as a state token")
	}
	// the token does not verify with JWT_SECRET itself
	if _, err := jwt.Parse(tok, func(*jwt.Token) (interface{}, error) { return []byte(oldJWTSecret), nil }); err == nil {
		t.Fatal("new token verifies with JWT_SECRET")
	}
}

func TestKidRotation_PreviousRootAccepted(t *testing.T) {
	oldRoot := setKeys(t)
	oldTok, err := Sign(PurposeState, claims(time.Now(), time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	// rotate: new root v2, old root as previous v1
	t.Setenv("SIGNING_ROOT_KEY", randKey(t))
	t.Setenv("SIGNING_ROOT_KEY_VERSION", "2")
	t.Setenv("SIGNING_ROOT_KEY_PREVIOUS", oldRoot)
	t.Setenv("SIGNING_ROOT_KEY_PREVIOUS_VERSION", "1")
	newTok, err := Sign(PurposeState, claims(time.Now(), time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _ := jwt.NewParser().ParseUnverified(newTok, &jwt.RegisteredClaims{})
	if parsed.Header["kid"] != "state-v2" {
		t.Fatalf("kid = %v", parsed.Header["kid"])
	}
	if err := verify(PurposeState, oldTok, nil); err != nil {
		t.Fatalf("previous-kid token rejected during rotation: %v", err)
	}
	if err := verify(PurposeState, newTok, nil); err != nil {
		t.Fatalf("current token: %v", err)
	}
	// rotation finished: previous removed
	t.Setenv("SIGNING_ROOT_KEY_PREVIOUS", "")
	if err := verify(PurposeState, oldTok, nil); !errors.Is(err, ErrUnknownKeyVersion) {
		t.Fatalf("old kid after rotation: %v, want ErrUnknownKeyVersion", err)
	}
}

func TestLegacyToken_AcceptedInsideWindow(t *testing.T) {
	setKeys(t)
	now := time.Now()
	t.Setenv("LEGACY_TOKEN_ISSUED_BEFORE", now.Add(-time.Minute).UTC().Format(time.RFC3339))
	t.Setenv("LEGACY_TOKEN_CUTOFF", now.Add(7*24*time.Hour).UTC().Format(time.RFC3339))
	tok := signRaw(t, []byte(oldJWTSecret), "", claims(now.Add(-time.Hour), 24*time.Hour))
	if err := verify(PurposeUser, tok, []byte(oldJWTSecret)); err != nil {
		t.Fatalf("legacy user token in window: %v", err)
	}
	stateTok := signRaw(t, []byte("state:"+oldJWTSecret), "", claims(now.Add(-time.Hour), 7*24*time.Hour))
	if err := verify(PurposeState, stateTok, LegacyStateSecret()); err != nil {
		t.Fatalf("legacy state token in window: %v", err)
	}
	// legacy state key is not the user key
	if err := verify(PurposeUser, stateTok, []byte(oldJWTSecret)); err == nil {
		t.Fatal("legacy state token accepted as user token")
	}
}

func TestLegacyToken_RejectedAfterCutoffDespiteLongExp(t *testing.T) {
	setKeys(t)
	now := time.Now()
	t.Setenv("LEGACY_TOKEN_ISSUED_BEFORE", now.Add(-48*time.Hour).UTC().Format(time.RFC3339))
	t.Setenv("LEGACY_TOKEN_CUTOFF", now.Add(-time.Minute).UTC().Format(time.RFC3339))
	// freshly signed with the old JWT_SECRET, no kid, exp one year out
	tok := signRaw(t, []byte(oldJWTSecret), "", claims(now.Add(-72*time.Hour), 365*24*time.Hour))
	if err := verify(PurposeUser, tok, []byte(oldJWTSecret)); !errors.Is(err, ErrLegacyTokenRejected) {
		t.Fatalf("after cutoff: %v, want ErrLegacyTokenRejected", err)
	}
}

func TestLegacyToken_RejectedWhenCutoffUnset(t *testing.T) {
	setKeys(t)
	tok := signRaw(t, []byte(oldJWTSecret), "", claims(time.Now().Add(-time.Hour), time.Hour*2))
	if err := verify(PurposeUser, tok, []byte(oldJWTSecret)); !errors.Is(err, ErrLegacyTokenRejected) {
		t.Fatalf("cutoff unset: %v, want ErrLegacyTokenRejected", err)
	}
}

func TestLegacyToken_RejectedWhenIssuedAfterSwitchOrNoIat(t *testing.T) {
	setKeys(t)
	now := time.Now()
	t.Setenv("LEGACY_TOKEN_ISSUED_BEFORE", now.Add(-time.Hour).UTC().Format(time.RFC3339))
	t.Setenv("LEGACY_TOKEN_CUTOFF", now.Add(time.Hour).UTC().Format(time.RFC3339))
	tok := signRaw(t, []byte(oldJWTSecret), "", claims(now, time.Hour))
	if err := verify(PurposeUser, tok, []byte(oldJWTSecret)); !errors.Is(err, ErrLegacyTokenRejected) {
		t.Fatalf("iat after switch: %v", err)
	}
	noIat := signRaw(t, []byte(oldJWTSecret), "", jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))})
	if err := verify(PurposeUser, noIat, []byte(oldJWTSecret)); !errors.Is(err, ErrLegacyTokenRejected) {
		t.Fatalf("no iat: %v", err)
	}
}

// New keys do not depend on JWT_SECRET: a token signed with a key derived from
// JWT_SECRET with the new labels, with or without a valid-looking kid, fails.
func TestKeysDerivedFromJWTSecretRejected(t *testing.T) {
	setKeys(t)
	now := time.Now()
	t.Setenv("LEGACY_TOKEN_ISSUED_BEFORE", now.UTC().Format(time.RFC3339))
	t.Setenv("LEGACY_TOKEN_CUTOFF", now.Add(time.Hour).UTC().Format(time.RFC3339))
	for _, p := range []Purpose{PurposeUser, PurposeState, PurposeAgent, PurposeRun, PurposeRunTask} {
		derived := DeriveSigningKey([]byte(oldJWTSecret), p)
		for _, kid := range []string{Kid(p, 1), ""} {
			tok := signRaw(t, derived, kid, claims(now.Add(-time.Minute), time.Hour))
			legacy := []byte(oldJWTSecret)
			if p == PurposeState {
				legacy = LegacyStateSecret()
			}
			if err := verify(p, tok, legacy); err == nil {
				t.Fatalf("%s token (kid %q) signed with an HKDF(JWT_SECRET) key was accepted", p, kid)
			}
		}
	}
}

func TestLegacySigningMode_DevelopmentOnly(t *testing.T) {
	setKeys(t)
	t.Setenv("SIGNING_ROOT_KEY", "")
	t.Setenv("ENV", "development")
	if !LegacySigningMode() {
		t.Fatal("expected legacy signing mode in development without SIGNING_ROOT_KEY")
	}
	tok, err := Sign(PurposeUser, claims(time.Now(), time.Hour), []byte(oldJWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	parsed, _, _ := jwt.NewParser().ParseUnverified(tok, &jwt.RegisteredClaims{})
	if _, ok := parsed.Header["kid"]; ok {
		t.Fatal("legacy mode token carries kid")
	}
	if err := verify(PurposeUser, tok, []byte(oldJWTSecret)); err != nil {
		t.Fatal(err)
	}
	if _, err := Sign(PurposeAgent, claims(time.Now(), time.Hour), nil); err == nil {
		t.Fatal("agent token signed without SIGNING_ROOT_KEY")
	}
	t.Setenv("ENV", "production")
	if LegacySigningMode() {
		t.Fatal("legacy mode in production")
	}
}

func TestCheckStartup(t *testing.T) {
	setKeys(t)
	if _, err := CheckStartup(); err != nil {
		t.Fatalf("valid production config: %v", err)
	}
	for _, name := range []string{"DATA_ENCRYPTION_KEY", "SIGNING_ROOT_KEY"} {
		t.Run(name, func(t *testing.T) {
			setKeys(t)
			t.Setenv(name, "")
			if _, err := CheckStartup(); err == nil {
				t.Fatalf("production without %s started", name)
			}
			t.Setenv("ENV", "development")
			w, err := CheckStartup()
			if err != nil || len(w) == 0 {
				t.Fatalf("development without %s: warnings=%v err=%v", name, w, err)
			}
			t.Setenv(name, "CHANGE-ME!openssl-rand-base64-32")
			if _, err := CheckStartup(); err == nil {
				t.Fatalf("malformed %s accepted", name)
			}
		})
	}
	setKeys(t)
	t.Setenv("DATA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 16)))
	if _, err := CheckStartup(); err == nil {
		t.Fatal("16-byte data key accepted")
	}
	setKeys(t)
	t.Setenv("LEGACY_TOKEN_CUTOFF", "next tuesday")
	if _, err := CheckStartup(); err == nil {
		t.Fatal("malformed LEGACY_TOKEN_CUTOFF accepted")
	}
}
