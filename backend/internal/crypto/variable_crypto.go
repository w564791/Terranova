package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"iac-platform/internal/keys"
)

// Value encryption (AES-256-GCM).
//
// Current format: "tnk<version>:" + base64(nonce | ciphertext), encrypted with
// version <version> of DATA_ENCRYPTION_KEY (keys.DataKeys). The key is
// independent of JWT_SECRET. Rows that store such values also record the
// version in a key_version column (workspace_variables, varset_variables).
//
// Legacy format (key version 0): base64(nonce | ciphertext) under
// SHA-256(JWT_SECRET). It is only decrypted, never written, once
// DATA_ENCRYPTION_KEY is set; the re-encryption job rewrites legacy variable
// rows. In development without DATA_ENCRYPTION_KEY (legacy mode) values are
// still written in the legacy format.

// LegacyKeyVersion the key version of legacy (JWT_SECRET-derived) ciphertexts.
const LegacyKeyVersion int16 = 0

const versionedPrefix = "tnk"

// ErrDecrypt a versioned ciphertext did not decrypt (wrong / missing key or
// tampered data). Legacy ciphertexts keep the historical lenient behaviour.
var ErrDecrypt = errors.New("value decryption failed")

// legacyKey SHA-256(JWT_SECRET): decrypt-only once DATA_ENCRYPTION_KEY is set.
func legacyKey() []byte {
	h := sha256.Sum256([]byte(os.Getenv("JWT_SECRET")))
	return h[:]
}

func gcmFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// EncryptValue encrypts plaintext with the current data key.
func EncryptValue(plaintext string) (string, error) {
	ct, _, err := EncryptValueVersioned(plaintext)
	return ct, err
}

// EncryptValueVersioned encrypts plaintext and returns the key version used
// (LegacyKeyVersion only in development legacy mode).
func EncryptValueVersioned(plaintext string) (string, int16, error) {
	if plaintext == "" {
		return "", LegacyKeyVersion, nil
	}
	if keys.LegacyEncryptionMode() {
		out, err := seal(legacyKey(), []byte(plaintext))
		if err != nil {
			return "", 0, err
		}
		return base64.StdEncoding.EncodeToString(out), LegacyKeyVersion, nil
	}
	k, err := keys.DataKeys.Current()
	if err != nil {
		return "", 0, fmt.Errorf("encrypt: %w", err)
	}
	out, err := seal(k.Material, []byte(plaintext))
	if err != nil {
		return "", 0, err
	}
	return versionedPrefix + strconv.Itoa(k.Version) + ":" + base64.StdEncoding.EncodeToString(out), int16(k.Version), nil
}

// parseVersioned splits "tnk<v>:<b64>"; ok false for anything else.
func parseVersioned(s string) (version int, payload string, ok bool) {
	rest, found := strings.CutPrefix(s, versionedPrefix)
	if !found {
		return 0, "", false
	}
	num, payload, found := strings.Cut(rest, ":")
	if !found {
		return 0, "", false
	}
	v, err := strconv.Atoi(num)
	if err != nil || v < 1 || v > 32767 {
		return 0, "", false
	}
	return v, payload, true
}

// CiphertextKeyVersion the key version of a stored value (LegacyKeyVersion
// for legacy ciphertexts and anything unversioned).
func CiphertextKeyVersion(value string) int16 {
	if v, _, ok := parseVersioned(value); ok {
		return int16(v)
	}
	return LegacyKeyVersion
}

// DecryptValue decrypts a stored value, choosing the key from the value
// itself (versioned prefix, else legacy).
func DecryptValue(ciphertext string) (string, error) {
	return DecryptValueWithVersion(ciphertext, CiphertextKeyVersion(ciphertext))
}

// DecryptValueWithVersion decrypts a value whose row records keyVersion
// (key_version column). keyVersion > 0 selects that DATA_ENCRYPTION_KEY
// version, and the value must carry the same version; keyVersion 0 is a
// legacy row (a versioned value in it is decrypted by its own prefix, e.g.
// written by a path that did not set the column).
func DecryptValueWithVersion(ciphertext string, keyVersion int16) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	v, payload, versioned := parseVersioned(ciphertext)
	if keyVersion > 0 && (!versioned || v != int(keyVersion)) {
		return "", fmt.Errorf("%w: key_version %d does not match the stored value", ErrDecrypt, keyVersion)
	}
	if versioned {
		k, err := keys.DataKeys.ByVersion(v)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrDecrypt, err)
		}
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrDecrypt, err)
		}
		gcm, err := gcmFor(k.Material)
		if err != nil {
			return "", err
		}
		if len(data) < gcm.NonceSize() {
			return "", ErrDecrypt
		}
		pt, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
		if err != nil {
			return "", ErrDecrypt
		}
		return string(pt), nil
	}
	return decryptLegacy(ciphertext)
}

// decryptLegacy the historical behaviour: values that are not valid legacy
// ciphertexts are returned unchanged (plaintext rows).
func decryptLegacy(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return ciphertext, nil
	}
	gcm, err := gcmFor(legacyKey())
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return ciphertext, nil
	}
	pt, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return ciphertext, nil
	}
	return string(pt), nil
}

// IsLegacyCiphertext whether value decrypts under the legacy key.
func IsLegacyCiphertext(value string) bool {
	if _, _, ok := parseVersioned(value); ok || value == "" {
		return false
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	gcm, err := gcmFor(legacyKey())
	if err != nil || len(data) < gcm.NonceSize() {
		return false
	}
	_, err = gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	return err == nil
}

// IsEncrypted reports whether value looks encrypted: a versioned value, or
// (historical heuristic) base64 at least one nonce long.
func IsEncrypted(value string) bool {
	if value == "" {
		return false
	}
	if _, _, ok := parseVersioned(value); ok {
		return true
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	return len(data) >= 12
}
