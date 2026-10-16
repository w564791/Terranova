-- Read-only: remaining variable_sets orphans (scope=specific, org_id NULL).
-- When orphans_total = 0, run (ops):
--   ALTER TABLE public.variable_sets VALIDATE CONSTRAINT chk_variable_sets_specific_org;
-- Usage: psql "$DSN" -f scripts/varset_orphan_stats.sql
SELECT COUNT(*)                                          AS orphans_total,
       COUNT(*) FILTER (WHERE is_deleted = false)        AS orphans_active,
       COUNT(*) FILTER (WHERE is_deleted = true)         AS orphans_deleted,
       COUNT(*) FILTER (WHERE created_by IS NULL)        AS orphans_no_creator,
       (SELECT convalidated FROM pg_constraint
         WHERE conname = 'chk_variable_sets_specific_org'
           AND conrelid = 'public.variable_sets'::regclass) AS constraint_validated
  FROM public.variable_sets
 WHERE scope = 'specific' AND org_id IS NULL;
