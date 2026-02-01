-- ============================================================================
-- 诊断和修复 duplicate variable_id 问题
-- ============================================================================

-- Step 1: 检查是否有重复的 variable_id
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '========================================';
    RAISE NOTICE '步骤 1: 检查重复的 variable_id';
    RAISE NOTICE '========================================';
END $$;

SELECT 
    variable_id,
    COUNT(*) as count,
    STRING_AGG(id::TEXT, ', ') as ids,
    STRING_AGG(version::TEXT, ', ') as versions
FROM workspace_variables
GROUP BY variable_id
HAVING COUNT(*) > 1
ORDER BY COUNT(*) DESC;

-- Step 2: 查看所有变量的详细信息
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
    RAISE NOTICE '步骤 2: 查看所有变量';
    RAISE NOTICE '========================================';
END $$;

SELECT 
    id,
    variable_id,
    workspace_id,
    key,
    version,
    variable_type,
    is_deleted,
    created_at
FROM workspace_variables
ORDER BY variable_id, version;

-- Step 3: 修复方案 - 删除重复的记录（保留每个 variable_id 的最新版本）
-- ============================================================================
DO $$
DECLARE
    deleted_count INT;
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
    RAISE NOTICE '步骤 3: 清理重复记录';
    RAISE NOTICE '========================================';
    
    -- 删除重复的记录，只保留每个 variable_id 的最新版本
    WITH duplicates AS (
        SELECT 
            id,
            ROW_NUMBER() OVER (PARTITION BY variable_id ORDER BY version DESC, id DESC) as rn
        FROM workspace_variables
    )
    DELETE FROM workspace_variables
    WHERE id IN (
        SELECT id FROM duplicates WHERE rn > 1
    );
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    IF deleted_count > 0 THEN
        RAISE NOTICE ' 已删除 % 条重复记录', deleted_count;
    ELSE
        RAISE NOTICE ' 没有发现重复记录';
    END IF;
END $$;

-- Step 4: 验证修复结果
-- ============================================================================
DO $$
DECLARE
    duplicate_count INT;
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
    RAISE NOTICE '步骤 4: 验证修复结果';
    RAISE NOTICE '========================================';
    
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT variable_id
        FROM workspace_variables
        GROUP BY variable_id
        HAVING COUNT(*) > 1
    ) t;
    
    IF duplicate_count = 0 THEN
        RAISE NOTICE ' 修复成功！没有重复的 variable_id';
    ELSE
        RAISE WARNING '❌ 仍有 % 个 variable_id 存在重复', duplicate_count;
    END $$;
    
    -- 显示当前状态
    RAISE NOTICE '';
    RAISE NOTICE '当前变量状态：';
END $$;

SELECT 
    variable_id,
    COUNT(*) as version_count,
    MAX(version) as latest_version,
    STRING_AGG(version::TEXT, ', ' ORDER BY version) as all_versions
FROM workspace_variables
GROUP BY variable_id
ORDER BY variable_id;

-- ============================================================================
-- 完成提示
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
    RAISE NOTICE ' 诊断和修复完成';
    RAISE NOTICE '========================================';
    RAISE NOTICE '';
    RAISE NOTICE '如果仍有问题，请检查：';
    RAISE NOTICE '1. 后端代码是否已重新编译';
    RAISE NOTICE '2. 后端服务是否已重启';
    RAISE NOTICE '3. 前端是否已刷新（清除缓存）';
END $$;
