-- Structured task failure code (kept in sync with versioned migration
-- 20261010_01_workspace_task_error_code). Additive and idempotent: one
-- nullable column; existing rows stay NULL. Values are models.TaskErrorCode*
-- (e.g. bundle_republish_required), shown next to error_message.

ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS error_code character varying(64);
