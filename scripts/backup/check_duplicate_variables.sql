-- 检查是否有重复的变量（同一个 variable_id 有多个未删除的版本）
SELECT 
    workspace_id,
    variable_id,
    key,
    COUNT(*) as version_count,
    STRING_AGG(version::text, ', ' ORDER BY version) as versions,
    STRING_AGG(CASE WHEN is_deleted THEN 'deleted' ELSE 'active' END, ', ' ORDER BY version) as statuses
FROM workspace_variables
GROUP BY workspace_id, variable_id, key
HAVING COUNT(*) > 1
ORDER BY workspace_id, key;

-- 检查是否有同一个 workspace 中 key 相同但 variable_id 不同的情况
SELECT 
    workspace_id,
    key,
    COUNT(DISTINCT variable_id) as variable_id_count,
    STRING_AGG(DISTINCT variable_id, ', ') as variable_ids,
    STRING_AGG(DISTINCT version::text, ', ') as versions
FROM workspace_variables
WHERE is_deleted = false
GROUP BY workspace_id, key
HAVING COUNT(DISTINCT variable_id) > 1
ORDER BY workspace_id, key;

-- 查看具体的重复变量详情
SELECT 
    id,
    workspace_id,
    variable_id,
    key,
    version,
    is_deleted,
    created_at,
    updated_at
FROM workspace_variables
WHERE workspace_id IN (
    SELECT workspace_id 
    FROM workspace_variables 
    WHERE is_deleted = false
    GROUP BY workspace_id, key
    HAVING COUNT(*) > 1
)
ORDER BY workspace_id, key, version;
