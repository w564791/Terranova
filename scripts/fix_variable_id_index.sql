-- ============================================================================
-- 修复 variable_id 索引问题
-- ============================================================================
-- 问题：idx_variable_id 要求 variable_id 全局唯一
-- 但我们的设计是：同一个 variable_id 可以有多个版本
-- 解决：删除 idx_variable_id，只保留 idx_variable_id_version
-- ============================================================================

-- Step 1: 删除错误的唯一索引
DROP INDEX IF EXISTS idx_variable_id;

-- Step 2: 验证正确的索引存在
-- idx_variable_id_version 确保 (variable_id, version) 组合唯一
SELECT 
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename = 'workspace_variables'
AND indexname LIKE '%variable_id%';

-- Step 3: 如果需要，可以创建一个非唯一的索引用于查询优化
CREATE INDEX IF NOT EXISTS idx_variable_id_lookup 
ON workspace_variables(variable_id);

-- 完成提示
DO $$
BEGIN
    RAISE NOTICE '========================================';
    RAISE NOTICE ' 索引修复完成！';
    RAISE NOTICE '';
    RAISE NOTICE '变更说明：';
    RAISE NOTICE '1. 删除了 idx_variable_id（全局唯一约束）';
    RAISE NOTICE '2. 保留了 idx_variable_id_version（variable_id + version 唯一）';
    RAISE NOTICE '3. 创建了 idx_variable_id_lookup（非唯一，用于查询优化）';
    RAISE NOTICE '';
    RAISE NOTICE '现在同一个 variable_id 可以有多个版本了！';
    RAISE NOTICE '========================================';
END $$;
