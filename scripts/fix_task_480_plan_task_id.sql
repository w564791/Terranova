-- 修复任务480的plan_task_id问题
-- 问题：plan_and_apply任务的plan_task_id为NULL，导致无法执行apply

-- 1. 查看任务480当前状态
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    execution_mode,
    plan_task_id,
    created_at,
    updated_at
FROM workspace_tasks
WHERE id = 480;

-- 2. 修复：设置plan_task_id为任务自身ID
UPDATE workspace_tasks 
SET plan_task_id = 480,
    updated_at = NOW()
WHERE id = 480 
  AND task_type = 'plan_and_apply'
  AND plan_task_id IS NULL;

-- 3. 验证修复结果
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    plan_task_id,
    'Fixed' as result
FROM workspace_tasks
WHERE id = 480;

-- 4. 查找其他可能有相同问题的任务
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
ORDER BY created_at DESC;
