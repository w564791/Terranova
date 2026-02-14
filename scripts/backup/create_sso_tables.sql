-- SSO 多 Provider 支持数据库迁移脚本
-- 创建时间: 2026-02-10
-- 说明: 支持多身份提供商的 SSO 登录功能

-- ============================================
-- 1. 创建 user_identities 表（用户身份关联表）
-- ============================================
CREATE TABLE IF NOT EXISTS user_identities (
    id BIGSERIAL PRIMARY KEY,                -- 使用 BIGSERIAL 与 Go 模型 int64 一致
    user_id VARCHAR(20) NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    
    -- Provider 信息
    provider VARCHAR(50) NOT NULL,           -- 'auth0', 'azure_ad', 'google', 'github', 'okta'
    provider_user_id VARCHAR(255) NOT NULL,  -- Provider 返回的唯一标识
    provider_email VARCHAR(255),             -- Provider 返回的邮箱
    provider_name VARCHAR(255),              -- Provider 返回的显示名称
    provider_avatar VARCHAR(500),            -- Provider 返回的头像 URL
    
    -- 元数据
    raw_data JSONB,                          -- 原始用户信息（调试用）
    access_token_encrypted TEXT,             -- 加密存储的 access_token（可选）
    refresh_token_encrypted TEXT,            -- 加密存储的 refresh_token（可选）
    token_expires_at TIMESTAMP,              -- Token 过期时间
    
    -- 状态
    is_primary BOOLEAN DEFAULT FALSE,        -- 是否为主要登录方式
    is_verified BOOLEAN DEFAULT TRUE,        -- 邮箱是否已验证
    last_used_at TIMESTAMP,                  -- 最后使用时间
    
    -- 审计
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- 唯一约束1：同一 provider 的同一 provider_user_id 只能存在一条记录
    UNIQUE(provider, provider_user_id),
    -- 唯一约束2：同一用户在同一 provider 只能绑定一个账号
    UNIQUE(user_id, provider)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_user_identities_user_id ON user_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_identities_provider ON user_identities(provider);
CREATE INDEX IF NOT EXISTS idx_user_identities_email ON user_identities(provider_email);

-- 添加注释
COMMENT ON TABLE user_identities IS '用户身份关联表，支持多 Provider 绑定';
COMMENT ON COLUMN user_identities.provider IS '身份提供商：auth0, azure_ad, google, github, okta 等';
COMMENT ON COLUMN user_identities.provider_user_id IS 'Provider 返回的唯一用户标识（如 Auth0 的 sub）';
COMMENT ON COLUMN user_identities.is_primary IS '是否为主要登录方式，用于显示和默认选择';
COMMENT ON COLUMN user_identities.raw_data IS '原始用户信息 JSON，用于调试和扩展';
COMMENT ON COLUMN user_identities.access_token_encrypted IS 'AES 加密的 access_token，用于 API 调用';
COMMENT ON COLUMN user_identities.refresh_token_encrypted IS 'AES 加密的 refresh_token，用于刷新 token';

-- ============================================
-- 2. 创建 sso_providers 表（SSO 配置表）
-- ============================================
CREATE TABLE IF NOT EXISTS sso_providers (
    id SERIAL PRIMARY KEY,
    
    -- 基本信息
    provider_key VARCHAR(50) NOT NULL UNIQUE,  -- 唯一标识，用于路由：'auth0', 'azure_company_a'
    provider_type VARCHAR(30) NOT NULL,        -- 类型：'auth0', 'azure_ad', 'google', 'github', 'okta', 'saml'
    display_name VARCHAR(100) NOT NULL,        -- 显示名称：'使用 Google 登录'
    description TEXT,                          -- 描述
    icon VARCHAR(50),                          -- 图标名称：'google', 'microsoft', 'github'
    
    -- OAuth 配置（JSONB 存储，便于不同 Provider 的差异化配置）
    oauth_config JSONB NOT NULL,
    -- 示例：
    -- Auth0: {"domain": "xxx.auth0.com", "client_id": "...", "client_secret_encrypted": "..."}
    -- Azure AD: {"tenant_id": "...", "client_id": "...", "client_secret_encrypted": "..."}
    -- Google: {"client_id": "...", "client_secret_encrypted": "..."}
    
    -- 端点配置（可选，某些 Provider 需要自定义）
    authorize_endpoint VARCHAR(500),
    token_endpoint VARCHAR(500),
    userinfo_endpoint VARCHAR(500),
    
    -- 回调配置
    callback_url VARCHAR(500) NOT NULL,        -- 回调 URL
    allowed_callback_urls TEXT[],              -- 允许的回调 URL 列表
    
    -- 用户管理配置
    auto_create_user BOOLEAN DEFAULT TRUE,     -- 是否自动创建用户
    default_role VARCHAR(50) DEFAULT 'user',   -- 默认角色
    allowed_domains TEXT[],                    -- 允许的邮箱域名（企业 SSO 用）
    
    -- 属性映射（不同 Provider 返回字段名不同）
    attribute_mapping JSONB DEFAULT '{"user_id": "sub", "email": "email", "name": "name", "avatar": "picture"}',
    
    -- 状态
    is_enabled BOOLEAN DEFAULT TRUE,
    is_enterprise BOOLEAN DEFAULT FALSE,       -- 是否为企业专用
    organization_id VARCHAR(20),               -- 关联的组织（企业 SSO）
    
    -- 排序和显示
    display_order INT DEFAULT 0,               -- 显示顺序
    show_on_login_page BOOLEAN DEFAULT TRUE,   -- 是否在登录页显示
    
    -- 审计
    created_by VARCHAR(20),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引（用于登录页查询）
CREATE INDEX IF NOT EXISTS idx_sso_providers_enabled ON sso_providers(is_enabled);
CREATE INDEX IF NOT EXISTS idx_sso_providers_show_on_login ON sso_providers(show_on_login_page);
CREATE INDEX IF NOT EXISTS idx_sso_providers_display_order ON sso_providers(display_order);
CREATE INDEX IF NOT EXISTS idx_sso_providers_org_id ON sso_providers(organization_id);

-- 添加注释
COMMENT ON TABLE sso_providers IS 'SSO 身份提供商配置表';
COMMENT ON COLUMN sso_providers.provider_key IS '唯一标识，用于 API 路由，如 auth0, google, azure_company_a';
COMMENT ON COLUMN sso_providers.provider_type IS 'Provider 类型，决定使用哪个处理器：auth0, azure_ad, google, github, okta, saml';
COMMENT ON COLUMN sso_providers.oauth_config IS 'OAuth 配置，JSON 格式存储，包含 client_id, client_secret 等';
COMMENT ON COLUMN sso_providers.attribute_mapping IS '用户属性映射配置，用于标准化不同 Provider 返回的字段';
COMMENT ON COLUMN sso_providers.allowed_domains IS '允许的邮箱域名列表，用于企业 SSO 限制';
COMMENT ON COLUMN sso_providers.is_enterprise IS '是否为企业专用 Provider';

-- ============================================
-- 3. 创建 sso_login_logs 表（SSO 登录日志表）
-- ============================================
CREATE TABLE IF NOT EXISTS sso_login_logs (
    id BIGSERIAL PRIMARY KEY,
    
    -- 关联信息
    user_id VARCHAR(20),                       -- 登录成功后的用户 ID
    identity_id BIGINT,                        -- 关联的 user_identities.id（与 BIGSERIAL 一致）
    provider_key VARCHAR(50) NOT NULL,         -- 使用的 Provider
    
    -- 登录信息
    provider_user_id VARCHAR(255),             -- Provider 返回的用户 ID
    provider_email VARCHAR(255),               -- Provider 返回的邮箱
    
    -- 状态
    status VARCHAR(20) NOT NULL,               -- 'success', 'failed', 'user_created', 'user_linked'
    error_message TEXT,                        -- 失败原因
    
    -- 请求信息
    ip_address VARCHAR(45),
    user_agent TEXT,
    
    -- 时间
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_sso_login_logs_user_id ON sso_login_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_sso_login_logs_provider ON sso_login_logs(provider_key);
CREATE INDEX IF NOT EXISTS idx_sso_login_logs_created_at ON sso_login_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_sso_login_logs_status ON sso_login_logs(status);

-- 添加注释
COMMENT ON TABLE sso_login_logs IS 'SSO 登录日志，用于审计和问题排查';
COMMENT ON COLUMN sso_login_logs.status IS '登录状态：success-成功, failed-失败, user_created-新用户创建, user_linked-身份关联';
COMMENT ON COLUMN sso_login_logs.error_message IS '失败时的错误信息';

-- ============================================
-- 4. 修改 users 表
-- ============================================
-- 添加 is_sso_user 字段标识用户是否为 SSO 用户
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_sso_user BOOLEAN DEFAULT FALSE;

-- 修改 password_hash 为可空（SSO 用户无密码）
ALTER TABLE users ALTER COLUMN password_hash DROP NOT NULL;

-- 添加注释
COMMENT ON COLUMN users.oauth_provider IS '主要 OAuth 提供商（快速查询用），详细信息见 user_identities 表';
COMMENT ON COLUMN users.oauth_id IS '主要 OAuth 用户 ID（快速查询用），详细信息见 user_identities 表';
COMMENT ON COLUMN users.is_sso_user IS '是否为 SSO 用户（无密码登录）';
COMMENT ON COLUMN users.password_hash IS '密码哈希，SSO 用户可为空';

-- ============================================
-- 5. 插入默认 Provider 配置（示例）
-- ============================================
-- 注意：以下配置需要替换为实际的 client_id 和 client_secret

-- Auth0 配置示例（已注释，需要时取消注释并填入实际值）
-- INSERT INTO sso_providers (
--     provider_key, provider_type, display_name, icon,
--     oauth_config, callback_url, display_order
-- ) VALUES (
--     'auth0',
--     'auth0',
--     '使用 Auth0 登录',
--     'auth0',
--     '{"domain": "your-domain.auth0.com", "client_id": "your_client_id", "client_secret_encrypted": ""}',
--     'http://localhost:8080/api/auth/sso/auth0/callback',
--     1
-- ) ON CONFLICT (provider_key) DO NOTHING;

-- Google 配置示例（已注释，需要时取消注释并填入实际值）
-- INSERT INTO sso_providers (
--     provider_key, provider_type, display_name, icon,
--     oauth_config, callback_url, display_order
-- ) VALUES (
--     'google',
--     'google',
--     '使用 Google 登录',
--     'google',
--     '{"client_id": "your_client_id.apps.googleusercontent.com", "client_secret_encrypted": "", "scopes": ["openid", "profile", "email"]}',
--     'http://localhost:8080/api/auth/sso/google/callback',
--     2
-- ) ON CONFLICT (provider_key) DO NOTHING;

-- GitHub 配置示例（已注释，需要时取消注释并填入实际值）
-- INSERT INTO sso_providers (
--     provider_key, provider_type, display_name, icon,
--     oauth_config, callback_url, display_order
-- ) VALUES (
--     'github',
--     'github',
--     '使用 GitHub 登录',
--     'github',
--     '{"client_id": "your_client_id", "client_secret_encrypted": "", "scopes": ["user:email", "read:user"]}',
--     'http://localhost:8080/api/auth/sso/github/callback',
--     3
-- ) ON CONFLICT (provider_key) DO NOTHING;

-- ============================================
-- 6. 创建更新时间触发器
-- ============================================
-- 创建更新时间函数
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 为 user_identities 表创建触发器
DROP TRIGGER IF EXISTS update_user_identities_updated_at ON user_identities;
CREATE TRIGGER update_user_identities_updated_at
    BEFORE UPDATE ON user_identities
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 为 sso_providers 表创建触发器
DROP TRIGGER IF EXISTS update_sso_providers_updated_at ON sso_providers;
CREATE TRIGGER update_sso_providers_updated_at
    BEFORE UPDATE ON sso_providers
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- 完成
-- ============================================
-- 执行完成后，可以通过以下命令验证：
-- SELECT * FROM user_identities LIMIT 1;
-- SELECT * FROM sso_providers LIMIT 1;
-- SELECT * FROM sso_login_logs LIMIT 1;
-- \d+ user_identities
-- \d+ sso_providers
-- \d+ sso_login_logs