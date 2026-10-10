package migration

import (
	"context"

	"gorm.io/gorm"
)

// variableKeyVersionStatements adds the per-row key_version column read by
// internal/crypto.DecryptValueWithVersion. Existing rows get 0 (legacy key);
// services.ReencryptLegacyVariables rewrites them with DATA_ENCRYPTION_KEY.
// The re-encryption is not part of this schema migration: the migrate job need
// not hold the data key.
func variableKeyVersionStatements() []string {
	return []string{
		"ALTER TABLE workspace_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0",
		"ALTER TABLE varset_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0",
	}
}

func applyVariableKeyVersion(ctx context.Context, tx *gorm.DB) error {
	for _, s := range variableKeyVersionStatements() {
		if err := tx.WithContext(ctx).Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}
