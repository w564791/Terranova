-- 添加频率控制字段到 ai_configs 表
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS rate_limit_seconds INTEGER DEFAULT 10;

COMMENT ON COLUMN ai_configs.rate_limit_seconds IS '频率限制（秒），每个用户在此时间内只能分析一次';

-- 更新现有配置的默认值
UPDATE ai_configs SET rate_limit_seconds = 10 WHERE rate_limit_seconds IS NULL;
