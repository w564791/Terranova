-- Marker of the historical plan_json redaction backfill (kept in sync with
-- versioned migration 20261010_02_workspace_task_plan_json_redaction).
-- Additive and idempotent: one nullable column. NULL = plan_json not yet
-- rewritten by services.BackfillPlanJSONRedaction (runs in the application,
-- in the background, safe to re-run).

ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS plan_json_redaction_version smallint;
