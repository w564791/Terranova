-- 移除token_id明文，只保留hash值
-- 目的：最大化安全性，即使数据库泄露也无法获取token_id

-- 1. 清空user_tokens的token_id明文
UPDATE user_tokens SET token_id = NULL WHERE token_id IS NOT NULL;

-- 2. 清空team_tokens的token_id明文  
UPDATE team_tokens SET token_id = NULL WHERE token_id IS NOT NULL;

-- 3. 修改字段约束（允许NULL）
ALTER TABLE user_tokens ALTER COLUMN token_id DROP NOT NULL;
ALTER TABLE team_tokens ALTER COLUMN token_id DROP NOT NULL;

-- 4. 移除唯一索引（因为都是NULL了）
DROP INDEX IF EXISTS user_tokens_token_id_key;
DROP INDEX IF EXISTS team_tokens_token_id_key;
DROP INDEX IF EXISTS idx_team_tokens_token_id;

-- 5. 添加注释
COMMENT ON COLUMN user_tokens.token_id IS 'Token ID明文（已废弃，使用token_id_hash）';
COMMENT ON COLUMN team_tokens.token_id IS 'Token ID明文（已废弃，使用token_id_hash）';

-- 注意：
-- 1. 执行后token_id字段将全部为NULL
-- 2. 只能通过token_name识别token
-- 3. 验证使用token_id_hash
-- 4. 这是最安全的方案
