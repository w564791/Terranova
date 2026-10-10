package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func dataKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func setDataKey(t *testing.T) string {
	t.Helper()
	k := dataKey(t)
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "jwt-secret-a")
	t.Setenv("DATA_ENCRYPTION_KEY", k)
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "")
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS", "")
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS_VERSION", "")
	return k
}

func TestChangingJWTSecretDoesNotAffectDecryption(t *testing.T) {
	setDataKey(t)
	ct, ver, err := EncryptValueVersioned("s3cr3t")
	if err != nil {
		t.Fatal(err)
	}
	if ver != 1 || !strings.HasPrefix(ct, "tnk1:") {
		t.Fatalf("ciphertext %q version %d", ct, ver)
	}
	t.Setenv("JWT_SECRET", "a-completely-different-jwt-secret")
	pt, err := DecryptValueWithVersion(ct, ver)
	if err != nil || pt != "s3cr3t" {
		t.Fatalf("decrypt after JWT_SECRET change: %q, %v", pt, err)
	}
	t.Setenv("JWT_SECRET", "")
	if pt, err := DecryptValue(ct); err != nil || pt != "s3cr3t" {
		t.Fatalf("decrypt without JWT_SECRET: %q, %v", pt, err)
	}
}

func TestLegacyCiphertextStillDecrypts(t *testing.T) {
	setDataKey(t)
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("ENV", "development") // legacy mode writes the old format
	legacy, ver, err := EncryptValueVersioned("old-value")
	if err != nil || ver != LegacyKeyVersion || strings.HasPrefix(legacy, "tnk") {
		t.Fatalf("legacy encrypt: %q %d %v", legacy, ver, err)
	}
	setDataKey(t)
	if !IsLegacyCiphertext(legacy) {
		t.Fatal("not recognised as legacy")
	}
	if pt, err := DecryptValueWithVersion(legacy, LegacyKeyVersion); err != nil || pt != "old-value" {
		t.Fatalf("legacy decrypt: %q %v", pt, err)
	}
}

func TestKeyVersionSelectsKey(t *testing.T) {
	old := setDataKey(t)
	ct, _, err := EncryptValueVersioned("v1-value")
	if err != nil {
		t.Fatal(err)
	}
	// rotate to v2, v1 kept as previous
	t.Setenv("DATA_ENCRYPTION_KEY", dataKey(t))
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "2")
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS", old)
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS_VERSION", "1")
	if pt, err := DecryptValueWithVersion(ct, 1); err != nil || pt != "v1-value" {
		t.Fatalf("previous version: %q %v", pt, err)
	}
	ct2, ver2, _ := EncryptValueVersioned("v2-value")
	if ver2 != 2 {
		t.Fatalf("new version %d", ver2)
	}
	// row key_version disagreeing with the value is refused
	if _, err := DecryptValueWithVersion(ct2, 1); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("mismatched key_version: %v", err)
	}
	// previous key gone: v1 values no longer decrypt (error, not passthrough)
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS", "")
	if _, err := DecryptValue(ct); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("v1 without previous key: %v", err)
	}
}

func TestProductionWithoutDataKeyRefusesToEncrypt(t *testing.T) {
	setDataKey(t)
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	if _, err := EncryptValue("x"); err == nil {
		t.Fatal("encrypted without DATA_ENCRYPTION_KEY in production")
	}
}
