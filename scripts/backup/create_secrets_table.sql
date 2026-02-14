-- 创建通用secrets表
-- 用于存储所有类型的加密密文数据（Agent Pool、Workspace、Module等）

CREATE TABLE IF NOT EXISTS secrets (
    id SERIAL PRIMARY KEY,
    secret_id VARCHAR(50) UNIQUE NOT NULL,
    value_hash TEXT NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id VARCHAR(50),
    created_by VARCHAR(50),
    updated_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB
);

-- Create unique index for resource_type + resource_id + key combination
CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_unique_key 
ON secrets (resource_type, resource_id, ((metadata->>'key')));

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_secrets_secret_id ON secrets(secret_id);
CREATE INDEX IF NOT EXISTS idx_secrets_resource ON secrets(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_secrets_created_by ON secrets(created_by);
CREATE INDEX IF NOT EXISTS idx_secrets_is_active ON secrets(is_active);
CREATE INDEX IF NOT EXISTS idx_secrets_expires_at ON secrets(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_secrets_metadata_gin ON secrets USING GIN(metadata);

-- 添加注释
COMMENT ON TABLE secrets IS '通用密文存储表，支持多种资源类型的加密数据存储';
COMMENT ON COLUMN secrets.id IS '自增ID（数据库内部使用）';
COMMENT ON COLUMN secrets.secret_id IS '全局唯一ID，格式: secret-{16位随机小写字母+数字}';
COMMENT ON COLUMN secrets.value_hash IS 'AES-256-GCM加密后的密文值';
COMMENT ON COLUMN secrets.resource_type IS '资源类型: agent_pool, workspace, module, system等';
COMMENT ON COLUMN secrets.resource_id IS '关联的资源ID，可为NULL（system级别）';
COMMENT ON COLUMN secrets.created_by IS '创建者ID';
COMMENT ON COLUMN secrets.updated_by IS '最后更新者ID';
COMMENT ON COLUMN secrets.created_at IS '创建时间';
COMMENT ON COLUMN secrets.updated_at IS '更新时间';
COMMENT ON COLUMN secrets.last_used_at IS '最后使用时间';
COMMENT ON COLUMN secrets.expires_at IS '过期时间（可选）';
COMMENT ON COLUMN secrets.is_active IS '是否激活';
COMMENT ON COLUMN secrets.metadata IS '元数据（存储key、description等扩展信息）';
