-- 将主键从token_id改为token_id_hash
-- 这是一个复杂的迁移，需要重建表

-- ============ User Tokens ============

-- 1. 创建新表结构
CREATE TABLE user_tokens_new (
    token_id_hash VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(50) NOT NULL,
    token_name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP,
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP
);

-- 2. 创建索引
CREATE INDEX idx_user_tokens_new_user_id ON user_tokens_new(user_id);
CREATE INDEX idx_user_tokens_new_is_active ON user_tokens_new(is_active);

-- 3. 由于无法迁移旧数据（没有token_id_hash），直接使用新表
-- 删除旧表，重命名新表
DROP TABLE user_tokens;
ALTER TABLE user_tokens_new RENAME TO user_tokens;

-- ============ Team Tokens ============

-- 1. 创建新表结构
CREATE TABLE team_tokens_new (
    token_id_hash VARCHAR(64) PRIMARY KEY,
    team_id VARCHAR(50) NOT NULL,
    token_name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(50),
    revoked_at TIMESTAMP,
    revoked_by VARCHAR(50),
    last_used_at TIMESTAMP,
    expires_at TIMESTAMP
);

-- 2. 创建索引
CREATE INDEX idx_team_tokens_new_team_id ON team_tokens_new(team_id);
CREATE INDEX idx_team_tokens_new_is_active ON team_tokens_new(is_active);

-- 3. 删除旧表，重命名新表
DROP TABLE team_tokens;
ALTER TABLE team_tokens_new RENAME TO team_tokens;

-- 4. 添加注释
COMMENT ON TABLE user_tokens IS '用户Token表 - 使用token_id_hash作为主键';
COMMENT ON TABLE team_tokens IS '团队Token表 - 使用token_id_hash作为主键';
COMMENT ON COLUMN user_tokens.token_id_hash IS 'Token ID的SHA256哈希值（主键）';
COMMENT ON COLUMN team_tokens.token_id_hash IS 'Token ID的SHA256哈希值（主键）';

-- 注意：
-- 1. 此迁移会删除所有现有token
-- 2. 用户需要重新创建所有token
-- 3. 新表使用token_id_hash作为主键
-- 4. 不再保存token_id明文
