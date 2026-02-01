-- 修复 team_tokens 表的序列

-- 重置序列到当前最大ID
SELECT setval('team_tokens_new_id_seq', COALESCE((SELECT MAX(id) FROM team_tokens), 0) + 1, false);

-- 验证序列
SELECT last_value FROM team_tokens_new_id_seq;
