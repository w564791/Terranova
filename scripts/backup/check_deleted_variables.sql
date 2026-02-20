-- 检查已删除的变量

-- 1. 查看所有变量（包括已删除的）
SELECT 
    id,
    variable_id,
    key,
    version,
    is_deleted,
    created_at
FROM workspace_variables
ORDER BY variable_id, version;

-- 2. 查看每个 variable_id 的最新版本
SELECT 
    variable_id,
    key,
    MAX(version) as latest_version,
    MAX(CASE WHEN version = (SELECT MAX(v2.version) FROM workspace_variables v2 WHERE v2.variable_id = workspace_variables.variable_id) 
        THEN is_deleted ELSE NULL END) as is_latest_deleted
FROM workspace_variables
GROUP BY variable_id, key
ORDER BY variable_id;

-- 3. 模拟 ListVariables 的查询
WITH latest_versions AS (
    SELECT variable_id, MAX(version) as max_version
    FROM workspace_variables
    WHERE workspace_id = 'ws-mb7m9ii5ey' AND is_deleted = false
    GROUP BY variable_id
)
SELECT 
    wv.id,
    wv.variable_id,
    wv.key,
    wv.version,
    wv.is_deleted
FROM workspace_variables wv
INNER JOIN latest_versions lv 
    ON wv.variable_id = lv.variable_id 
    AND wv.version = lv.max_version
WHERE wv.workspace_id = 'ws-mb7m9ii5ey' 
    AND wv.is_deleted = false
ORDER BY wv.key;
