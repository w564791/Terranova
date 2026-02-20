-- Module Schema V2 数据库迁移脚本
-- 添加支持 OpenAPI v3 Schema 的字段

-- 1. 添加 schema_version 字段，用于区分 v1 和 v2 Schema
ALTER TABLE schemas ADD COLUMN IF NOT EXISTS schema_version VARCHAR(10) DEFAULT 'v1';

-- 2. 添加 openapi_schema 字段，存储 OpenAPI v3 格式的 Schema
-- 这个字段与 schema_data 分开存储，便于兼容和迁移
ALTER TABLE schemas ADD COLUMN IF NOT EXISTS openapi_schema JSONB;

-- 3. 添加 variables_tf 字段，存储原始的 variables.tf 内容
-- 用于后续重新解析或编辑
ALTER TABLE schemas ADD COLUMN IF NOT EXISTS variables_tf TEXT;

-- 4. 添加 ui_config 字段，存储 UI 配置（从 openapi_schema 的 x-iac-platform.ui 提取）
-- 便于快速访问 UI 配置而不需要解析整个 openapi_schema
ALTER TABLE schemas ADD COLUMN IF NOT EXISTS ui_config JSONB;

-- 5. 添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_schemas_schema_version ON schemas(schema_version);
CREATE INDEX IF NOT EXISTS idx_schemas_module_id_version ON schemas(module_id, schema_version);

-- 6. 更新现有记录的 schema_version 为 'v1'
UPDATE schemas SET schema_version = 'v1' WHERE schema_version IS NULL;

-- 7. 添加注释
COMMENT ON COLUMN schemas.schema_version IS 'Schema版本: v1(旧格式) 或 v2(OpenAPI格式)';
COMMENT ON COLUMN schemas.openapi_schema IS 'OpenAPI v3 格式的 Schema 定义';
COMMENT ON COLUMN schemas.variables_tf IS '原始的 Terraform variables.tf 文件内容';
COMMENT ON COLUMN schemas.ui_config IS 'UI 配置，从 openapi_schema 的 x-iac-platform.ui 提取';

-- 验证迁移结果
SELECT 
    column_name, 
    data_type, 
    is_nullable,
    column_default
FROM information_schema.columns 
WHERE table_name = 'schemas' 
ORDER BY ordinal_position;
