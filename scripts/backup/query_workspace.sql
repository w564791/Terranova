-- 查询workspace基本信息
SELECT 
    id,
    name,
    description,
    execution_mode,
    terraform_version,
    state,
    ui_mode,
    created_at,
    updated_at
FROM workspaces
WHERE id = 12;

-- 查询workspace的所有任务
SELECT 
    id,
    task_type,
    status,
    stage,
    description,
    changes_add,
    changes_change,
    changes_destroy,
    created_at,
    completed_at,
    duration
FROM workspace_tasks
WHERE workspace_id = 12
ORDER BY created_at DESC
LIMIT 10;

-- 查询特定任务的资源变更
SELECT 
    id,
    resource_address,
    resource_type,
    action,
    apply_status,
    created_at
FROM workspace_task_resource_changes
WHERE task_id = 210
ORDER BY id;

-- 统计资源变更
SELECT 
    action,
    COUNT(*) as count
FROM workspace_task_resource_changes
WHERE task_id = 210
GROUP BY action;
