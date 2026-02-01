-- ============================================================================
-- Workspace Variables 版本控制迁移脚本
-- ============================================================================
-- 功能：为 workspace_variables 表添加版本控制功能
-- 策略：
--   1. 将现有表重命名为 workspace_variables_backup
--   2. 创建新表，添加 variable_id, version, is_deleted 字段
--   3. 将备份表数据迁移到新表（作为 version 1）
-- ============================================================================

-- Step 1: 备份现有表
-- ============================================================================
DO $$
BEGIN
    -- 检查备份表是否已存在
    IF EXISTS (SELECT 1 FROM information_schema.tables 
               WHERE table_name = 'workspace_variables_backup') THEN
        RAISE NOTICE '备份表 workspace_variables_backup 已存在，跳过备份步骤';
    ELSE
        -- 重命名现有表为备份表
        ALTER TABLE workspace_variables RENAME TO workspace_variables_backup;
        RAISE NOTICE ' 已将 workspace_variables 重命名为 workspace_variables_backup';
    END IF;
END $$;

-- Step 2: 创建新的 workspace_variables 表
-- ============================================================================
CREATE TABLE IF NOT EXISTS workspace_variables (
    id BIGSERIAL PRIMARY KEY,
    variable_id VARCHAR(20) NOT NULL,
    workspace_id VARCHAR(50) NOT NULL,
    key VARCHAR(100) NOT NULL,
    version INT NOT NULL DEFAULT 1,
    value TEXT,
    variable_type VARCHAR(20) NOT NULL DEFAULT 'terraform',
    value_format VARCHAR(20) NOT NULL DEFAULT 'string',
    sensitive BOOLEAN NOT NULL DEFAULT FALSE,
    description TEXT,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(20)
);

-- Step 3: 创建索引
-- ============================================================================
-- 唯一索引：variable_id 全局唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_variable_id 
ON workspace_variables(variable_id);

-- 唯一索引：variable_id + version 组合唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_variable_id_version 
ON workspace_variables(variable_id, version);

-- 唯一索引：workspace_id + key + variable_type + version 组合唯一
-- 确保同一 workspace 内，同一变量名的每个版本唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_key_type_version 
ON workspace_variables(workspace_id, key, variable_type, version);

-- 查询优化索引：用于查询最新版本
CREATE INDEX IF NOT EXISTS idx_workspace_key_version 
ON workspace_variables(workspace_id, key, variable_type, version DESC);

-- 查询优化索引：用于过滤已删除的变量
CREATE INDEX IF NOT EXISTS idx_is_deleted 
ON workspace_variables(is_deleted);

-- 查询优化索引：用于按时间查询
CREATE INDEX IF NOT EXISTS idx_created_at 
ON workspace_variables(created_at);

-- Step 4: 数据迁移
-- ============================================================================
-- 从备份表迁移数据到新表，为每条记录生成 variable_id 和设置 version = 1
DO $$
DECLARE
    migrated_count INT;
BEGIN
    -- 检查备份表是否存在
    IF EXISTS (SELECT 1 FROM information_schema.tables 
               WHERE table_name = 'workspace_variables_backup') THEN
        
        -- 迁移数据
        INSERT INTO workspace_variables (
            id,
            variable_id,
            workspace_id,
            key,
            version,
            value,
            variable_type,
            value_format,
            sensitive,
            description,
            is_deleted,
            created_at,
            updated_at,
            created_by
        )
        SELECT 
            id,
            -- 生成 variable_id: var- + 16位小写字母数字组合
            'var-' || LOWER(SUBSTRING(MD5(CONCAT('variable', id::TEXT, workspace_id, key)), 1, 16)),
            workspace_id,
            key,
            1 as version,  -- 所有现有数据作为 version 1
            value,
            variable_type,
            value_format,
            sensitive,
            description,
            FALSE as is_deleted,  -- 现有数据都是未删除状态
            created_at,
            updated_at,
            created_by
        FROM workspace_variables_backup;
        
        GET DIAGNOSTICS migrated_count = ROW_COUNT;
        RAISE NOTICE ' 已迁移 % 条记录到新表', migrated_count;
        
        -- 更新序列，确保新插入的 id 不会冲突
        PERFORM setval('workspace_variables_id_seq', 
                      (SELECT MAX(id) FROM workspace_variables) + 1, 
                      false);
        RAISE NOTICE ' 已更新 id 序列';
        
    ELSE
        RAISE NOTICE '  备份表不存在，跳过数据迁移';
    END IF;
END $$;

-- Step 5: 验证迁移结果
-- ============================================================================
DO $$
DECLARE
    backup_count INT;
    new_count INT;
BEGIN
    -- 统计备份表记录数
    IF EXISTS (SELECT 1 FROM information_schema.tables 
               WHERE table_name = 'workspace_variables_backup') THEN
        SELECT COUNT(*) INTO backup_count FROM workspace_variables_backup;
    ELSE
        backup_count := 0;
    END IF;
    
    -- 统计新表记录数
    SELECT COUNT(*) INTO new_count FROM workspace_variables;
    
    RAISE NOTICE '========================================';
    RAISE NOTICE '迁移验证结果：';
    RAISE NOTICE '备份表记录数: %', backup_count;
    RAISE NOTICE '新表记录数: %', new_count;
    
    IF backup_count = new_count THEN
        RAISE NOTICE ' 数据迁移成功！记录数一致';
    ELSIF backup_count = 0 THEN
        RAISE NOTICE '  备份表为空或不存在';
    ELSE
        RAISE WARNING '❌ 数据迁移可能有问题！记录数不一致';
    END IF;
    RAISE NOTICE '========================================';
END $$;

-- Step 6: 显示示例数据
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '新表示例数据（前5条）：';
END $$;

SELECT 
    id,
    variable_id,
    workspace_id,
    key,
    version,
    variable_type,
    sensitive,
    is_deleted,
    created_at
FROM workspace_variables
ORDER BY id
LIMIT 5;

-- ============================================================================
-- 迁移完成提示
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '========================================';
    RAISE NOTICE ' 变量版本控制迁移完成！';
    RAISE NOTICE '';
    RAISE NOTICE '变更说明：';
    RAISE NOTICE '1. 原表已重命名为: workspace_variables_backup';
    RAISE NOTICE '2. 创建了新表: workspace_variables';
    RAISE NOTICE '3. 新增字段:';
    RAISE NOTICE '   - variable_id: 变量语义化ID (var-xxxxxxxxxxxxxxxx)';
    RAISE NOTICE '   - version: 版本号 (从1开始)';
    RAISE NOTICE '   - is_deleted: 软删除标记';
    RAISE NOTICE '4. 所有现有数据已迁移为 version 1';
    RAISE NOTICE '';
    RAISE NOTICE '回滚方法（如需要）：';
    RAISE NOTICE '  DROP TABLE workspace_variables;';
    RAISE NOTICE '  ALTER TABLE workspace_variables_backup RENAME TO workspace_variables;';
    RAISE NOTICE '========================================';
END $$;
