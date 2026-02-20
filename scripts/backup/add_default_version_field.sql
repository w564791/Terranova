-- 添加is_default字段到terraform_versions表
-- 用于支持全局默认Terraform版本功能

-- 1. 添加字段
ALTER TABLE terraform_versions 
ADD COLUMN IF NOT EXISTS is_default BOOLEAN DEFAULT false;

-- 2. 创建索引
CREATE INDEX IF NOT EXISTS idx_terraform_versions_is_default 
ON terraform_versions(is_default);

-- 3. 创建唯一约束（确保只有一个默认版本）
-- 使用部分唯一索引，只对is_default=true的行生效
CREATE UNIQUE INDEX IF NOT EXISTS idx_terraform_versions_unique_default 
ON terraform_versions(is_default) 
WHERE is_default = true;

-- 4. 设置第一个启用的版本为默认版本（如果还没有默认版本）
UPDATE terraform_versions 
SET is_default = true 
WHERE id = (
    SELECT id FROM terraform_versions 
    WHERE enabled = true 
    ORDER BY created_at ASC 
    LIMIT 1
)
AND NOT EXISTS (
    SELECT 1 FROM terraform_versions WHERE is_default = true
);

-- 5. 添加字段注释
COMMENT ON COLUMN terraform_versions.is_default IS '是否为默认版本（全局唯一）';

-- 验证结果
SELECT 
    version, 
    enabled, 
    is_default,
    created_at
FROM terraform_versions 
ORDER BY is_default DESC, created_at DESC;
