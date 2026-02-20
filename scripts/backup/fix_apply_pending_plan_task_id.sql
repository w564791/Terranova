-- 修复 apply_pending 状态任务的 plan_task_id
-- 问题：旧代码中 plan_and_apply 任务在 Plan 完成后变为 apply_pending，但 plan_task_id 未设置
-- 影响：Server 重启后这些任务无法执行 Apply（报错：apply task has no associated plan task）
-- 解决：将 plan_task_id 设置为指向自己（plan_and_apply 任务的 plan 数据在自己身上）

-- 查看需要修复的任务
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

-- 修复：将 plan_task_id 设置为指向自己
UPDATE workspace_tasks
SET plan_task_id = id
WHERE task_type = 'plan_and_apply'
  AND status = 'apply_pending'
  AND plan_task_id IS NULL;

-- 验证修复结果
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
ORDER BY created_at DESC;
