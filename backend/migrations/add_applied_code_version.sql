-- v0.6.1: Add applied_code_version to workspace_task_resource_changes
-- Stores the resource_code_versions.version at apply time for fast lookup
-- by query_resource_code_diff, replacing slow JSONB scan on snapshot_resource_versions.

ALTER TABLE workspace_task_resource_changes
    ADD COLUMN IF NOT EXISTS applied_code_version INTEGER;

COMMENT ON COLUMN workspace_task_resource_changes.applied_code_version
    IS 'resource_code_versions.version at apply completion time';

-- Backfill from snapshot_resource_versions for existing completed records
-- module_address format: module.{resource_type}_{resource_name}
-- resource_id format: {resource_type}.{resource_name}
-- So: module_address = 'module.' || REPLACE(resource_id, '.', '_')
UPDATE workspace_task_resource_changes rc
SET applied_code_version = (snap.value->>'version')::int
FROM workspace_tasks t,
     LATERAL jsonb_each(t.snapshot_resource_versions) AS snap(key, value)
WHERE rc.task_id = t.id
  AND rc.apply_status = 'completed'
  AND rc.applied_code_version IS NULL
  AND rc.module_address = 'module.' || REPLACE(snap.key, '.', '_');
