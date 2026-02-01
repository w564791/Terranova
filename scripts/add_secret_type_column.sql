-- 添加secret_type列到secrets表
ALTER TABLE secrets ADD COLUMN IF NOT EXISTS secret_type VARCHAR(20) NOT NULL DEFAULT 'hcp';

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_secrets_secret_type ON secrets(secret_type);

-- 添加注释
COMMENT ON COLUMN secrets.secret_type IS '密文类型: hcp (HashiCorp Cloud Platform)';
