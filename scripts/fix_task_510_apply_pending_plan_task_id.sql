-- Fix plan_task_id for plan_and_apply tasks in apply_pending status
-- This script fixes tasks where plan_task_id is NULL but should point to itself

-- Display tasks that will be fixed
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    plan_task_id,
    created_at
FROM workspace_tasks
WHERE task_type = 'plan_and_apply'
  AND status = 'apply_pending'
  AND plan_task_id IS NULL
ORDER BY id;

-- Fix the tasks
UPDATE workspace_tasks
SET plan_task_id = id
WHERE task_type = 'plan_and_apply'
  AND status = 'apply_pending'
  AND plan_task_id IS NULL;

-- Verify the fix
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    plan_task_id,
    created_at
FROM workspace_tasks
WHERE task_type = 'plan_and_apply'
  AND status = 'apply_pending'
ORDER BY id;
