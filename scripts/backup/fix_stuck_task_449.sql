-- Fix stuck task 449 that panicked before the fix was applied
-- This task got stuck in "running" status due to the panic that occurred
-- The panic has been fixed, but this task needs manual cleanup

UPDATE workspace_tasks 
SET 
    status = 'failed',
    stage = 'fetching',
    error_message = 'Task panicked: runtime error: slice bounds out of range [:16] with length 0. This issue has been fixed in the latest version.',
    completed_at = NOW(),
    updated_at = NOW()
WHERE id = 449;

-- Verify the update
SELECT id, workspace_id, status, stage, error_message, completed_at 
FROM workspace_tasks 
WHERE id = 449;
