-- 添加 embedding batch 配置字段
-- 用于支持批量 embedding 生成，提升效率

ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS embedding_batch_enabled BOOLEAN DEFAULT false;
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS embedding_batch_size INTEGER DEFAULT 10;

COMMENT ON COLUMN ai_configs.embedding_batch_enabled IS '是否启用批量 embedding（仅 embedding 能力使用，Titan V2、OpenAI 等模型支持）';
COMMENT ON COLUMN ai_configs.embedding_batch_size IS '批量大小（建议 10-50，过大可能导致 API 超时）';
