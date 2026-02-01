-- 将token_id改为只保存hash值（方案A：最安全）
-- 目的：即使数据库泄露，也无法获取token_id明文

-- 1. 为user_tokens添加token_id_hash字段
ALTER TABLE user_tokens ADD COLUMN IF NOT EXISTS token_id_hash VARCHAR(64);

-- 2. 为team_tokens添加token_id_hash字段  
ALTER TABLE team_tokens ADD COLUMN IF NOT EXISTS token_id_hash VARCHAR(64);

-- 3. 为现有记录生成hash（使用SHA256）
-- 注意：这一步需要在应用层完成，因为SQL无法直接计算SHA256
-- 或者清空现有token，要求用户重新创建

-- 4. 创建索引
CREATE INDEX IF NOT EXISTS idx_user_tokens_token_id_hash ON user_tokens(token_id_hash);
CREATE INDEX IF NOT EXISTS idx_team_tokens_token_id_hash ON team_tokens(token_id_hash);

-- 5. 添加注释
COMMENT ON COLUMN user_tokens.token_id_hash IS 'Token ID的SHA256哈希值（用于验证）';
COMMENT ON COLUMN team_tokens.token_id_hash IS 'Token ID的SHA256哈希值（用于验证）';

-- 6. 清空现有token（因为无法计算旧token_id的hash）
-- 用户需要重新创建token
UPDATE user_tokens SET is_active = false, revoked_at = NOW() WHERE token_id_hash IS NULL;
UPDATE team_tokens SET is_active = false, revoked_at = NOW() WHERE token_id_hash IS NULL;

-- 7. 可选：移除token_id明文字段（如果不需要显示）
-- ALTER TABLE user_tokens DROP COLUMN IF EXISTS token_id;
-- ALTER TABLE team_tokens DROP COLUMN IF EXISTS token_id;

-- 注意：
-- 1. 执行此迁移后，所有现有token将失效
-- 2. 用户需要重新创建token
-- 3. Token列表将只显示token_name，不显示token_id
