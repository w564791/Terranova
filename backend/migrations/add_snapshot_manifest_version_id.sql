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
-- Note: this is best-effort; older tasks before this column existed
-- will have NULL (acceptable — no apply history to compare against).
UPDATE workspace_tasks t
SET snapshot_manifest_version_id = md.version_id
FROM workspaces w
JOIN manifest_deployments md ON md.id = w.manifest_deployment_id
WHERE t.workspace_id = w.workspace_id
  AND t.status = 'applied'
  AND t.snapshot_manifest_version_id IS NULL
  AND w.manifest_deployment_id IS NOT NULL;
