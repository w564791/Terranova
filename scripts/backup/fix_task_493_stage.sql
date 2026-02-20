-- Fix task 493 stage field
-- Task status is 'applied' but stage is still 'applying'
-- This causes the Apply Progress to show as pending in the UI

UPDATE workspace_tasks 
SET stage = 'applied' 
WHERE id = 493 AND status = 'applied';

-- Verify the update
SELECT id, workspace_id, task_type, status, stage, completed_at 
FROM workspace_tasks 
WHERE id = 493;
