-- 添加 Vector 搜索配置字段到 ai_configs 表
-- 这些字段仅用于 embedding 能力的配置

-- 添加 top_k 字段（向量搜索返回的最大结果数）
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS top_k INTEGER DEFAULT 50;

-- 添加 similarity_threshold 字段（向量搜索相似度阈值）
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS similarity_threshold DOUBLE PRECISION DEFAULT 0.3;

-- 添加注释
COMMENT ON COLUMN ai_configs.top_k IS '向量搜索返回的最大结果数（仅 embedding 能力使用）';
COMMENT ON COLUMN ai_configs.similarity_threshold IS '向量搜索相似度阈值 0-1（仅 embedding 能力使用）';
