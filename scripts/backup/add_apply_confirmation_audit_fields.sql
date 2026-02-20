-- Add audit fields to track who confirmed the apply and when
-- This helps prevent and detect unauthorized apply executions

ALTER TABLE workspace_tasks 
ADD COLUMN IF NOT EXISTS apply_confirmed_by VARCHAR(255),
ADD COLUMN IF NOT EXISTS apply_confirmed_at TIMESTAMP;

COMMENT ON COLUMN workspace_tasks.apply_confirmed_by IS 'User ID who confirmed the apply via ConfirmApply API';
COMMENT ON COLUMN workspace_tasks.apply_confirmed_at IS 'Timestamp when apply was confirmed by user';

-- Create index for audit queries
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_apply_confirmed 
ON workspace_tasks(apply_confirmed_by, apply_confirmed_at) 
WHERE apply_confirmed_by IS NOT NULL;

-- Verify the changes
SELECT column_name, data_type, is_nullable, column_default
FROM information_schema.columns
WHERE table_name = 'workspace_tasks' 
  AND column_name IN ('apply_confirmed_by', 'apply_confirmed_at')
ORDER BY ordinal_position;
