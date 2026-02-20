-- SSO State 存储迁移脚本
-- 将 OAuth2 CSRF state 从内存 map 迁移到 PostgreSQL
-- 创建时间: 2026-02-10

CREATE TABLE IF NOT EXISTS sso_states (
    state VARCHAR(64) PRIMARY KEY,            -- base64 编码的随机 state 字符串
    provider_key VARCHAR(50) NOT NULL,        -- SSO Provider 标识
    redirect_url VARCHAR(500) DEFAULT '/',    -- 登录成功后前端跳转 URL
    action VARCHAR(10) NOT NULL DEFAULT 'login', -- 'login' 或 'link'
    user_id VARCHAR(20) DEFAULT '',           -- link 操作时的用户 ID
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL             -- 过期时间（创建时间 + 10 分钟）
);

-- 创建过期时间索引（用于清理过期 state）
CREATE INDEX IF NOT EXISTS idx_sso_states_expires_at ON sso_states(expires_at);

-- 添加注释
COMMENT ON TABLE sso_states IS 'SSO OAuth2 state 参数存储，用于 CSRF 防护';
COMMENT ON COLUMN sso_states.state IS 'OAuth2 state 参数，base64 编码的 32 字节随机数';
COMMENT ON COLUMN sso_states.action IS '操作类型：login-登录, link-绑定身份';
COMMENT ON COLUMN sso_states.expires_at IS '过期时间，默认创建后 10 分钟';