-- Patch: team_tokens 活跃 name 唯一 + 配额辅助
-- 关联：docs/iam/35 B-2
-- 说明：
--   应用层已用事务降低竞态；本补丁在 PostgreSQL 上加部分唯一索引，
--   保证同一 team 下 is_active=true 时 token_name 唯一。
--   执行前请清理重复活跃名：
--     SELECT team_id, token_name, COUNT(*) FROM team_tokens
--     WHERE is_active = true GROUP BY 1,2 HAVING COUNT(*) > 1;

CREATE UNIQUE INDEX IF NOT EXISTS uq_team_tokens_active_name
  ON team_tokens (team_id, token_name)
  WHERE is_active = true;
