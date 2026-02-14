-- 查找和修复重复的 variable_id 问题
-- 此脚本用于识别和清理软删除重建时生成的重复 variable_id

-- ============================================
-- 第一步：查找重复的变量
-- ============================================

-- 1.1 查找相同 key 但不同 variable_id 的变量
SELECT 
    workspace_id,
    key,
    variable_type,
    COUNT(DISTINCT variable_id) as variable_id_count,
    STRING_AGG(DISTINCT variable_id, ', ' ORDER BY variable_id) as variable_ids,
    STRING_AGG(
        DISTINCT variable_id || ' (v' || 
        (SELECT MAX(version) FROM workspace_variables wv2 WHERE wv2.variable_id = wv.variable_id) || 
        ', ' || 
        (SELECT COUNT(*) FROM workspace_variables wv3 WHERE wv3.variable_id = wv.variable_id) || 
        ' versions)',
        ', '
    ) as details
FROM (
    SELECT DISTINCT 
        workspace_id,
        key,
        variable_type,
        variable_id
    FROM workspace_variables
) wv
GROUP BY workspace_id, key, variable_type
HAVING COUNT(DISTINCT variable_id) > 1
ORDER BY workspace_id, key;

-- 1.2 查看每个重复变量的详细信息
SELECT 
    wv.workspace_id,
    wv.key,
    wv.variable_type,
    wv.variable_id,
    wv.version,
    wv.is_deleted,
    wv.created_at,
    wv.updated_at,
    -- 标记哪个是最早的 variable_id（应该保留的）
    CASE 
        WHEN wv.variable_id = (
            SELECT MIN(variable_id) 
            FROM workspace_variables wv2 
            WHERE wv2.workspace_id = wv.workspace_id 
            AND wv2.key = wv.key 
            AND wv2.variable_type = wv.variable_type
        ) THEN '✓ KEEP (最早的)'
        ELSE '✗ REMOVE (重复的)'
    END as action
FROM workspace_variables wv
WHERE EXISTS (
    SELECT 1
    FROM (
        SELECT workspace_id, key, variable_type
        FROM workspace_variables
        GROUP BY workspace_id, key, variable_type
        HAVING COUNT(DISTINCT variable_id) > 1
    ) dup
    WHERE dup.workspace_id = wv.workspace_id
    AND dup.key = wv.key
    AND dup.variable_type = wv.variable_type
)
ORDER BY wv.workspace_id, wv.key, wv.variable_id, wv.version;

-- ============================================
-- 第二步：生成修复脚本（手动执行）
-- ============================================

-- 2.1 生成删除重复 variable_id 的 SQL
-- 注意：这只是生成 SQL，不会实际执行删除
SELECT 
    'DELETE FROM workspace_variables WHERE variable_id = ''' || variable_id || ''';' as delete_sql,
    workspace_id,
    key,
    variable_type,
    variable_id,
    '(重复的 variable_id，应该删除)' as reason
FROM workspace_variables wv
WHERE variable_id NOT IN (
    -- 保留每个 key 的最早的 variable_id
    SELECT MIN(variable_id)
    FROM workspace_variables wv2
    WHERE wv2.workspace_id = wv.workspace_id
    AND wv2.key = wv.key
    AND wv2.variable_type = wv.variable_type
    GROUP BY wv2.workspace_id, wv2.key, wv2.variable_type
)
AND EXISTS (
    -- 只处理有重复的变量
    SELECT 1
    FROM (
        SELECT workspace_id, key, variable_type
        FROM workspace_variables
        GROUP BY workspace_id, key, variable_type
        HAVING COUNT(DISTINCT variable_id) > 1
    ) dup
    WHERE dup.workspace_id = wv.workspace_id
    AND dup.key = wv.key
    AND dup.variable_type = wv.variable_type
)
ORDER BY workspace_id, key, variable_id;

-- ============================================
-- 第三步：验证修复后的数据
-- ============================================

-- 3.1 验证没有重复的 variable_id
SELECT 
    workspace_id,
    key,
    variable_type,
    COUNT(DISTINCT variable_id) as variable_id_count
FROM (
    SELECT DISTINCT 
        workspace_id,
        key,
        variable_type,
        variable_id
    FROM workspace_variables
) t
GROUP BY workspace_id, key, variable_type
HAVING COUNT(DISTINCT variable_id) > 1;
-- 预期结果：0 行（没有重复）

-- 3.2 验证每个 workspace 的变量数量
SELECT 
    workspace_id,
    COUNT(DISTINCT variable_id) as unique_variables,
    COUNT(DISTINCT CASE WHEN is_deleted = false THEN variable_id END) as active_variables,
    COUNT(*) as total_versions
FROM workspace_variables
GROUP BY workspace_id
ORDER BY workspace_id;

-- ============================================
-- 使用说明
-- ============================================

/*
使用步骤：

1. 运行第一步的查询，查看是否有重复的 variable_id
   - 如果没有重复，说明数据正常，无需修复
   - 如果有重复，继续下一步

2. 运行第二步的查询，生成删除 SQL
   - 仔细检查生成的 SQL，确认要删除的是重复的 variable_id
   - 建议先在测试环境执行

3. 手动执行生成的删除 SQL
   - 可以一条一条执行，或者批量执行
   - 建议先备份数据

4. 运行第三步的查询，验证修复结果
   - 确认没有重复的 variable_id
   - 确认变量数量正确

注意事项：
- 此脚本会删除重复的 variable_id 及其所有版本
- 保留的是最早创建的 variable_id（字母序最小的）
- 删除前请务必备份数据
- 建议先在测试环境验证

修复原理：
- 对于每个 (workspace_id, key, variable_type) 组合
- 如果存在多个 variable_id，保留最早的（MIN(variable_id)）
- 删除其他重复的 variable_id 及其所有版本
*/
