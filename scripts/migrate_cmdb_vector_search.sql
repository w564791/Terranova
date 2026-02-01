-- CMDB 向量化搜索：综合迁移脚本
-- 执行方式: PGPASSWORD=postgres123 psql -h localhost -p 15432 -U postgres -d iac_platform -f scripts/migrate_cmdb_vector_search.sql
-- 
-- 此脚本包含以下内容：
-- 1. 安装 pgvector 扩展
-- 2. 添加 embedding 相关列到 resource_index 表
-- 3. 创建 embedding_tasks 队列表
-- 4. 添加 embedding 能力的 AI 配置
-- 5. 创建必要的索引
--
-- 注意：HNSW 索引不支持超过 2000 维，使用 IVFFlat 索引代替

-- ============================================================
-- 1. 安装 pgvector 扩展
-- ============================================================
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        CREATE EXTENSION vector;
        RAISE NOTICE '✅ pgvector 扩展已安装';
    ELSE
        RAISE NOTICE '✅ pgvector 扩展已存在';
    END IF;
END $$;

-- ============================================================
-- 2. 添加 embedding 相关列到 resource_index 表
-- ============================================================
ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding VECTOR(3072);

ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding_text TEXT;

ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding_model VARCHAR(100);

ALTER TABLE resource_index 
ADD COLUMN IF NOT EXISTS embedding_updated_at TIMESTAMP;

-- 添加注释
COMMENT ON COLUMN resource_index.embedding IS '资源的语义向量（3072维，OpenAI text-embedding-3-large）';
COMMENT ON COLUMN resource_index.embedding_text IS '用于生成 embedding 的原始文本';
COMMENT ON COLUMN resource_index.embedding_model IS '使用的 embedding 模型 ID';
COMMENT ON COLUMN resource_index.embedding_updated_at IS 'embedding 最后更新时间';

-- ============================================================
-- 3. 创建 embedding_tasks 队列表
-- ============================================================
CREATE TABLE IF NOT EXISTS embedding_tasks (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL,
    workspace_id VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    retry_count INTEGER DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP,
    
    CONSTRAINT fk_embedding_task_resource 
        FOREIGN KEY (resource_id) 
        REFERENCES resource_index(id) 
        ON DELETE CASCADE,
    
    CONSTRAINT uk_embedding_task_resource UNIQUE (resource_id)
);

-- 添加注释
COMMENT ON TABLE embedding_tasks IS 'Embedding 生成任务队列表';
COMMENT ON COLUMN embedding_tasks.resource_id IS '关联的资源 ID（resource_index.id）';
COMMENT ON COLUMN embedding_tasks.workspace_id IS '资源所属的 Workspace ID';
COMMENT ON COLUMN embedding_tasks.status IS '任务状态：pending-待处理, processing-处理中, completed-已完成, failed-失败';
COMMENT ON COLUMN embedding_tasks.retry_count IS '重试次数，超过 3 次视为失败';
COMMENT ON COLUMN embedding_tasks.error_message IS '错误信息';

-- ============================================================
-- 4. 创建索引
-- ============================================================

-- 注意：HNSW 索引不支持超过 2000 维的向量
-- 使用 IVFFlat 索引代替（需要先有数据才能创建，这里先跳过）
-- 或者使用精确搜索（不创建向量索引，对于小数据集足够）

-- 部分索引（只索引有 embedding 的记录）
CREATE INDEX IF NOT EXISTS idx_resource_has_embedding ON resource_index (id)
WHERE embedding IS NOT NULL;

-- 复合索引（用于按资源类型和 workspace 过滤后的向量搜索）
CREATE INDEX IF NOT EXISTS idx_resource_type_workspace_embedding ON resource_index (resource_type, workspace_id)
WHERE embedding IS NOT NULL;

-- embedding_tasks 表索引
CREATE INDEX IF NOT EXISTS idx_embedding_tasks_status ON embedding_tasks(status);
CREATE INDEX IF NOT EXISTS idx_embedding_tasks_workspace ON embedding_tasks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_embedding_tasks_created ON embedding_tasks(created_at);
CREATE INDEX IF NOT EXISTS idx_embedding_tasks_pending ON embedding_tasks(status, retry_count, created_at)
WHERE status = 'pending';

-- ============================================================
-- 5. 添加 embedding 能力的 AI 配置
-- ============================================================

-- OpenAI text-embedding-3-large (3072 维，高精度，推荐)
INSERT INTO ai_configs (
    service_type, 
    model_id,
    api_key,
    base_url,
    enabled, 
    capabilities, 
    priority,
    created_at,
    updated_at
) VALUES (
    'openai',
    'text-embedding-3-large',
    '',
    'https://api.openai.com/v1',
    false,
    '["embedding"]',
    20,
    NOW(),
    NOW()
) ON CONFLICT DO NOTHING;

-- OpenAI text-embedding-3-small (1536 维，成本更低，备选)
INSERT INTO ai_configs (
    service_type, 
    model_id,
    api_key,
    base_url,
    enabled, 
    capabilities, 
    priority,
    created_at,
    updated_at
) VALUES (
    'openai',
    'text-embedding-3-small',
    '',
    'https://api.openai.com/v1',
    false,
    '["embedding"]',
    10,
    NOW(),
    NOW()
) ON CONFLICT DO NOTHING;

-- ============================================================
-- 6. 验证迁移结果
-- ============================================================
DO $$
DECLARE
    col_count INTEGER;
    table_exists BOOLEAN;
    config_count INTEGER;
BEGIN
    -- 检查 embedding 列
    SELECT COUNT(*) INTO col_count
    FROM information_schema.columns 
    WHERE table_name = 'resource_index' 
    AND column_name IN ('embedding', 'embedding_text', 'embedding_model', 'embedding_updated_at');
    
    IF col_count = 4 THEN
        RAISE NOTICE '✅ resource_index 表 embedding 列添加成功（共 4 列）';
    ELSE
        RAISE WARNING '⚠️ resource_index 表部分列添加失败，当前只有 % 列', col_count;
    END IF;
    
    -- 检查 embedding_tasks 表
    SELECT EXISTS (
        SELECT 1 FROM information_schema.tables WHERE table_name = 'embedding_tasks'
    ) INTO table_exists;
    
    IF table_exists THEN
        RAISE NOTICE '✅ embedding_tasks 表创建成功';
    ELSE
        RAISE WARNING '⚠️ embedding_tasks 表创建失败';
    END IF;
    
    -- 检查 AI 配置
    SELECT COUNT(*) INTO config_count
    FROM ai_configs 
    WHERE capabilities::text LIKE '%embedding%';
    
    IF config_count >= 1 THEN
        RAISE NOTICE '✅ embedding AI 配置添加成功（共 % 个配置）', config_count;
    ELSE
        RAISE WARNING '⚠️ embedding AI 配置添加失败或不完整';
    END IF;
    
    RAISE NOTICE '';
    RAISE NOTICE '========================================';
    RAISE NOTICE '⚠️ 重要提示：';
    RAISE NOTICE '1. 请在 AI 配置管理界面填写 OpenAI API Key';
    RAISE NOTICE '2. 向量索引将在有数据后自动创建';
    RAISE NOTICE '3. 对于小数据集，精确搜索性能足够';
    RAISE NOTICE '========================================';
END $$;

-- 显示 embedding 配置
SELECT 
    id,
    service_type,
    model_id,
    CASE WHEN api_key = '' THEN '❌ 未配置' ELSE '✅ 已配置' END as api_key_status,
    enabled,
    capabilities,
    priority
FROM ai_configs 
WHERE capabilities::text LIKE '%embedding%'
ORDER BY priority DESC;
