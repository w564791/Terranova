-- 创建团队Token表
-- 用于团队API认证，每个团队最多2个有效token

CREATE TABLE IF NOT EXISTS team_tokens (
    id SERIAL PRIMARY KEY,
    team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    token_name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by INTEGER REFERENCES users(id),
    revoked_at TIMESTAMP,
    revoked_by INTEGER REFERENCES users(id),
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP,
    CONSTRAINT unique_team_token_name UNIQUE(team_id, token_name)
);

-- 创建索引
CREATE INDEX idx_team_tokens_team_id ON team_tokens(team_id);
CREATE INDEX idx_team_tokens_is_active ON team_tokens(is_active);
CREATE INDEX idx_team_tokens_token_hash ON team_tokens(token_hash);

-- 添加注释
COMMENT ON TABLE team_tokens IS '团队API Token表';
COMMENT ON COLUMN team_tokens.token_name IS 'Token名称（用于标识）';
COMMENT ON COLUMN team_tokens.token_hash IS 'Token哈希值（用于验证）';
COMMENT ON COLUMN team_tokens.is_active IS '是否有效';
COMMENT ON COLUMN team_tokens.revoked_at IS '吊销时间';
COMMENT ON COLUMN team_tokens.last_used_at IS '最后使用时间';
COMMENT ON COLUMN team_tokens.expires_at IS '过期时间';
