-- 检查最新任务的变量快照格式
-- 用于验证变量快照优化是否生效

-- 查询最新的任务及其快照
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    created_at,
    snapshot_created_at,
    jsonb_pretty(snapshot_variables) as snapshot_variables_formatted
FROM workspace_tasks
WHERE workspace_id = 'ws-mb7m9ii5ey'
ORDER BY id DESC
LIMIT 3;

-- 检查快照中的字段
SELECT 
    id,
    jsonb_array_length(snapshot_variables) as variable_count,
    jsonb_array_elements(snapshot_variables) as variable_snapshot
FROM workspace_tasks
WHERE workspace_id = 'ws-mb7m9ii5ey'
  AND snapshot_variables IS NOT NULL
ORDER BY id DESC
LIMIT 1;
