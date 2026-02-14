-- 修改 embedding 列维度从 1024 到 1536
-- 注意：这会清空所有现有的 embedding 数据

-- 1. 先清空现有 embedding
UPDATE resource_index SET embedding = NULL;

-- 2. 删除旧列
ALTER TABLE resource_index DROP COLUMN IF EXISTS embedding;

-- 3. 添加新列（1536 维度，支持 Cohere Embed v4）
ALTER TABLE resource_index ADD COLUMN embedding vector(1536);

-- 4. 重建索引（如果有的话）
-- DROP INDEX IF EXISTS resource_index_embedding_idx;
-- CREATE INDEX resource_index_embedding_idx ON resource_index USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- 验证
SELECT column_name, udt_name, 
       (SELECT atttypmod FROM pg_attribute WHERE attrelid = 'resource_index'::regclass AND attname = 'embedding') as dimension
FROM information_schema.columns 
WHERE table_name = 'resource_index' AND column_name = 'embedding';
