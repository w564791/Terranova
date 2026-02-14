-- 为user_tokens表添加session绑定功能
-- 目的：当用户logout时，可以同时吊销所有关联的user token

-- 方案1：添加created_by_session_id字段（可选，用于追踪）
ALTER TABLE user_tokens ADD COLUMN IF NOT EXISTS created_by_session_id VARCHAR(50);
ALTER TABLE user_tokens ADD COLUMN IF NOT EXISTS last_login_session_id VARCHAR(50);

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_user_tokens_last_login_session ON user_tokens(last_login_session_id);

-- 添加注释
COMMENT ON COLUMN user_tokens.created_by_session_id IS '创建此token时的session_id（可选）';
COMMENT ON COLUMN user_tokens.last_login_session_id IS '最后一次登录的session_id - logout时用于吊销token';
