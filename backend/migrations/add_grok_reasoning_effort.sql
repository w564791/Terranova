-- Grok 官方 API：专属 reasoning effort（low / medium / high）
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS grok_reasoning_effort varchar(20) NOT NULL DEFAULT 'high';

COMMENT ON COLUMN ai_configs.grok_reasoning_effort IS 'Grok reasoning effort: low | medium | high (only used when service_type=grok)';
