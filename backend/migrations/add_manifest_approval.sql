-- Manifest approval binding (kept in sync with versioned migration
-- 20261010_13_manifest_approval). Additive and idempotent.
--   manifest_runs.task_id:              plan_and_apply task of an approval run
--   manifest_runs.approved_bundle_hash: bundle_hash the approved plan was computed from
--   manifest_runs.approved_plan_hash:   SHA-256 of the binary plan.out that apply runs
--   manifest_runs.approved_by / _at:    approver and time
--   chk_manifest_runs_approval:         only purpose=approval, runner=agent runs can be approved

ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS task_id integer;
ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_bundle_hash character varying(64);
ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_plan_hash character varying(64);
ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_by character varying(20);
ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS approved_at timestamp with time zone;
CREATE UNIQUE INDEX IF NOT EXISTS uq_manifest_runs_task ON public.manifest_runs (task_id) WHERE task_id IS NOT NULL;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_manifest_runs_task' AND conrelid = 'public.manifest_runs'::regclass) THEN
        ALTER TABLE public.manifest_runs ADD CONSTRAINT fk_manifest_runs_task FOREIGN KEY (task_id) REFERENCES public.workspace_tasks(id) ON DELETE SET NULL;
    END IF;
END $$;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifest_runs_approval' AND conrelid = 'public.manifest_runs'::regclass) THEN
        ALTER TABLE public.manifest_runs ADD CONSTRAINT chk_manifest_runs_approval CHECK (
            (approved_at IS NULL AND approved_by IS NULL AND approved_bundle_hash IS NULL AND approved_plan_hash IS NULL)
            OR (purpose = 'approval' AND runner = 'agent'
                AND approved_at IS NOT NULL AND approved_by IS NOT NULL
                AND approved_bundle_hash = bundle_hash
                AND approved_plan_hash ~ '^[0-9a-f]{64}$'));
    END IF;
END $$;
