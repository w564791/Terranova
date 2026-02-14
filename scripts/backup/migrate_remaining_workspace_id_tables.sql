-- Migration: Convert remaining workspace_id columns from INTEGER to VARCHAR(50)
-- Tables: deployments, resource_dependencies, workspace_members, workspace_permissions, 
--         workspace_permissions_backup, workspace_project_relations, workspace_resources_snapshot

BEGIN;

-- 1. deployments
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);
UPDATE deployments d SET workspace_id_new = w.workspace_id FROM workspaces w WHERE d.workspace_id = w.id;
ALTER TABLE deployments DROP COLUMN workspace_id;
ALTER TABLE deployments RENAME COLUMN workspace_id_new TO workspace_id;
ALTER TABLE deployments ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_deployments_workspace_id ON deployments(workspace_id);

-- 2. resource_dependencies
ALTER TABLE resource_dependencies ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);
UPDATE resource_dependencies rd SET workspace_id_new = w.workspace_id FROM workspaces w WHERE rd.workspace_id = w.id;
ALTER TABLE resource_dependencies DROP COLUMN workspace_id;
ALTER TABLE resource_dependencies RENAME COLUMN workspace_id_new TO workspace_id;
ALTER TABLE resource_dependencies ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_resource_dependencies_workspace_id ON resource_dependencies(workspace_id);

-- 3. workspace_members
ALTER TABLE workspace_members ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);
UPDATE workspace_members wm SET workspace_id_new = w.workspace_id FROM workspaces w WHERE wm.workspace_id = w.id;
ALTER TABLE workspace_members DROP COLUMN workspace_id;
ALTER TABLE workspace_members RENAME COLUMN workspace_id_new TO workspace_id;
ALTER TABLE workspace_members ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workspace_members_workspace_id ON workspace_members(workspace_id);

-- 4. workspace_permissions
ALTER TABLE workspace_permissions ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);
UPDATE workspace_permissions wp SET workspace_id_new = w.workspace_id FROM workspaces w WHERE wp.workspace_id = w.id;
ALTER TABLE workspace_permissions DROP COLUMN workspace_id;
ALTER TABLE workspace_permissions RENAME COLUMN workspace_id_new TO workspace_id;
ALTER TABLE workspace_permissions ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workspace_permissions_workspace_id ON workspace_permissions(workspace_id);

-- 5. workspace_permissions_backup
ALTER TABLE workspace_permissions_backup ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);
UPDATE workspace_permissions_backup wpb SET workspace_id_new = w.workspace_id FROM workspaces w WHERE wpb.workspace_id = w.id;
ALTER TABLE workspace_permissions_backup DROP COLUMN workspace_id;
ALTER TABLE workspace_permissions_backup RENAME COLUMN workspace_id_new TO workspace_id;
-- 备份表可能有 NULL 值，不强制 NOT NULL

-- 6. workspace_project_relations
ALTER TABLE workspace_project_relations ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);
UPDATE workspace_project_relations wpr SET workspace_id_new = w.workspace_id FROM workspaces w WHERE wpr.workspace_id = w.id;
ALTER TABLE workspace_project_relations DROP COLUMN workspace_id;
ALTER TABLE workspace_project_relations RENAME COLUMN workspace_id_new TO workspace_id;
ALTER TABLE workspace_project_relations ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workspace_project_relations_workspace_id ON workspace_project_relations(workspace_id);

-- 7. workspace_resources_snapshot
ALTER TABLE workspace_resources_snapshot ADD COLUMN IF NOT EXISTS workspace_id_new VARCHAR(50);
UPDATE workspace_resources_snapshot wrs SET workspace_id_new = w.workspace_id FROM workspaces w WHERE wrs.workspace_id = w.id;
ALTER TABLE workspace_resources_snapshot DROP COLUMN workspace_id;
ALTER TABLE workspace_resources_snapshot RENAME COLUMN workspace_id_new TO workspace_id;
ALTER TABLE workspace_resources_snapshot ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workspace_resources_snapshot_workspace_id ON workspace_resources_snapshot(workspace_id);

COMMIT;

-- Verification
SELECT 'Migration completed' AS status;
SELECT table_name, COUNT(*) as records 
FROM (
  SELECT 'deployments' as table_name, COUNT(*) FROM deployments
  UNION ALL SELECT 'resource_dependencies', COUNT(*) FROM resource_dependencies
  UNION ALL SELECT 'workspace_members', COUNT(*) FROM workspace_members
  UNION ALL SELECT 'workspace_permissions', COUNT(*) FROM workspace_permissions
  UNION ALL SELECT 'workspace_project_relations', COUNT(*) FROM workspace_project_relations
  UNION ALL SELECT 'workspace_resources_snapshot', COUNT(*) FROM workspace_resources_snapshot
) t
GROUP BY table_name
ORDER BY table_name;
