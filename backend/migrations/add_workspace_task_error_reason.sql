-- Short machine reason next to the structured task failure code (kept in
-- sync with versioned migration 20261010_03_workspace_task_error_reason).
-- Additive and idempotent: one nullable column; existing rows stay NULL.
-- Values are rule / capability names only (e.g. denylisted_file,
-- hash_mismatch, manifest_bundle_v1), never paths or content.

ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS error_reason character varying(64);
