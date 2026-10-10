// Package keys gives access to the platform's root key material.
//
// Two independent roots, neither derived from JWT_SECRET:
//
//   - DATA_ENCRYPTION_KEY: encrypts data at rest (internal/crypto). 32 bytes.
//   - SIGNING_ROOT_KEY: the HKDF root of every JWT signing key (one key per
//     purpose, see signing.go). At least 32 bytes.
//
// JWT_SECRET is only used to read what was written before these keys existed:
// decrypting legacy ciphertexts (key version 0) and verifying legacy tokens
// without a kid until LEGACY_TOKEN_CUTOFF.
//
// Key access goes through KeyProvider. EnvKeyProvider reads the environment
// (current key plus an optional previous key for rotation); a KMS-backed
// provider can replace the package variables DataKeys / SigningRoots (see
// kms.go).
package keys

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Key is one version of a root key.
type Key struct {
	Version  int // >= 1; version 0 is reserved for legacy (JWT_SECRET-derived) data
	Material []byte
}

// KeyProvider returns root key versions.
type KeyProvider interface {
	// Current is the key new ciphertexts / signatures use. ErrKeyNotConfigured
	// when there is none.
	Current() (Key, error)
	// ByVersion returns the current or a previous (rotation) key.
	ByVersion(version int) (Key, error)
}

var (
	// ErrKeyNotConfigured the key's environment variable is not set.
	ErrKeyNotConfigured = errors.New("key not configured")
	// ErrUnknownKeyVersion no key with that version is available.
	ErrUnknownKeyVersion = errors.New("unknown key version")
)

// EnvKeyProvider reads <Name> / <Name>_VERSION (current) and
// <Name>_PREVIOUS / <Name>_PREVIOUS_VERSION (previous, during rotation).
// Values are base64 (standard or URL alphabet, padding optional) or hex.
// The environment is read on every call (no caching), so a restart with new
// variables is all a rotation needs.
type EnvKeyProvider struct {
	Name   string
	MinLen int // minimum decoded length
	MaxLen int // 0 = no maximum
}

// DataKeys is the data-encryption key provider (DATA_ENCRYPTION_KEY, exactly
// 32 bytes for AES-256).
var DataKeys KeyProvider = EnvKeyProvider{Name: "DATA_ENCRYPTION_KEY", MinLen: 32, MaxLen: 32}

// SigningRoots is the JWT signing root provider (SIGNING_ROOT_KEY).
var SigningRoots KeyProvider = EnvKeyProvider{Name: "SIGNING_ROOT_KEY", MinLen: 32}

func (p EnvKeyProvider) Current() (Key, error) {
	return p.load(p.Name, 1)
}

func (p EnvKeyProvider) ByVersion(version int) (Key, error) {
	if cur, err := p.Current(); err == nil && cur.Version == version {
		return cur, nil
	} else if err != nil && !errors.Is(err, ErrKeyNotConfigured) {
		return Key{}, err
	}
	if prev, err := p.load(p.Name+"_PREVIOUS", 0); err == nil && prev.Version == version {
		return prev, nil
	} else if err != nil && !errors.Is(err, ErrKeyNotConfigured) {
		return Key{}, err
	}
	return Key{}, fmt.Errorf("%w: %s v%d", ErrUnknownKeyVersion, p.Name, version)
}

// Configured reports whether the current key variable is set.
func (p EnvKeyProvider) Configured() bool {
	return strings.TrimSpace(os.Getenv(p.Name)) != ""
}

func (p EnvKeyProvider) load(name string, defaultVersion int) (Key, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return Key{}, fmt.Errorf("%w: %s", ErrKeyNotConfigured, name)
	}
	material, err := decodeKey(raw)
	if err != nil {
		return Key{}, fmt.Errorf("%s: %w", name, err)
	}
	if len(material) < p.MinLen || (p.MaxLen > 0 && len(material) > p.MaxLen) {
		if p.MaxLen == p.MinLen {
			return Key{}, fmt.Errorf("%s: decoded key is %d bytes, want %d", name, len(material), p.MinLen)
		}
		return Key{}, fmt.Errorf("%s: decoded key is %d bytes, want at least %d", name, len(material), p.MinLen)
	}
	version := defaultVersion
	if v := strings.TrimSpace(os.Getenv(name + "_VERSION")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 32767 {
			return Key{}, fmt.Errorf("%s_VERSION must be an integer in 1..32767", name)
		}
		version = n
	}
	if version < 1 {
		return Key{}, fmt.Errorf("%s_VERSION is required for a previous key", name)
	}
	return Key{Version: version, Material: material}, nil
}

func decodeKey(s string) ([]byte, error) {
	if b, err := hex.DecodeString(s); err == nil && len(s) >= 64 {
		return b, nil
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("key must be base64 or hex encoded")
}
