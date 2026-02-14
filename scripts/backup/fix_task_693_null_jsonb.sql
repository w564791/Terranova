-- Fix task 693 NULL JSONB fields
-- This fixes the "JSONB data is neither map nor array" error

UPDATE workspace_tasks 
SET 
  snapshot_provider_config = '{}'::jsonb,
  snapshot_variables = '{}'::jsonb,
  snapshot_resource_versions = '{}'::jsonb
WHERE id = 693;

-- Verify the fix
SELECT id, workspace_id, status, 
       snapshot_provider_config, 
       snapshot_variables, 
       snapshot_resource_versions 
FROM workspace_tasks 
WHERE id = 693;
