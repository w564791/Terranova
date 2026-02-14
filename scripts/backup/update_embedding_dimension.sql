-- 更新 embedding 列维度从 3072 改为 1024
-- 用于支持 Bedrock Titan Embedding 模型（1024 维）

-- 1. 删除现有的 embedding 列（如果有数据会丢失）
ALTER TABLE resource_index DROP COLUMN IF EXISTS embedding;

-- 2. 重新创建 embedding 列为 1024 维
ALTER TABLE resource_index ADD COLUMN embedding VECTOR(1024);

-- 3. 创建向量索引（使用 IVFFlat，因为 HNSW 不支持超过 2000 维，但 1024 维可以使用 HNSW）
-- 对于 1024 维，可以使用 HNSW 索引
DROP INDEX IF EXISTS idx_resource_embedding;
CREATE INDEX idx_resource_embedding ON resource_index 
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- 4. 更新 AI 配置，添加 Bedrock Titan Embedding 配置
-- 删除旧的 OpenAI embedding 配置（如果存在）
DELETE FROM ai_configs WHERE capabilities @> '["embedding"]' AND service_type = 'openai';

-- 添加 Bedrock Titan Embedding 配置
INSERT INTO ai_configs (
    name,
    service_type, 
    model_id,
    aws_region,
    enabled, 
    capabilities, 
    priority,
    created_at,
    updated_at
) VALUES (
    'Bedrock Titan Embedding (1024维)',
    'bedrock',
    'amazon.titan-embed-text-v1',
    'us-east-1',
    false,  -- 专用配置
    '["embedding"]',
    20,
    NOW(),
    NOW()
) ON CONFLICT DO NOTHING;

-- 5. 清空 embedding_tasks 队列（因为维度变了，需要重新生成）
DELETE FROM embedding_tasks;

-- 6. 清空所有资源的 embedding 相关字段
UPDATE resource_index SET 
    embedding = NULL,
    embedding_text = NULL,
    embedding_model = NULL,
    embedding_updated_at = NULL;

-- 验证
SELECT 
    'embedding 列维度已更新为 1024' as status,
    (SELECT COUNT(*) FROM ai_configs WHERE capabilities @> '["embedding"]') as embedding_configs_count;
