-- 调试脚本：查看当前数据库状态

-- 1. 查看所有变量
SELECT 
    id,
    variable_id,
    workspace_id,
    key,
    version,
    is_deleted,
    created_at
FROM workspace_variables
ORDER BY variable_id, version;

-- 2. 检查是否有重复的 variable_id
SELECT 
    variable_id,
    COUNT(*) as count
FROM workspace_variables
GROUP BY variable_id
HAVING COUNT(*) > 1;

-- 3. 查看 variable_id 为 'var-20a83ab6e573992c' 的所有记录
SELECT *
FROM workspace_variables
WHERE variable_id = 'var-20a83ab6e573992c'
ORDER BY version;
