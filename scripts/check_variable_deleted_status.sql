-- 查询workspace中所有变量的删除状态

SELECT 
    variable_id,
    key,
    version,
    is_deleted,
    variable_type
FROM workspace_variables
WHERE workspace_id = 'ws-mb7m9ii5ey'
ORDER BY variable_id, version;

-- 只查询未删除的最新版本
SELECT 
    variable_id,
    key,
    version,
    is_deleted,
    variable_type
FROM workspace_variables wv
WHERE wv.workspace_id = 'ws-mb7m9ii5ey' 
  AND wv.is_deleted = false
  AND wv.version = (
    SELECT MAX(version)
    FROM workspace_variables
    WHERE workspace_id = wv.workspace_id 
      AND variable_id = wv.variable_id
      AND is_deleted = false
  )
ORDER BY variable_id;
