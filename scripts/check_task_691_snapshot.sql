-- 检查任务691的快照数据
SELECT 
    id,
    workspace_id,
    task_type,
    status,
    stage,
    plan_task_id,
    snapshot_created_at,
    jsonb_pretty(snapshot_variables) as snapshot_variables,
    jsonb_array_length(snapshot_variables) as variable_count
FROM workspace_tasks 
WHERE id = 691;

-- 检查快照变量的第一个元素,看看包含哪些字段
SELECT 
    id,
    jsonb_array_element(snapshot_variables, 0) as first_variable
FROM workspace_tasks 
WHERE id = 691;
