-- MFA功能数据库迁移脚本
-- 执行时间: 2026-02-09
-- 功能: 为用户表添加MFA相关字段，添加MFA系统配置

-- ============================================
-- 1. 用户表扩展字段
-- ============================================

-- 添加MFA是否启用字段
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN DEFAULT false;

-- 添加TOTP密钥字段（AES加密存储）
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_secret VARCHAR(128);

-- 添加MFA首次验证成功时间
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_verified_at TIMESTAMP;

-- 添加备用恢复码（JSON数组，AES加密存储）
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_backup_codes TEXT;

-- 添加MFA验证失败次数
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_failed_attempts INTEGER DEFAULT 0;

-- 添加MFA锁定截止时间
ALTER TABLE users ADD COLUMN IF NOT EXISTS mfa_locked_until TIMESTAMP;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_users_mfa_enabled ON users(mfa_enabled);

-- 添加字段注释
COMMENT ON COLUMN users.mfa_enabled IS 'MFA是否已启用';
COMMENT ON COLUMN users.mfa_secret IS 'TOTP密钥（AES加密存储）';
COMMENT ON COLUMN users.mfa_verified_at IS 'MFA首次验证成功时间';
COMMENT ON COLUMN users.mfa_backup_codes IS '备用恢复码（JSON数组，AES加密存储）';
COMMENT ON COLUMN users.mfa_failed_attempts IS 'MFA验证失败次数，用于防暴力破解';
COMMENT ON COLUMN users.mfa_locked_until IS 'MFA锁定截止时间';

-- ============================================
-- 2. MFA临时令牌表（用于登录时的两步验证）
-- ============================================

CREATE TABLE IF NOT EXISTS mfa_tokens (
    id SERIAL PRIMARY KEY,
    token VARCHAR(64) UNIQUE NOT NULL,
    user_id VARCHAR(20) NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    ip_address VARCHAR(45),
    created_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT false,
    used_at TIMESTAMP
);

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_mfa_tokens_token ON mfa_tokens(token);
CREATE INDEX IF NOT EXISTS idx_mfa_tokens_user_id ON mfa_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_mfa_tokens_expires_at ON mfa_tokens(expires_at);

-- 添加表注释
COMMENT ON TABLE mfa_tokens IS 'MFA临时令牌表，用于登录时的两步验证';
COMMENT ON COLUMN mfa_tokens.token IS '临时令牌';
COMMENT ON COLUMN mfa_tokens.user_id IS '用户ID';
COMMENT ON COLUMN mfa_tokens.ip_address IS '请求IP地址';
COMMENT ON COLUMN mfa_tokens.expires_at IS '过期时间';
COMMENT ON COLUMN mfa_tokens.used IS '是否已使用';
COMMENT ON COLUMN mfa_tokens.used_at IS '使用时间';

-- ============================================
-- 3. MFA系统配置
-- ============================================

-- 插入MFA相关系统配置（如果不存在）
INSERT INTO system_configs (key, value, description) 
VALUES ('mfa_enabled', 'true', '是否启用MFA功能')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_configs (key, value, description) 
VALUES ('mfa_enforcement', '"optional"', 'MFA强制策略：optional（可选）、required_new（新用户必须）、required_all（所有用户必须）')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_configs (key, value, description) 
VALUES ('mfa_enforcement_enabled_at', 'null', '强制策略启用时间（用于判断新用户和宽限期）')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_configs (key, value, description) 
VALUES ('mfa_issuer', '"IaC Platform"', 'TOTP发行者名称，显示在Authenticator应用中')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_configs (key, value, description) 
VALUES ('mfa_grace_period_days', '7', '强制MFA的宽限期（天），仅在required_all模式下生效')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_configs (key, value, description) 
VALUES ('mfa_max_failed_attempts', '5', 'MFA验证最大失败尝试次数')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_configs (key, value, description) 
VALUES ('mfa_lockout_duration_minutes', '15', 'MFA验证失败后的锁定时长（分钟）')
ON CONFLICT (key) DO NOTHING;

-- ============================================
-- 4. 清理过期的MFA临时令牌（可选，用于定期清理）
-- ============================================

-- 创建清理函数
CREATE OR REPLACE FUNCTION cleanup_expired_mfa_tokens()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM mfa_tokens 
    WHERE expires_at < NOW() OR used = true;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_expired_mfa_tokens() IS '清理过期或已使用的MFA临时令牌';

-- ============================================
-- 验证迁移结果
-- ============================================

-- 检查用户表字段
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' AND column_name = 'mfa_enabled'
    ) THEN
        RAISE NOTICE 'MFA字段添加成功';
    ELSE
        RAISE EXCEPTION 'MFA字段添加失败';
    END IF;
END $$;

-- 检查MFA令牌表
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_name = 'mfa_tokens'
    ) THEN
        RAISE NOTICE 'MFA令牌表创建成功';
    ELSE
        RAISE EXCEPTION 'MFA令牌表创建失败';
    END IF;
END $$;