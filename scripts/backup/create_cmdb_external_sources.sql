-- 创建CMDB外部数据源相关表
-- 用于支持从第三方CMDB系统同步资源数据

-- ============================================
-- 1. 创建外部数据源配置表
-- ============================================
CREATE TABLE IF NOT EXISTS cmdb_external_sources (
    id SERIAL PRIMARY KEY,
    source_id VARCHAR(50) UNIQUE NOT NULL,           -- 唯一标识: cmdb-src-{随机字符}
    name VARCHAR(100) NOT NULL,                       -- 数据源名称
    description TEXT,                                 -- 描述
    
    -- API配置
    api_endpoint VARCHAR(500) NOT NULL,               -- API端点URL
    http_method VARCHAR(10) DEFAULT 'GET',            -- HTTP方法: GET/POST
    request_body TEXT,                                -- POST请求体模板（可选）
    
    -- 认证配置（Header）
    auth_headers JSONB,                               -- Header配置: [{"key": "X-API-Key", "secret_id": "secret-xxx"}, ...]
    
    -- 数据映射配置
    response_path VARCHAR(200),                       -- 响应数据路径（JSONPath），如 "$.data.resources"
    field_mapping JSONB NOT NULL,                     -- 字段映射配置
    
    -- 主键配置
    primary_key_field VARCHAR(100) NOT NULL,          -- 主键字段路径，如 "$.id" 或 "$.name"
    
    -- 云环境配置
    cloud_provider VARCHAR(50),                       -- 云提供商: aws/azure/gcp/aliyun 等（用户输入）
    account_id VARCHAR(100),                          -- 云账户ID（用户输入）
    account_name VARCHAR(200),                        -- 云账户名称（用户输入，可选）
    region VARCHAR(50),                               -- 区域（用户输入，可选）
    
    -- 同步配置
    sync_interval_minutes INT DEFAULT 60,             -- 同步间隔（分钟），0表示手动同步
    is_enabled BOOLEAN DEFAULT true,                  -- 是否启用
    
    -- 过滤配置
    resource_type_filter VARCHAR(100),                -- 资源类型过滤（可选）
    
    -- 元数据
    organization_id VARCHAR(50),                      -- 所属组织（可选，用于多租户）
    created_by VARCHAR(50),
    updated_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_sync_at TIMESTAMP,                           -- 最后同步时间
    last_sync_status VARCHAR(20),                     -- 最后同步状态: success/failed/running
    last_sync_message TEXT,                           -- 最后同步消息
    last_sync_count INT DEFAULT 0                     -- 最后同步资源数量
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_org ON cmdb_external_sources(organization_id);
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_enabled ON cmdb_external_sources(is_enabled);
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_provider ON cmdb_external_sources(cloud_provider);
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_account ON cmdb_external_sources(account_id);
CREATE INDEX IF NOT EXISTS idx_cmdb_external_sources_name ON cmdb_external_sources(name);

-- 添加注释
COMMENT ON TABLE cmdb_external_sources IS '外部CMDB数据源配置表';
COMMENT ON COLUMN cmdb_external_sources.source_id IS '唯一标识，格式: cmdb-src-{16位随机字符}';
COMMENT ON COLUMN cmdb_external_sources.auth_headers IS 'Header配置，格式: [{"key": "X-API-Key", "secret_id": "secret-xxx"}]';
COMMENT ON COLUMN cmdb_external_sources.field_mapping IS '字段映射配置，JSONPath格式';
COMMENT ON COLUMN cmdb_external_sources.primary_key_field IS '主键字段路径，用于唯一标识资源';
COMMENT ON COLUMN cmdb_external_sources.cloud_provider IS '云提供商: aws/azure/gcp/aliyun等';
COMMENT ON COLUMN cmdb_external_sources.sync_interval_minutes IS '同步间隔（分钟），0表示手动同步';
COMMENT ON COLUMN cmdb_external_sources.last_sync_status IS '最后同步状态: success/failed/running';

-- ============================================
-- 2. 创建同步日志表
-- ============================================
CREATE TABLE IF NOT EXISTS cmdb_sync_logs (
    id SERIAL PRIMARY KEY,
    source_id VARCHAR(50) NOT NULL,
    started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    status VARCHAR(20) NOT NULL DEFAULT 'running',    -- running/success/failed
    resources_synced INT DEFAULT 0,
    resources_added INT DEFAULT 0,
    resources_updated INT DEFAULT 0,
    resources_deleted INT DEFAULT 0,
    error_message TEXT,
    
    CONSTRAINT fk_cmdb_sync_logs_source 
        FOREIGN KEY (source_id) 
        REFERENCES cmdb_external_sources(source_id) 
        ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_source ON cmdb_sync_logs(source_id);
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_started ON cmdb_sync_logs(started_at);
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_status ON cmdb_sync_logs(status);

-- 添加注释
COMMENT ON TABLE cmdb_sync_logs IS 'CMDB外部数据源同步日志表';
COMMENT ON COLUMN cmdb_sync_logs.status IS '同步状态: running/success/failed';

-- ============================================
-- 3. 扩展resource_index表
-- ============================================

-- 数据来源字段
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) DEFAULT 'terraform';
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS external_source_id VARCHAR(50);

-- 云环境字段
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_provider VARCHAR(50);
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_account_id VARCHAR(100);
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_account_name VARCHAR(200);
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS cloud_region VARCHAR(50);

-- 主键字段
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS primary_key_value VARCHAR(500);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_resource_index_source_type ON resource_index(source_type);
CREATE INDEX IF NOT EXISTS idx_resource_index_external_source ON resource_index(external_source_id);
CREATE INDEX IF NOT EXISTS idx_resource_index_cloud_provider ON resource_index(cloud_provider);
CREATE INDEX IF NOT EXISTS idx_resource_index_cloud_account ON resource_index(cloud_account_id);
CREATE INDEX IF NOT EXISTS idx_resource_index_primary_key ON resource_index(primary_key_value);

-- 添加注释
COMMENT ON COLUMN resource_index.source_type IS '数据来源: terraform(默认)/external';
COMMENT ON COLUMN resource_index.external_source_id IS '外部数据源ID，仅当source_type=external时有值';
COMMENT ON COLUMN resource_index.cloud_provider IS '云提供商: aws/azure/gcp/aliyun等';
COMMENT ON COLUMN resource_index.cloud_account_id IS '云账户ID';
COMMENT ON COLUMN resource_index.cloud_account_name IS '云账户名称';
COMMENT ON COLUMN resource_index.cloud_region IS '云区域';
COMMENT ON COLUMN resource_index.primary_key_value IS '主键值，用于唯一标识资源';

-- ============================================
-- 4. 更新现有数据的source_type为terraform
-- ============================================
UPDATE resource_index SET source_type = 'terraform' WHERE source_type IS NULL;

-- ============================================
-- 完成
-- ============================================
SELECT 'CMDB外部数据源表创建完成' AS status;
