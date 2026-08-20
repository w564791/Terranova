-- Multi-tenant hardening: workspace one-project binding + role tenantization
-- Safe to re-run (IF NOT EXISTS / exception blocks).

-- 1) workspace_project_relations: one project per workspace
-- Never silently keep an arbitrary project: that changes tenant ownership.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
      FROM workspace_project_relations
     GROUP BY workspace_id
    HAVING COUNT(*) > 1
  ) THEN
    RAISE EXCEPTION 'workspace_project_relations contains duplicate workspace ownership; resolve manually before applying this migration';
  END IF;
EXCEPTION
  WHEN undefined_table THEN NULL;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_workspace_project_relations_workspace_id
  ON workspace_project_relations (workspace_id);

-- 2) iam_roles.org_id: 0 = platform/system role; >0 = tenant custom role
ALTER TABLE iam_roles ADD COLUMN IF NOT EXISTS org_id BIGINT NOT NULL DEFAULT 0;

-- Drop legacy global unique on name if present (name varies by install)
DO $$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT c.conname
    FROM pg_constraint c
    JOIN pg_class t ON c.conrelid = t.oid
    WHERE t.relname = 'iam_roles'
      AND c.contype = 'u'
      AND pg_get_constraintdef(c.oid) ILIKE '%(name)%'
      AND pg_get_constraintdef(c.oid) NOT ILIKE '%org_id%'
  LOOP
    EXECUTE format('ALTER TABLE iam_roles DROP CONSTRAINT %I', r.conname);
  END LOOP;
EXCEPTION
  WHEN undefined_table THEN NULL;
END $$;

DROP INDEX IF EXISTS idx_iam_roles_name;
DROP INDEX IF EXISTS iam_roles_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_role_org_name
  ON iam_roles (org_id, name);

-- System roles stay org_id=0
UPDATE iam_roles SET org_id = 0 WHERE is_system = true;
