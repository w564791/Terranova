-- 创建资源索引表
-- 用于CMDB树状结构功能，存储从Terraform state解析的资源信息

-- 1. 资源索引表
CREATE TABLE IF NOT EXISTS resource_index (
    id SERIAL PRIMARY KEY,
    
    -- 基本标识
    workspace_id VARCHAR(50) NOT NULL,
    terraform_address TEXT NOT NULL,        -- 完整Terraform地址，如 module.ec2.aws_instance.main[0]
    
    -- 资源信息
    resource_type VARCHAR(100) NOT NULL,    -- Terraform资源类型，如 aws_security_group
    resource_name VARCHAR(100) NOT NULL,    -- Terraform资源名称，如 main
    resource_mode VARCHAR(20) NOT NULL DEFAULT 'managed',  -- managed 或 data
    index_key TEXT,                         -- count/for_each的key，如 "0" 或 "primary"
    
    -- 云资源信息（从attributes提取）
    cloud_resource_id VARCHAR(255),         -- 云资源ID，如 sg-0123456789
    cloud_resource_name VARCHAR(255),       -- 云资源名称（从name或tags.Name提取）
    cloud_resource_arn TEXT,                -- ARN（如果有）
    description TEXT,                       -- 资源描述
    
    -- Module层级信息
    module_path TEXT,                       -- module路径，如 module.ec2.module.sg
    module_depth INTEGER DEFAULT 0,         -- module嵌套深度
    parent_module_path TEXT,                -- 父module路径
    root_module_name VARCHAR(100),          -- 根module名称（平台资源名）
    
    -- 属性快照
    attributes JSONB,                       -- 资源属性（可选，用于搜索）
    tags JSONB,                             -- 资源标签
    
    -- 元数据
    provider VARCHAR(200),                  -- provider信息
    state_version_id INTEGER,               -- 关联的state版本
    last_synced_at TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    
    -- 唯一约束
    CONSTRAINT uk_resource_index_address UNIQUE (workspace_id, terraform_address)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_resource_index_workspace ON resource_index(workspace_id);
CREATE INDEX IF NOT EXISTS idx_resource_index_type ON resource_index(resource_type);
CREATE INDEX IF NOT EXISTS idx_resource_index_cloud_id ON resource_index(cloud_resource_id);
CREATE INDEX IF NOT EXISTS idx_resource_index_cloud_name ON resource_index(cloud_resource_name);
CREATE INDEX IF NOT EXISTS idx_resource_index_module_path ON resource_index(module_path);
CREATE INDEX IF NOT EXISTS idx_resource_index_root_module ON resource_index(root_module_name);
CREATE INDEX IF NOT EXISTS idx_resource_index_mode ON resource_index(resource_mode);
CREATE INDEX IF NOT EXISTS idx_resource_index_parent_module ON resource_index(parent_module_path);

-- 全文搜索索引（用于资源搜索）
CREATE INDEX IF NOT EXISTS idx_resource_index_search ON resource_index 
    USING GIN (to_tsvector('english', 
        COALESCE(cloud_resource_id, '') || ' ' || 
        COALESCE(cloud_resource_name, '') || ' ' || 
        COALESCE(description, '') || ' ' ||
        COALESCE(terraform_address, '')
    ));

-- 2. Module层级表（用于优化树状查询）
CREATE TABLE IF NOT EXISTS module_hierarchy (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL,
    module_path TEXT NOT NULL,              -- 完整module路径
    module_name VARCHAR(100) NOT NULL,      -- module名称
    module_key TEXT,                        -- for_each的key
    parent_path TEXT,                       -- 父module路径
    depth INTEGER DEFAULT 0,                -- 嵌套深度
    
    -- 统计信息
    resource_count INTEGER DEFAULT 0,       -- 直接包含的资源数
    total_resource_count INTEGER DEFAULT 0, -- 包含子module的总资源数
    child_module_count INTEGER DEFAULT 0,   -- 子module数量
    
    -- 元数据
    source VARCHAR(500),                    -- module source（如果能获取）
    last_synced_at TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    
    CONSTRAINT uk_module_hierarchy UNIQUE (workspace_id, module_path)
);

CREATE INDEX IF NOT EXISTS idx_module_hierarchy_workspace ON module_hierarchy(workspace_id);
CREATE INDEX IF NOT EXISTS idx_module_hierarchy_parent ON module_hierarchy(parent_path);
CREATE INDEX IF NOT EXISTS idx_module_hierarchy_depth ON module_hierarchy(depth);

-- 3. 添加外键约束（可选，根据需要启用）
-- ALTER TABLE resource_index 
--     ADD CONSTRAINT fk_resource_index_workspace 
--     FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE;

-- ALTER TABLE module_hierarchy 
--     ADD CONSTRAINT fk_module_hierarchy_workspace 
--     FOREIGN KEY (workspace_id) REFERENCES workspaces(workspace_id) ON DELETE CASCADE;

-- 4. 创建视图：workspace资源统计
CREATE OR REPLACE VIEW v_workspace_resource_stats AS
SELECT 
    workspace_id,
    COUNT(*) as total_resources,
    COUNT(DISTINCT resource_type) as resource_type_count,
    COUNT(DISTINCT root_module_name) as root_module_count,
    COUNT(*) FILTER (WHERE resource_mode = 'managed') as managed_count,
    COUNT(*) FILTER (WHERE resource_mode = 'data') as data_count,
    MAX(last_synced_at) as last_synced_at
FROM resource_index
GROUP BY workspace_id;

-- 5. 创建视图：资源类型统计
CREATE OR REPLACE VIEW v_resource_type_stats AS
SELECT 
    workspace_id,
    resource_type,
    COUNT(*) as count,
    COUNT(DISTINCT cloud_resource_id) as unique_cloud_ids
FROM resource_index
WHERE resource_mode = 'managed'
GROUP BY workspace_id, resource_type
ORDER BY workspace_id, count DESC;

-- 6. 创建函数：构建资源树
CREATE OR REPLACE FUNCTION get_workspace_resource_tree(p_workspace_id VARCHAR(50))
RETURNS TABLE (
    node_type VARCHAR(20),
    node_name VARCHAR(255),
    node_path TEXT,
    terraform_address TEXT,
    resource_type VARCHAR(100),
    cloud_resource_id VARCHAR(255),
    cloud_resource_name VARCHAR(255),
    description TEXT,
    depth INTEGER,
    parent_path TEXT,
    resource_count INTEGER
) AS $$
BEGIN
    -- 返回module节点
    RETURN QUERY
    SELECT 
        'module'::VARCHAR(20) as node_type,
        mh.module_name::VARCHAR(255) as node_name,
        mh.module_path as node_path,
        NULL::TEXT as terraform_address,
        NULL::VARCHAR(100) as resource_type,
        NULL::VARCHAR(255) as cloud_resource_id,
        NULL::VARCHAR(255) as cloud_resource_name,
        NULL::TEXT as description,
        mh.depth,
        mh.parent_path,
        mh.total_resource_count as resource_count
    FROM module_hierarchy mh
    WHERE mh.workspace_id = p_workspace_id
    
    UNION ALL
    
    -- 返回资源节点
    SELECT 
        'resource'::VARCHAR(20) as node_type,
        ri.resource_name::VARCHAR(255) as node_name,
        ri.module_path as node_path,
        ri.terraform_address,
        ri.resource_type,
        ri.cloud_resource_id,
        ri.cloud_resource_name,
        ri.description,
        ri.module_depth + 1 as depth,
        ri.module_path as parent_path,
        1 as resource_count
    FROM resource_index ri
    WHERE ri.workspace_id = p_workspace_id
    
    ORDER BY depth, node_path, node_type DESC, node_name;
END;
$$ LANGUAGE plpgsql;

-- 7. 创建函数：搜索资源（支持跳转到资源预览页面）
CREATE OR REPLACE FUNCTION search_resources(
    p_query TEXT,
    p_workspace_id VARCHAR(50) DEFAULT NULL,
    p_resource_type VARCHAR(100) DEFAULT NULL,
    p_limit INTEGER DEFAULT 20
)
RETURNS TABLE (
    workspace_id VARCHAR(50),
    terraform_address TEXT,
    resource_type VARCHAR(100),
    resource_name VARCHAR(100),
    cloud_resource_id VARCHAR(255),
    cloud_resource_name VARCHAR(255),
    description TEXT,
    module_path TEXT,
    root_module_name VARCHAR(100),
    platform_resource_id INTEGER,
    platform_resource_name VARCHAR(100),
    jump_url TEXT,
    match_rank REAL
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        ri.workspace_id,
        ri.terraform_address,
        ri.resource_type,
        ri.resource_name,
        ri.cloud_resource_id,
        ri.cloud_resource_name,
        ri.description,
        ri.module_path,
        ri.root_module_name,
        wr.id::INTEGER as platform_resource_id,
        wr.resource_name::VARCHAR(100) as platform_resource_name,
        CASE 
            WHEN wr.id IS NOT NULL THEN 
                CONCAT('/workspaces/', ri.workspace_id, '/resources/', wr.id)
            ELSE NULL
        END as jump_url,
        CASE 
            -- 精确匹配cloud_resource_id得分最高
            WHEN ri.cloud_resource_id = p_query THEN 1.0
            -- 精确匹配cloud_resource_name
            WHEN ri.cloud_resource_name = p_query THEN 0.9
            -- 前缀匹配
            WHEN ri.cloud_resource_id LIKE p_query || '%' THEN 0.8
            WHEN ri.cloud_resource_name LIKE p_query || '%' THEN 0.7
            -- 包含匹配
            WHEN ri.cloud_resource_id LIKE '%' || p_query || '%' THEN 0.6
            WHEN ri.cloud_resource_name LIKE '%' || p_query || '%' THEN 0.5
            WHEN ri.description LIKE '%' || p_query || '%' THEN 0.4
            WHEN ri.terraform_address LIKE '%' || p_query || '%' THEN 0.3
            ELSE 0.1
        END::REAL as match_rank
    FROM resource_index ri
    LEFT JOIN workspace_resources wr ON 
        ri.workspace_id = wr.workspace_id 
        AND ri.root_module_name = wr.resource_name
        AND wr.is_active = true
    WHERE 
        ri.resource_mode = 'managed'
        AND (p_workspace_id IS NULL OR ri.workspace_id = p_workspace_id)
        AND (p_resource_type IS NULL OR ri.resource_type = p_resource_type)
        AND (
            ri.cloud_resource_id ILIKE '%' || p_query || '%'
            OR ri.cloud_resource_name ILIKE '%' || p_query || '%'
            OR ri.description ILIKE '%' || p_query || '%'
            OR ri.terraform_address ILIKE '%' || p_query || '%'
        )
    ORDER BY match_rank DESC, ri.cloud_resource_name
    LIMIT p_limit;
END;
$$ LANGUAGE plpgsql;

-- 8. 创建函数：根据云资源ID查找平台资源（用于快速跳转）
CREATE OR REPLACE FUNCTION find_platform_resource_by_cloud_id(
    p_cloud_resource_id VARCHAR(255)
)
RETURNS TABLE (
    workspace_id VARCHAR(50),
    workspace_name VARCHAR(100),
    platform_resource_id INTEGER,
    platform_resource_name VARCHAR(100),
    terraform_address TEXT,
    resource_type VARCHAR(100),
    cloud_resource_name VARCHAR(255),
    module_path TEXT,
    jump_url TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        ri.workspace_id,
        w.name::VARCHAR(100) as workspace_name,
        wr.id::INTEGER as platform_resource_id,
        wr.resource_name::VARCHAR(100) as platform_resource_name,
        ri.terraform_address,
        ri.resource_type,
        ri.cloud_resource_name,
        ri.module_path,
        CASE 
            WHEN wr.id IS NOT NULL THEN 
                CONCAT('/workspaces/', ri.workspace_id, '/resources/', wr.id)
            ELSE NULL
        END as jump_url
    FROM resource_index ri
    LEFT JOIN workspace_resources wr ON 
        ri.workspace_id = wr.workspace_id 
        AND ri.root_module_name = wr.resource_name
        AND wr.is_active = true
    LEFT JOIN workspaces w ON ri.workspace_id = w.workspace_id
    WHERE ri.cloud_resource_id = p_cloud_resource_id
    AND ri.resource_mode = 'managed';
END;
$$ LANGUAGE plpgsql;

-- 验证表创建
SELECT 
    table_name,
    (SELECT COUNT(*) FROM information_schema.columns WHERE table_name = t.table_name) as column_count
FROM information_schema.tables t
WHERE table_schema = 'public' 
    AND table_name IN ('resource_index', 'module_hierarchy')
ORDER BY table_name;

-- 输出创建结果
DO $$
BEGIN
    RAISE NOTICE 'Resource index tables created successfully';
    RAISE NOTICE 'Tables: resource_index, module_hierarchy';
    RAISE NOTICE 'Views: v_workspace_resource_stats, v_resource_type_stats';
    RAISE NOTICE 'Functions: get_workspace_resource_tree, search_resources';
END $$;
