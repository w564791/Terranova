-- 创建用户个人Token表
CREATE TABLE IF NOT EXISTS user_tokens (
    token_id VARCHAR(30) PRIMARY KEY,  -- token-{8-15位小写字母+数字}
    user_id VARCHAR(20) NOT NULL,
    token_name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    CONSTRAINT fk_user_tokens_user FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX idx_user_tokens_user_id ON user_tokens(user_id);
CREATE INDEX idx_user_tokens_is_active ON user_tokens(is_active);
CREATE INDEX idx_user_tokens_token_hash ON user_tokens(token_hash);

-- 添加注释
COMMENT ON TABLE user_tokens IS '用户个人Token表';
COMMENT ON COLUMN user_tokens.token_id IS 'Token ID (格式: token-{8-15位随机字符})';
COMMENT ON COLUMN user_tokens.user_id IS '用户ID';
COMMENT ON COLUMN user_tokens.token_name IS 'Token名称';
COMMENT ON COLUMN user_tokens.token_hash IS 'Token哈希值（SHA256）';
COMMENT ON COLUMN user_tokens.is_active IS '是否有效';
COMMENT ON COLUMN user_tokens.created_at IS '创建时间';
COMMENT ON COLUMN user_tokens.revoked_at IS '吊销时间';
COMMENT ON COLUMN user_tokens.last_used_at IS '最后使用时间';
COMMENT ON COLUMN user_tokens.expires_at IS '过期时间（NULL表示永不过期）';
