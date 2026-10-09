package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// workspaceTaskErrorCodeStatements is the DDL contract of migration
// 20261010_01_workspace_task_error_code: the structured failure code of a
// task (models.TaskErrorCode*, e.g. bundle_republish_required), next to the
// free-text error_message. Additive and idempotent; existing rows stay NULL.
// Keep backend/migrations/add_workspace_task_error_code.sql and the seed
// block in manifests/db/init_seed_data.sql in sync
// (TestWorkspaceTaskErrorCodeSQLInSync).
func workspaceTaskErrorCodeStatements() []string {
	return []string{
		`ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS error_code character varying(64)`,
	}
}

func applyWorkspaceTaskErrorCode(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range workspaceTaskErrorCodeStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("workspace task error_code: %w", err)
		}
	}
	return nil
}
