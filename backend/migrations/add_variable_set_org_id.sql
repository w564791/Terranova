-- variable_sets.org_id scoping (kept in sync with versioned migration
-- 20261016_01_variable_set_org_id). Additive and idempotent.
--
-- Backfill (applied by the Go migration, not this SQL file alone):
--   1. Assigned, all mounts in one org → that org_id
--   2. Cross-org mounts: org with most mounts; drop other-org mounts (WARN)
--   3. Unassigned + created_by: creator's earliest org with VARIABLE_SETS WRITE
--   4. global: org_id stays NULL
-- Orphans (specific + NULL) stay visible only to creator until first mount.

ALTER TABLE public.variable_sets ADD COLUMN IF NOT EXISTS org_id bigint;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
     WHERE conname = 'chk_variable_sets_org_scope'
       AND conrelid = 'public.variable_sets'::regclass
  ) THEN
    ALTER TABLE public.variable_sets
      ADD CONSTRAINT chk_variable_sets_org_scope CHECK (
        (scope = 'global' AND org_id IS NULL)
        OR (scope = 'specific')
      );
  END IF;
END $$;

DROP INDEX IF EXISTS public.idx_variable_sets_name;

CREATE UNIQUE INDEX IF NOT EXISTS idx_variable_sets_org_name
  ON public.variable_sets (org_id, name)
  WHERE is_deleted = false AND org_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_variable_sets_global_name
  ON public.variable_sets (name)
  WHERE is_deleted = false AND org_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_variable_sets_org_id
  ON public.variable_sets (org_id) WHERE org_id IS NOT NULL;

COMMENT ON COLUMN public.variable_sets.org_id IS 'Owning organization for specific varsets; NULL for global (and legacy unassigned orphans pending first mount)';
