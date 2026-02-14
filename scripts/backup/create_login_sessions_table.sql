-- 创建登录会话表
CREATE TABLE IF NOT EXISTS login_sessions (
    session_id VARCHAR(50) PRIMARY KEY,
    user_id VARCHAR(50) NOT NULL,
    username VARCHAR(100) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP,
    ip_address VARCHAR(50),
    user_agent TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    revoked_at TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_login_sessions_user_id ON login_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_login_sessions_is_active ON login_sessions(is_active);
CREATE INDEX IF NOT EXISTS idx_login_sessions_expires_at ON login_sessions(expires_at);

-- 添加注释
COMMENT ON TABLE login_sessions IS '登录会话表 - 用于验证和管理login token';
COMMENT ON COLUMN login_sessions.session_id IS '会话ID - 存储在JWT的session_id字段中';
COMMENT ON COLUMN login_sessions.user_id IS '用户ID';
COMMENT ON COLUMN login_sessions.username IS '用户名';
COMMENT ON COLUMN login_sessions.created_at IS '创建时间';
COMMENT ON COLUMN login_sessions.expires_at IS '过期时间';
COMMENT ON COLUMN login_sessions.last_used_at IS '最后使用时间';
COMMENT ON COLUMN login_sessions.ip_address IS '登录IP地址';
COMMENT ON COLUMN login_sessions.user_agent IS '用户代理';
COMMENT ON COLUMN login_sessions.is_active IS '是否激活';
COMMENT ON COLUMN login_sessions.revoked_at IS '吊销时间';
