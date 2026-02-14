-- 删除team_tokens表的token_name唯一约束
-- Token名称应该可以重复，它只是一个备注/comment

-- 删除唯一约束
ALTER TABLE team_tokens DROP CONSTRAINT IF EXISTS unique_team_token_name;

-- 删除唯一索引（如果存在）
DROP INDEX IF EXISTS unique_team_token_name;

-- 添加注释说明
COMMENT ON COLUMN team_tokens.token_name IS 'Token备注名称（可重复，用于标识token用途）';
