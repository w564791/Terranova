-- variable_sets: specific ⇒ org_id NOT NULL (kept in sync with versioned
-- migration 20261016_02_variable_set_org_required). NOT VALID: legacy orphans
-- are healed on write; when scripts/varset_orphan_stats.sql reports 0, run
--   ALTER TABLE public.variable_sets VALIDATE CONSTRAINT chk_variable_sets_specific_org;
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
     WHERE conname = 'chk_variable_sets_specific_org'
       AND conrelid = 'public.variable_sets'::regclass
  ) THEN
    ALTER TABLE public.variable_sets
      ADD CONSTRAINT chk_variable_sets_specific_org
      CHECK (scope <> 'specific' OR org_id IS NOT NULL) NOT VALID;
  END IF;
END $$;
