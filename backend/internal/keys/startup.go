package keys

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// LegacyMode reports that no SIGNING_ROOT_KEY / DATA_ENCRYPTION_KEY is
// configured outside production: everything behaves as before these keys
// existed (JWT_SECRET signs and encrypts). Never in production: CheckStartup
// refuses to start there.
func LegacySigningMode() bool {
	return !configured(SigningRoots) && !isProduction()
}

// LegacyEncryptionMode see LegacySigningMode, for DATA_ENCRYPTION_KEY.
func LegacyEncryptionMode() bool {
	return !configured(DataKeys) && !isProduction()
}

func configured(p KeyProvider) bool {
	_, err := p.Current()
	return !errors.Is(err, ErrKeyNotConfigured)
}

func isProduction() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")
}

// CheckStartup validates the key configuration. In production a missing
// DATA_ENCRYPTION_KEY or SIGNING_ROOT_KEY is an error (refuse to start); in
// development it is a warning and the legacy JWT_SECRET scheme is used. A key
// that is set but malformed, or a malformed LEGACY_TOKEN_CUTOFF /
// LEGACY_TOKEN_ISSUED_BEFORE, is always an error.
func CheckStartup() (warnings []string, err error) {
	var errs []error
	for _, k := range []struct {
		name string
		p    KeyProvider
	}{{"DATA_ENCRYPTION_KEY", DataKeys}, {"SIGNING_ROOT_KEY", SigningRoots}} {
		_, e := k.p.Current()
		switch {
		case errors.Is(e, ErrKeyNotConfigured) && isProduction():
			errs = append(errs, fmt.Errorf("%s is required in production (ENV=production)", k.name))
		case errors.Is(e, ErrKeyNotConfigured):
			warnings = append(warnings, fmt.Sprintf("%s is not set: LEGACY MODE, JWT_SECRET is used (development only, refused in production)", k.name))
		case e != nil:
			errs = append(errs, e)
		}
		if p, ok := k.p.(EnvKeyProvider); ok && os.Getenv(p.Name+"_PREVIOUS") != "" {
			if _, e := p.load(p.Name+"_PREVIOUS", 0); e != nil {
				errs = append(errs, e)
			}
		}
	}
	if LegacyEncryptionMode() && strings.TrimSpace(os.Getenv("JWT_SECRET")) == "" {
		errs = append(errs, errors.New("JWT_SECRET is required while DATA_ENCRYPTION_KEY is not set (legacy encryption mode)"))
	}
	cutoff, cutoffErr := legacyCutoff()
	if cutoffErr != nil {
		errs = append(errs, cutoffErr)
	} else if cutoff.IsZero() && !LegacySigningMode() {
		warnings = append(warnings, "LEGACY_TOKEN_CUTOFF is not set: tokens without kid (signed with JWT_SECRET) are rejected")
	} else if !cutoff.IsZero() && time.Now().After(cutoff) {
		warnings = append(warnings, "LEGACY_TOKEN_CUTOFF has passed: remove it, rotate JWT_SECRET out of the deployment")
	}
	if _, e := legacyIssuedBefore(); e != nil {
		errs = append(errs, e)
	}
	return warnings, errors.Join(errs...)
}

// IsProduction reports ENV=production, the single mode switch for the
// startup checks (keys, TLS trust).
func IsProduction() bool { return isProduction() }
