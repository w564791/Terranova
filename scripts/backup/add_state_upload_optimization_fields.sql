-- State 上传优化功能 - 数据库迁移脚本
-- 添加 lineage、serial、导入标记、回滚标记等字段

-- 1. 添加新字段
ALTER TABLE workspace_state_versions 
ADD COLUMN IF NOT EXISTS lineage VARCHAR(255),
ADD COLUMN IF NOT EXISTS serial INTEGER,
ADD COLUMN IF NOT EXISTS is_imported BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS import_source VARCHAR(50),
ADD COLUMN IF NOT EXISTS is_rollback BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS rollback_from_version INTEGER,
ADD COLUMN IF NOT EXISTS description TEXT;

-- 2. 为现有数据填充默认值
UPDATE workspace_state_versions 
SET is_imported = FALSE,
    import_source = 'terraform_apply',
    is_rollback = FALSE
WHERE is_imported IS NULL;

-- 3. 从 content 中提取 lineage 和 serial（如果存在）
UPDATE workspace_state_versions
SET lineage = content->>'lineage',
    serial = CAST(content->>'serial' AS INTEGER)
WHERE content->>'lineage' IS NOT NULL 
  AND content->>'serial' IS NOT NULL
  AND lineage IS NULL;

-- 4. 添加索引（提升查询性能）
CREATE INDEX IF NOT EXISTS idx_state_versions_lineage 
ON workspace_state_versions(workspace_id, lineage);

CREATE INDEX IF NOT EXISTS idx_state_versions_is_imported 
ON workspace_state_versions(workspace_id, is_imported);

CREATE INDEX IF NOT EXISTS idx_state_versions_is_rollback 
ON workspace_state_versions(workspace_id, is_rollback);

CREATE INDEX IF NOT EXISTS idx_state_versions_created_by 
ON workspace_state_versions(created_by);

-- 5. 注意：rollback_from_version 存储的是版本号（version），不是数据库 ID
-- 因此不添加外键约束，而是通过应用层保证数据一致性
-- 如果需要查询回滚源版本，使用：
-- SELECT * FROM workspace_state_versions 
-- WHERE workspace_id = ? AND version = rollback_from_version

-- 6. 添加注释（文档化）
COMMENT ON COLUMN workspace_state_versions.lineage IS 'Terraform state lineage ID (用于校验 state 一致性)';
COMMENT ON COLUMN workspace_state_versions.serial IS 'Terraform state serial number (用于校验版本递增)';
COMMENT ON COLUMN workspace_state_versions.is_imported IS '是否为用户手动导入的 state';
COMMENT ON COLUMN workspace_state_versions.import_source IS 'State 来源: user_upload, api, terraform_apply';
COMMENT ON COLUMN workspace_state_versions.is_rollback IS '是否为回滚操作创建的版本';
COMMENT ON COLUMN workspace_state_versions.rollback_from_version IS '回滚源版本号（version 字段值，不是数据库 ID）';
COMMENT ON COLUMN workspace_state_versions.description IS 'State 版本描述（上传说明或回滚原因）';

-- 7. 验证迁移结果
DO $$ 
DECLARE
    v_count INTEGER;
BEGIN
    -- 检查字段是否添加成功
    SELECT COUNT(*) INTO v_count
    FROM information_schema.columns
    WHERE table_name = 'workspace_state_versions'
      AND column_name IN ('lineage', 'serial', 'is_imported', 'import_source', 
                          'is_rollback', 'rollback_from_version', 'description');
    
    IF v_count = 7 THEN
        RAISE NOTICE '✓ All 7 new columns added successfully';
    ELSE
        RAISE WARNING '✗ Expected 7 columns, found %', v_count;
    END IF;
    
    -- 检查索引是否创建成功
    SELECT COUNT(*) INTO v_count
    FROM pg_indexes
    WHERE tablename = 'workspace_state_versions'
      AND indexname IN ('idx_state_versions_lineage', 'idx_state_versions_is_imported',
                        'idx_state_versions_is_rollback', 'idx_state_versions_created_by');
    
    IF v_count = 4 THEN
        RAISE NOTICE '✓ All 4 indexes created successfully';
    ELSE
        RAISE WARNING '✗ Expected 4 indexes, found %', v_count;
    END IF;
    
    -- 统计现有数据
    SELECT COUNT(*) INTO v_count FROM workspace_state_versions;
    RAISE NOTICE 'Total state versions: %', v_count;
    
    SELECT COUNT(*) INTO v_count 
    FROM workspace_state_versions 
    WHERE lineage IS NOT NULL;
    RAISE NOTICE 'Versions with lineage: %', v_count;
END $$;    
