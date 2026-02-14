-- Module 多版本能力数据库迁移脚本
-- 执行顺序：最后执行（在后端代码部署后）
-- 
-- 此脚本包含：
-- 1. 创建 module_versions 表
-- 2. 修改 modules 表（添加 default_version_id 字段）
-- 3. 修改 schemas 表（添加 module_version_id 和 inherited_from_schema_id 字段）
-- 4. 修改 module_demos 表（添加 module_version_id 和 inherited_from_demo_id 字段）
-- 5. 数据迁移：为现有模块创建版本记录并关联现有数据

-- ========== 1. 创建 module_versions 表 ==========
CREATE TABLE IF NOT EXISTS module_versions (
    id VARCHAR(30) PRIMARY KEY,                    -- modv-xxx 语义化 ID
    module_id INT NOT NULL,                        -- 外键关联 modules 表
    version VARCHAR(50) NOT NULL,                  -- Terraform Module 版本 (如 6.1.5)
    source VARCHAR(500),                           -- Module source (可覆盖)
    module_source VARCHAR(500),                    -- 完整 source URL
    is_default BOOLEAN DEFAULT false,              -- 是否为默认版本
    status VARCHAR(20) DEFAULT 'active',           -- active, deprecated, archived
    inherited_from_version_id VARCHAR(30),         -- 继承自哪个版本（用于追溯）
    created_by VARCHAR(20),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    CONSTRAINT fk_module_versions_module FOREIGN KEY (module_id) REFERENCES modules(id) ON DELETE CASCADE,
    CONSTRAINT uk_module_versions_module_version UNIQUE (module_id, version)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_module_versions_module ON module_versions(module_id);
CREATE INDEX IF NOT EXISTS idx_module_versions_default ON module_versions(module_id, is_default);
CREATE INDEX IF NOT EXISTS idx_module_versions_status ON module_versions(status);

COMMENT ON TABLE module_versions IS 'Module 版本表，支持同一 Module 的多个 Terraform 版本';
COMMENT ON COLUMN module_versions.id IS '语义化 ID，格式：modv-xxx';
COMMENT ON COLUMN module_versions.module_id IS '关联的 Module ID';
COMMENT ON COLUMN module_versions.version IS 'Terraform Module 版本号，如 6.1.5';
COMMENT ON COLUMN module_versions.is_default IS '是否为默认版本，每个 Module 只能有一个默认版本';
COMMENT ON COLUMN module_versions.status IS '版本状态：active（活跃）、deprecated（已废弃）、archived（已归档）';
COMMENT ON COLUMN module_versions.inherited_from_version_id IS '继承自哪个版本，用于追溯 Schema 来源';

-- ========== 2. 修改 modules 表 ==========
-- 添加 default_version_id 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'modules' AND column_name = 'default_version_id'
    ) THEN
        ALTER TABLE modules ADD COLUMN default_version_id VARCHAR(30);
        COMMENT ON COLUMN modules.default_version_id IS '默认版本 ID，指向 module_versions 表';
    END IF;
END $$;

-- ========== 3. 修改 schemas 表 ==========
-- 添加 module_version_id 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'schemas' AND column_name = 'module_version_id'
    ) THEN
        ALTER TABLE schemas ADD COLUMN module_version_id VARCHAR(30);
        CREATE INDEX IF NOT EXISTS idx_schemas_module_version ON schemas(module_version_id);
        COMMENT ON COLUMN schemas.module_version_id IS '关联的 Module 版本 ID';
    END IF;
END $$;

-- 添加 inherited_from_schema_id 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'schemas' AND column_name = 'inherited_from_schema_id'
    ) THEN
        ALTER TABLE schemas ADD COLUMN inherited_from_schema_id INT;
        COMMENT ON COLUMN schemas.inherited_from_schema_id IS '继承自哪个 Schema，用于追溯';
    END IF;
END $$;

-- ========== 4. 修改 module_demos 表 ==========
-- 添加 module_version_id 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'module_demos' AND column_name = 'module_version_id'
    ) THEN
        ALTER TABLE module_demos ADD COLUMN module_version_id VARCHAR(30);
        CREATE INDEX IF NOT EXISTS idx_module_demos_version ON module_demos(module_version_id);
        COMMENT ON COLUMN module_demos.module_version_id IS '关联的 Module 版本 ID';
    END IF;
END $$;

-- 添加 inherited_from_demo_id 字段
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'module_demos' AND column_name = 'inherited_from_demo_id'
    ) THEN
        ALTER TABLE module_demos ADD COLUMN inherited_from_demo_id INT;
        COMMENT ON COLUMN module_demos.inherited_from_demo_id IS '继承自哪个 Demo，用于追溯';
    END IF;
