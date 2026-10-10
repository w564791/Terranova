package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// manifestApprovalStatements is the DDL contract of migration
// 20261010_13_manifest_approval (spec §9 step 7, approval + apply double hash):
//   - manifest_runs.task_id: the plan_and_apply task an approval run belongs
//     to (at most one run per task; FK ON DELETE SET NULL);
//   - manifest_runs.approved_bundle_hash / approved_plan_hash / approved_by /
//     approved_at: what the approver approved (bundle_hash of the bundle the
//     plan was computed from, SHA-256 of the binary plan.out that apply runs);
//   - chk_manifest_runs_approval: only purpose=approval, runner=agent runs can
//     carry an approval, the approved bundle is the run's own bundle and all
//     approval columns are set together. Sandbox / preview runs can never be
//     approved, whatever writes the row.
//
// Additive and idempotent (constraints added in DO blocks guarded by
// pg_constraint). Keep backend/migrations/add_manifest_approval.sql and the
// seed block in manifests/db/init_seed_data.sql in sync.
func manifestApprovalStatements() []string {
	return []string{
		`ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS task_id integer`,
		`ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_bundle_hash character varying(64)`,
		`ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_plan_hash character varying(64)`,
		`ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_by character varying(20)`,
		`ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_at timestamp with time zone`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_manifest_runs_task ON public.manifest_runs (task_id) WHERE task_id IS NOT NULL`,
		`DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_manifest_runs_task' AND conrelid = 'public.manifest_runs'::regclass) THEN
        ALTER TABLE public.manifest_runs ADD CONSTRAINT fk_manifest_runs_task FOREIGN KEY (task_id) REFERENCES public.workspace_tasks(id) ON DELETE SET NULL;
    END IF;
END $$`,
		`DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifest_runs_approval' AND conrelid = 'public.manifest_runs'::regclass) THEN
        ALTER TABLE public.manifest_runs ADD CONSTRAINT chk_manifest_runs_approval CHECK (
            (approved_at IS NULL AND approved_by IS NULL AND approved_bundle_hash IS NULL AND approved_plan_hash IS NULL)
            OR (purpose = 'approval' AND runner = 'agent'
                AND approved_at IS NOT NULL AND approved_by IS NOT NULL
                AND approved_bundle_hash = bundle_hash
                AND approved_plan_hash ~ '^[0-9a-f]{64}$'));
    END IF;
END $$`,
	}
}

func applyManifestApproval(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range manifestApprovalStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("manifest approval: %w", err)
		}
	}
	return nil
}
