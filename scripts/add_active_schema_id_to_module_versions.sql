-- 为 module_versions 表添加 active_schema_id 字段
-- 用于关联当前使用的 Schema

-- 添加 active_schema_id 字段
ALTER TABLE module_versions ADD COLUMN IF NOT EXISTS active_schema_id INTEGER REFERENCES schemas(id);

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_module_versions_active_schema ON module_versions(active_schema_id);

-- 迁移现有数据：为每个 ModuleVersion 设置其最新的 active Schema
UPDATE module_versions mv
SET active_schema_id = (
    SELECT s.id 
    FROM schemas s 
    WHERE s.module_version_id = mv.id 
    AND s.status = 'active'
    ORDER BY s.created_at DESC 
    LIMIT 1
)
WHERE mv.active_schema_id IS NULL;

-- 如果没有 active 的，选择最新的 Schema
UPDATE module_versions mv
SET active_schema_id = (
    SELECT s.id 
    FROM schemas s 
    WHERE s.module_version_id = mv.id 
    ORDER BY s.created_at DESC 
    LIMIT 1
)
WHERE mv.active_schema_id IS NULL;

-- 验证迁移结果
SELECT 
    mv.id as version_id,
    mv.version,
    mv.module_id,
    mv.active_schema_id,
    s.version as schema_version,
    s.status as schema_status
FROM module_versions mv
LEFT JOIN schemas s ON mv.active_schema_id = s.id
ORDER BY mv.module_id, mv.created_at;