END $$;

-- ========== 5. 数据迁移 ==========
-- 为现有模块创建版本记录并关联现有数据
-- 注意：此部分可以通过 API 调用 POST /api/v1/modules/migrate-versions 来执行
-- 或者使用以下 SQL 脚本手动执行

-- 创建临时函数生成 ID（使用 md5 和 random 替代 gen_random_bytes）
CREATE OR REPLACE FUNCTION generate_module_version_id() RETURNS VARCHAR(30) AS $$
DECLARE
    random_part VARCHAR(20);
BEGIN
    SELECT substring(md5(random()::text || clock_timestamp()::text) for 20) INTO random_part;
    RETURN 'modv-' || random_part;
END;
$$ LANGUAGE plpgsql;

-- 为每个现有模块创建版本记录
DO $$
DECLARE
    module_record RECORD;
    new_version_id VARCHAR(30);
BEGIN
    FOR module_record IN 
        SELECT id, version, source, module_source, created_by, created_at 
        FROM modules 
        WHERE id NOT IN (SELECT DISTINCT module_id FROM module_versions)
    LOOP
        -- 生成新的版本 ID
        new_version_id := generate_module_version_id();
        
        -- 创建版本记录
        INSERT INTO module_versions (id, module_id, version, source, module_source, is_default, status, created_by, created_at, updated_at)
        VALUES (
            new_version_id,
            module_record.id,
            module_record.version,
            module_record.source,
            module_record.module_source,
            true,  -- 设为默认版本
            'active',
            module_record.created_by,
            module_record.created_at,
            NOW()
        );
        
        -- 更新 Module 的 default_version_id
        UPDATE modules SET default_version_id = new_version_id WHERE id = module_record.id;
        
        -- 更新现有 Schema 的 module_version_id
        UPDATE schemas SET module_version_id = new_version_id 
        WHERE module_id = module_record.id AND module_version_id IS NULL;
        
        -- 更新现有 Demo 的 module_version_id
        UPDATE module_demos SET module_version_id = new_version_id 
        WHERE module_id = module_record.id AND module_version_id IS NULL;
        
        RAISE NOTICE 'Migrated module % (version %) -> %', module_record.id, module_record.version, new_version_id;
    END LOOP;
END $$;

-- 清理临时函数
DROP FUNCTION IF EXISTS generate_module_version_id();

-- ========== 6. 验证迁移结果 ==========
-- 检查迁移是否成功
DO $$
DECLARE
    modules_without_version INT;
    schemas_without_version INT;
    demos_without_version INT;
BEGIN
    -- 检查是否有模块没有版本记录
    SELECT COUNT(*) INTO modules_without_version
    FROM modules m
    WHERE NOT EXISTS (SELECT 1 FROM module_versions mv WHERE mv.module_id = m.id);
    
    -- 检查是否有 Schema 没有关联版本
    SELECT COUNT(*) INTO schemas_without_version
    FROM schemas s
    WHERE s.module_version_id IS NULL;
    
    -- 检查是否有 Demo 没有关联版本
    SELECT COUNT(*) INTO demos_without_version
    FROM module_demos d
    WHERE d.module_version_id IS NULL;
    
    RAISE NOTICE '========== Migration Verification ==========';
    RAISE NOTICE 'Modules without version: %', modules_without_version;
    RAISE NOTICE 'Schemas without version: %', schemas_without_version;
    RAISE NOTICE 'Demos without version: %', demos_without_version;
    
    IF modules_without_version = 0 AND schemas_without_version = 0 AND demos_without_version = 0 THEN
        RAISE NOTICE 'Migration completed successfully!';
    ELSE
        RAISE WARNING 'Migration may be incomplete. Please check the data.';
    END IF;
END $$;

-- ========== 7. 显示迁移统计 ==========
SELECT 
    'module_versions' as table_name,
    COUNT(*) as record_count
FROM module_versions
UNION ALL
SELECT 
    'modules with default_version_id' as table_name,
    COUNT(*) as record_count
FROM modules WHERE default_version_id IS NOT NULL
UNION ALL
SELECT 
    'schemas with module_version_id' as table_name,
    COUNT(*) as record_count
FROM schemas WHERE module_version_id IS NOT NULL
UNION ALL
SELECT 
    'module_demos with module_version_id' as table_name,
    COUNT(*) as record_count
FROM module_demos WHERE module_version_id IS NOT NULL;
