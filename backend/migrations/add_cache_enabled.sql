-- Add prompt caching toggle to ai_configs (Bedrock only)
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS cache_enabled boolean NOT NULL DEFAULT true;

COMMENT ON COLUMN ai_configs.cache_enabled IS 'Enable Bedrock prompt caching (cache_control on system prompt)';
