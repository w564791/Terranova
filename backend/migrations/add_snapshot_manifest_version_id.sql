-- Add snapshot_manifest_version_id to workspace_tasks
-- Stores the manifest_deployments.version_id at plan time for manifest-managed
-- workspaces, enabling query_resource_code_diff to compare HCL between
-- the applied version and the current version.

ALTER TABLE workspace_tasks
    ADD COLUMN IF NOT EXISTS snapshot_manifest_version_id VARCHAR(36);

COMMENT ON COLUMN workspace_tasks.snapshot_manifest_version_id
    IS 'manifest version ID at plan time for manifest-managed workspaces';

-- Backfill: for existing applied tasks in manifest-managed workspaces,
-- set snapshot_manifest_version_id from current deployment version.
--
-- LIMITATION: uses CURRENT deployment version, not the version at the time
-- of each task. After an upgrade (v1→v2), old v1 tasks will be backfilled
-- with v2's version_id, making code diff report "code unchanged" until
-- the next apply creates a task with the correct snapshot. This is
-- acceptable because:
-- 1. Only affects the transition period after migration
-- 2. Next plan+apply cycle creates correct snapshots automatically
-- 3. Older tasks before this column existed have NULL (no history to compare)
UPDATE workspace_tasks t
SET snapshot_manifest_version_id = md.version_id
FROM workspaces w
JOIN manifest_deployments md ON md.id = w.manifest_deployment_id
WHERE t.workspace_id = w.workspace_id
  AND t.status = 'applied'
  AND t.snapshot_manifest_version_id IS NULL
  AND w.manifest_deployment_id IS NOT NULL;
