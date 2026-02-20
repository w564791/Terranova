-- Fix resource apply_status for task 493
-- When task is applied, all resources should be marked as completed
-- Frontend only supports: pending, applying, completed, failed

UPDATE workspace_task_resource_changes 
SET apply_status = 'completed',
    apply_completed_at = NOW()
WHERE task_id = 493 
  AND apply_status = 'pending';

-- Verify the update
SELECT id, resource_address, action, apply_status, apply_completed_at 
FROM workspace_task_resource_changes 
WHERE task_id = 493;
