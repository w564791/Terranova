-- Marker of the resource-change redaction backfill (kept in sync with
-- versioned migration 20261010_04_workspace_task_resource_change_redaction).
-- Additive and idempotent: existing rows get redaction_version NULL and
-- details_purged false until the application backfill
-- (services.BackfillResourceChangeRedaction) re-derives their before/after
-- from the redacted plan_json, or nulls before/after/after_unknown and sets
-- details_purged when no plan_json exists.

ALTER TABLE public.workspace_task_resource_changes ADD COLUMN IF NOT EXISTS redaction_version smallint;
ALTER TABLE public.workspace_task_resource_changes ADD COLUMN IF NOT EXISTS details_purged boolean DEFAULT false;
