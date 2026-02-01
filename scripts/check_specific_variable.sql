-- 查询特定变量ID的所有版本
SELECT 
    id,
    variable_id,
    workspace_id,
    key,
    version,
    is_deleted,
    created_at,
    updated_at
FROM workspace_variables
WHERE variable_id = 'var-rip37tutb78nmydn'
ORDER BY version ASC;
