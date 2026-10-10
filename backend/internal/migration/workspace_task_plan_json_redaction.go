package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// workspaceTaskPlanJSONRedactionStatements is the DDL contract of migration
// 20261010_02_workspace_task_plan_json_redaction: the marker of the
// historical plan_json backfill (services.BackfillPlanJSONRedaction). NULL /
// lower than services.PlanJSONRedactionVersion = not yet rewritten by the
// backfill. The backfill itself runs in the application (it needs the
// platform-side sensitive set and decrypted variables), in batches, in the
// background. Additive and idempotent. Keep
// backend/migrations/add_workspace_task_plan_json_redaction.sql and the seed
// block in manifests/db/init_seed_data.sql in sync.
func workspaceTaskPlanJSONRedactionStatements() []string {
	return []string{
		`ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS plan_json_redaction_version smallint`,
	}
}

func applyWorkspaceTaskPlanJSONRedaction(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range workspaceTaskPlanJSONRedactionStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("workspace task plan_json redaction marker: %w", err)
		}
	}
	return nil
}
