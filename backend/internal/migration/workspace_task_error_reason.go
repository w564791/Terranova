package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// workspaceTaskErrorReasonStatements is the DDL contract of migration
// 20261010_03_workspace_task_error_reason: a short machine reason next to
// error_code (a rule / capability name such as denylisted_file,
// hash_mismatch, manifest_bundle_v1; never paths or content). Additive and
// idempotent; existing rows stay NULL. Keep
// backend/migrations/add_workspace_task_error_reason.sql and the seed block
// in manifests/db/init_seed_data.sql in sync.
func workspaceTaskErrorReasonStatements() []string {
	return []string{
		`ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS error_reason character varying(64)`,
	}
}

func applyWorkspaceTaskErrorReason(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range workspaceTaskErrorReasonStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("workspace task error_reason: %w", err)
		}
	}
	return nil
}
