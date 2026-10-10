package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// workspaceTaskResourceChangeRedactionStatements is the DDL contract of
// migration 20261010_04_workspace_task_resource_change_redaction: the marker
// of the resource-change backfill (services.BackfillResourceChangeRedaction).
// NULL / lower than services.ResourceChangesRedactionVersion = values not yet
// re-derived from the redacted plan_json (or nulled). Rows written by the
// platform derivation carry the current version. details_purged = the
// backfill nulled before / after / after_unknown (no plan_json to derive
// from); set only by that path. The backfill runs in the
// application (it needs the platform-side sensitive set). Additive and
// idempotent. Keep
// backend/migrations/add_workspace_task_resource_change_redaction.sql and
// the seed block in manifests/db/init_seed_data.sql in sync.
func workspaceTaskResourceChangeRedactionStatements() []string {
	return []string{
		`ALTER TABLE public.workspace_task_resource_changes ADD COLUMN IF NOT EXISTS redaction_version smallint`,
		`ALTER TABLE public.workspace_task_resource_changes ADD COLUMN IF NOT EXISTS details_purged boolean DEFAULT false`,
	}
}

func applyWorkspaceTaskResourceChangeRedaction(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range workspaceTaskResourceChangeRedactionStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("workspace task resource change redaction marker: %w", err)
		}
	}
	return nil
}
