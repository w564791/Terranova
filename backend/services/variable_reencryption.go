package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"iac-platform/internal/crypto"
	"iac-platform/internal/keys"

	"gorm.io/gorm"
)

// VariableReencryptionResult counts of one ReencryptLegacyVariables pass.
type VariableReencryptionResult struct {
	Reencrypted int // legacy ciphertexts rewritten with DATA_ENCRYPTION_KEY
	Encrypted   int // sensitive rows that held plaintext, now encrypted
	Relabelled  int // already-versioned values whose key_version column was 0
}

// reencryptTables the tables holding encrypted variable values.
var reencryptTables = []string{"workspace_variables", "varset_variables"}

// ReencryptLegacyVariables rewrites sensitive variable rows still on the
// legacy key (key_version = 0) with the current DATA_ENCRYPTION_KEY.
//
// Idempotent and safe to run concurrently / repeatedly: only rows with
// key_version = 0 are read, and each row is updated with a compare-and-set on
// (id, key_version = 0, value), so a row changed in between is left for the
// next pass and a finished row is never touched again. Rows written by the new
// code already carry key_version >= 1 and are skipped.
//
// Classification of a key_version 0 row uses explicit markers only, never the
// shape of the value:
//   - "tnk<v>:" prefix: already encrypted, only the column is set (Relabelled);
//   - authenticates (AES-GCM) under the legacy key SHA-256(JWT_SECRET): legacy
//     ciphertext, decrypted and re-encrypted (Reencrypted);
//   - anything else, including values that are valid base64: plaintext that
//     was stored unencrypted, encrypted now (Encrypted).
//
// JWT_SECRET must therefore be the environment's real legacy secret: with a
// wrong one, legacy ciphertexts would be classified as plaintext and wrapped
// (still recoverable by decrypting and opening them with the right key). The
// job refuses to classify unprefixed rows when JWT_SECRET is not set.
//
// No-op in legacy encryption mode (no DATA_ENCRYPTION_KEY).
func ReencryptLegacyVariables(ctx context.Context, db *gorm.DB, batchSize int) (VariableReencryptionResult, error) {
	var res VariableReencryptionResult
	if keys.LegacyEncryptionMode() {
		return res, nil
	}
	if _, err := keys.DataKeys.Current(); err != nil {
		return res, err
	}
	if batchSize <= 0 {
		batchSize = 200
	}
	type row struct {
		ID    uint
		Value string
	}
	for _, table := range reencryptTables {
		var lastID uint
		for {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			var rows []row
			if err := db.WithContext(ctx).Raw(fmt.Sprintf(
				`SELECT id, value FROM %s WHERE sensitive AND key_version = 0 AND COALESCE(value, '') <> '' AND id > ? ORDER BY id LIMIT ?`, table),
				lastID, batchSize).Scan(&rows).Error; err != nil {
				return res, fmt.Errorf("%s: %w", table, err)
			}
			if len(rows) == 0 {
				break
			}
			for _, r := range rows {
				lastID = r.ID
				var (
					newValue   string
					newVersion int16
					counter    *int
				)
				switch {
				case crypto.CiphertextKeyVersion(r.Value) > 0:
					newValue, newVersion, counter = r.Value, crypto.CiphertextKeyVersion(r.Value), &res.Relabelled
				case os.Getenv("JWT_SECRET") == "":
					return res, fmt.Errorf("%s id %d: JWT_SECRET is required to tell legacy ciphertexts from plaintext", table, r.ID)
				case crypto.IsLegacyCiphertext(r.Value):
					pt, err := crypto.DecryptValueWithVersion(r.Value, crypto.LegacyKeyVersion)
					if err != nil {
						return res, err
					}
					if newValue, newVersion, err = crypto.EncryptValueVersioned(pt); err != nil {
						return res, err
					}
					counter = &res.Reencrypted
				default: // plaintext (failed authentication), whatever its shape
					var err error
					if newValue, newVersion, err = crypto.EncryptValueVersioned(r.Value); err != nil {
						return res, err
					}
					counter = &res.Encrypted
				}
				if newVersion < 1 {
					return res, errors.New("re-encryption produced a legacy value")
				}
				tx := db.WithContext(ctx).Exec(fmt.Sprintf(
					`UPDATE %s SET value = ?, key_version = ? WHERE id = ? AND key_version = 0 AND value = ?`, table),
					newValue, newVersion, r.ID, r.Value)
				if tx.Error != nil {
					return res, fmt.Errorf("%s id %d: %w", table, r.ID, tx.Error)
				}
				if tx.RowsAffected == 1 {
					*counter++
				}
			}
		}
	}
	return res, nil
}

// RunVariableReencryption one background pass, logged (leader only).
func RunVariableReencryption(ctx context.Context, db *gorm.DB) {
	res, err := ReencryptLegacyVariables(ctx, db, 200)
	if err != nil {
		log.Printf("[Keys] variable re-encryption failed: %v", err)
		return
	}
	remaining, err := CountLegacyVariableRows(ctx, db)
	if err != nil {
		log.Printf("[Keys] counting legacy variable rows failed: %v", err)
	}
	var left int64
	for _, n := range remaining {
		left += n
	}
	log.Printf("[Keys] variable re-encryption: reencrypted=%d encrypted(plaintext)=%d relabelled=%d; legacy rows remaining=%d %v",
		res.Reencrypted, res.Encrypted, res.Relabelled, left, remaining)
}

// CountLegacyVariableRows sensitive variable rows not yet on a data key
// (key_version = 0, non-empty value), per table: legacy ciphertexts and
// plaintext sensitive values alike (telling them apart needs JWT_SECRET).
func CountLegacyVariableRows(ctx context.Context, db *gorm.DB) (map[string]int64, error) {
	out := make(map[string]int64, len(reencryptTables))
	for _, table := range reencryptTables {
		var n int64
		if err := db.WithContext(ctx).Raw(fmt.Sprintf(
			`SELECT COUNT(*) FROM %s WHERE sensitive AND key_version = 0 AND COALESCE(value, '') <> ''`, table)).
			Scan(&n).Error; err != nil {
			return nil, fmt.Errorf("%s: %w", table, err)
		}
		out[table] = n
	}
	return out, nil
}

// CheckLegacyKeyAvailable refuses startup when sensitive rows with
// key_version 0 remain (legacy ciphertexts, or plaintext not yet encrypted)
// but JWT_SECRET is not set: without it a legacy ciphertext cannot be told
// from plaintext, so neither can be read correctly nor re-encrypted. This does
// not depend on LEGACY_TOKEN_CUTOFF: data stays legacy until the
// re-encryption job has rewritten it, however late that is.
func CheckLegacyKeyAvailable(ctx context.Context, db *gorm.DB) error {
	if os.Getenv("JWT_SECRET") != "" {
		return nil
	}
	counts, err := CountLegacyVariableRows(ctx, db)
	if err != nil {
		return fmt.Errorf("cannot check for legacy-encrypted rows: %w", err)
	}
	var parts []string
	for _, table := range reencryptTables {
		if counts[table] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", table, counts[table]))
		}
	}
	if len(parts) > 0 {
		return fmt.Errorf("JWT_SECRET is not set but sensitive rows with key_version 0 (legacy ciphertext or plaintext) remain (%s); "+
			"set JWT_SECRET until the re-encryption job reports zero legacy rows", strings.Join(parts, ", "))
	}
	return nil
}
