-- Add is_temp column to workspace_state_versions
-- Used by StateFileWatcher to mark intermediate state during terraform apply

ALTER TABLE workspace_state_versions
ADD COLUMN is_temp BOOLEAN DEFAULT FALSE;

-- Partial index: only index temp records for fast lookup/cleanup
CREATE INDEX idx_state_versions_temp
ON workspace_state_versions(workspace_id, is_temp)
WHERE is_temp = true;

-- Ensure existing records are explicitly non-temp
UPDATE workspace_state_versions SET is_temp = FALSE WHERE is_temp IS NULL;
