-- 添加 OpenAI Compatible 支持字段
-- 为 ai_configs 表添加 base_url 和 api_key 字段

-- 添加 base_url 字段（用于 OpenAI Compatible API）
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS base_url VARCHAR(500);

-- 添加 api_key 字段（用于 OpenAI Compatible API，加密存储）
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS api_key TEXT;

-- 添加注释
COMMENT ON COLUMN ai_configs.base_url IS 'OpenAI Compatible API 基础 URL（如 https://api.openai.com/v1）';
COMMENT ON COLUMN ai_configs.api_key IS 'OpenAI Compatible API 密钥（加密存储，查询时不返回）';
COMMENT ON COLUMN ai_configs.service_type IS '服务类型：bedrock, openai, azure_openai, ollama 等';

-- 更新现有记录的 service_type 默认值（如果为空）
UPDATE ai_configs SET service_type = 'bedrock' WHERE service_type IS NULL OR service_type = '';
