-- 关键词向量缓存表
-- 用于缓存常用关键词的 embedding 向量，避免重复调用 API

CREATE TABLE IF NOT EXISTS keyword_embedding_cache (
    id SERIAL PRIMARY KEY,
    keyword VARCHAR(500) NOT NULL,
    keyword_hash VARCHAR(64) NOT NULL UNIQUE,  -- SHA256 hash，用于索引
    embedding vector(1536) NOT NULL,
    embedding_model VARCHAR(100) NOT NULL,
    hit_count INTEGER DEFAULT 0,
    last_hit_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 使用 hash 索引，避免长字符串索引问题
CREATE INDEX IF NOT EXISTS idx_keyword_embedding_hash ON keyword_embedding_cache(keyword_hash);

-- 统计索引，用于清理低频缓存
CREATE INDEX IF NOT EXISTS idx_keyword_embedding_hit_count ON keyword_embedding_cache(hit_count);

-- 模型索引，用于按模型查询
CREATE INDEX IF NOT EXISTS idx_keyword_embedding_model ON keyword_embedding_cache(embedding_model);

COMMENT ON TABLE keyword_embedding_cache IS '关键词向量缓存表，用于加速向量搜索';
COMMENT ON COLUMN keyword_embedding_cache.keyword IS '原始关键词';
COMMENT ON COLUMN keyword_embedding_cache.keyword_hash IS '关键词的 SHA256 哈希，用于快速查找';
COMMENT ON COLUMN keyword_embedding_cache.embedding IS '向量数据（1536维）';
COMMENT ON COLUMN keyword_embedding_cache.embedding_model IS '生成向量使用的模型 ID';
COMMENT ON COLUMN keyword_embedding_cache.hit_count IS '缓存命中次数';
COMMENT ON COLUMN keyword_embedding_cache.last_hit_at IS '最后一次命中时间';