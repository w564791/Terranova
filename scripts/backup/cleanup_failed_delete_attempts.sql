-- 清理失败的删除尝试产生的残留数据

-- 查看所有变量的版本情况
SELECT 
    variable_id,
    key,
    COUNT(*) as version_count,
    MAX(version) as max_version,
    STRING_AGG(version::TEXT || '(' || CASE WHEN is_deleted THEN 'deleted' ELSE 'active' END || ')', ', ' ORDER BY version) as versions
FROM workspace_variables
GROUP BY variable_id, key
ORDER BY variable_id;

-- 如果发现有问题的数据，可以手动清理
-- 例如：删除失败尝试产生的重复版本
-- DELETE FROM workspace_variables WHERE id = xxx;
