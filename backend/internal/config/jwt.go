package config

import (
	"os"

	"iac-platform/internal/keys"
)

// GetJWTSecret the legacy JWT_SECRET.
//
// New tokens are signed with keys derived from SIGNING_ROOT_KEY
// (internal/keys); JWT_SECRET only verifies tokens without kid during the
// LEGACY_TOKEN_CUTOFF window, decrypts legacy variable ciphertexts, and signs
// in development legacy mode (no SIGNING_ROOT_KEY). It is required (panic
// when unset) only in that legacy mode; otherwise "" is returned when unset.
func GetJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" && keys.LegacySigningMode() {
		panic("JWT_SECRET environment variable is required but not set")
	}
	return secret
}
