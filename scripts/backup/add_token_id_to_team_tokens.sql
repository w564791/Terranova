-- 为team_tokens表添加token_id字段（字符串类型）
-- 目的：与user_tokens保持一致，使用语义化的token_id

-- 1. 添加token_id字段
ALTER TABLE team_tokens ADD COLUMN IF NOT EXISTS token_id VARCHAR(50) UNIQUE;

-- 2. 为现有记录生成token_id（如果有的话）
UPDATE team_tokens 
SET token_id = 'team-token-' || id::text 
WHERE token_id IS NULL;

-- 3. 设置为NOT NULL（在所有记录都有值之后）
ALTER TABLE team_tokens ALTER COLUMN token_id SET NOT NULL;

-- 4. 创建索引
CREATE INDEX IF NOT EXISTS idx_team_tokens_token_id ON team_tokens(token_id);

-- 5. 添加注释
COMMENT ON COLUMN team_tokens.token_id IS '团队Token的唯一标识符（字符串格式）';

-- 注意：这个迁移会影响现有的team token
-- 建议：
-- 1. 执行此迁移后，需要重新生成所有team token
-- 2. 或者修改team token service使用新的token_id字段
