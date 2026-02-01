-- Agent和K8s执行模式数据库迁移脚本
-- 版本: v1.0
-- 日期: 2025-10-09

-- ============================================
-- 1. agents表 - Agent管理
-- ============================================
CREATE TABLE IF NOT EXISTS agents (
    id SERIAL PRIMARY KEY,
    
    -- Agent唯一标识（由Agent自己生成，如hostname+uuid）
    agent_id VARCHAR(255) NOT NULL UNIQUE,
    
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- Agent类型
    agent_type VARCHAR(50) NOT NULL DEFAULT 'remote', -- 'remote', 'k8s'
    
    -- 状态
    status VARCHAR(50) NOT NULL DEFAULT 'offline', -- 'online', 'offline', 'busy', 'error'
    
    -- 标签（JSON数组）
    labels JSONB DEFAULT '[]',
    
    -- 能力（JSON对象）
    capabilities JSONB DEFAULT '{}',
    -- 例如: {"terraform_versions": ["1.5.0", "1.6.0"], "max_concurrent_tasks": 3}
    
    -- Token（用于Agent认证，多个Agent可以共享同一个Token）
    token VARCHAR(255) NOT NULL,
    token_expires_at TIMESTAMP,
    
    -- 连接信息
    endpoint VARCHAR(500), -- Agent的API端点
    
    -- 最后心跳时间
    last_heartbeat_at TIMESTAMP,
    
    -- 统计信息
    total_tasks INTEGER DEFAULT 0,
    success_tasks INTEGER DEFAULT 0,
    failed_tasks INTEGER DEFAULT 0,
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_agents_agent_id ON agents(agent_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_labels ON agents USING GIN(labels);
CREATE INDEX IF NOT EXISTS idx_agents_token ON agents(token);
CREATE INDEX IF NOT EXISTS idx_agents_deleted_at ON agents(deleted_at);

-- ============================================
-- 2. agent_pools表 - Agent池管理
-- ============================================
CREATE TABLE IF NOT EXISTS agent_pools (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- 池类型
    pool_type VARCHAR(50) NOT NULL DEFAULT 'static', -- 'static', 'dynamic'
    
    -- 选择策略
    selection_strategy VARCHAR(50) DEFAULT 'round_robin', 
    -- 'round_robin', 'least_busy', 'random', 'label_match'
    
    -- 标签要求（JSON数组）
    required_labels JSONB DEFAULT '[]',
    
    -- 关联的Agent ID列表（JSON数组）
    agent_ids JSONB DEFAULT '[]',
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_agent_pools_deleted_at ON agent_pools(deleted_at);

-- ============================================
-- 3. k8s_configs表 - K8s全局配置
-- ============================================
CREATE TABLE IF NOT EXISTS k8s_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    
    -- K8s集群配置
    kubeconfig TEXT, -- base64编码的kubeconfig
    context_name VARCHAR(255), -- 使用的context
    namespace VARCHAR(255) DEFAULT 'default',
    
    -- Pod模板配置
    pod_template JSONB NOT NULL,
    -- 例如:
    -- {
    --   "image": "hashicorp/terraform:1.6.0",
    --   "serviceAccountName": "terraform-runner",
    --   "resources": {
    --     "requests": {"cpu": "500m", "memory": "512Mi"},
    --     "limits": {"cpu": "1000m", "memory": "1Gi"}
    --   },
    --   "env": [
    --     {"name": "TF_LOG", "value": "INFO"}
    --   ]
    -- }
    
    -- ServiceAccount配置
    service_account_name VARCHAR(255) DEFAULT 'default',
    
    -- 镜像拉取密钥
    image_pull_secrets JSONB DEFAULT '[]',
    
    -- 是否为默认配置
    is_default BOOLEAN DEFAULT false,
    
    -- 状态
    status VARCHAR(50) DEFAULT 'active', -- 'active', 'inactive'
    
    -- 元数据
    metadata JSONB DEFAULT '{}',
    
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_k8s_configs_is_default ON k8s_configs(is_default);
CREATE INDEX IF NOT EXISTS idx_k8s_configs_deleted_at ON k8s_configs(deleted_at);

-- ============================================
-- 4. 更新workspace_tasks表 - 添加Agent和K8s相关字段
-- ============================================

-- 添加agent_id字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS agent_id INTEGER REFERENCES agents(id);

-- 添加k8s_config_id字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS k8s_config_id INTEGER REFERENCES k8s_configs(id);

-- 添加k8s_pod_name字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS k8s_pod_name VARCHAR(255);

-- 添加execution_node字段（执行节点标识）
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS execution_node VARCHAR(255);

-- 添加任务锁相关字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS locked_by VARCHAR(255); -- Agent ID
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS locked_at TIMESTAMP;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS lock_expires_at TIMESTAMP;

-- 索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_agent_id ON workspace_tasks(agent_id);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_k8s_pod_name ON workspace_tasks(k8s_pod_name);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_locked_by ON workspace_tasks(locked_by);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_lock_expires_at ON workspace_tasks(lock_expires_at);

-- ============================================
-- 5. 更新workspaces表 - 添加Agent Pool和K8s配置关联
-- ============================================

-- 添加agent_pool_id字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS agent_pool_id INTEGER REFERENCES agent_pools(id);

-- 添加k8s_config_id字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS k8s_config_id INTEGER REFERENCES k8s_configs(id);

-- 索引
CREATE INDEX IF NOT EXISTS idx_workspaces_agent_pool_id ON workspaces(agent_pool_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_k8s_config_id ON workspaces(k8s_config_id);

-- ============================================
-- 6. 创建触发器 - 自动更新updated_at
-- ============================================

-- agents表触发器
CREATE OR REPLACE FUNCTION update_agents_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_agents_updated_at ON agents;
CREATE TRIGGER trigger_update_agents_updated_at
    BEFORE UPDATE ON agents
    FOR EACH ROW
    EXECUTE FUNCTION update_agents_updated_at();

-- agent_pools表触发器
CREATE OR REPLACE FUNCTION update_agent_pools_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_agent_pools_updated_at ON agent_pools;
CREATE TRIGGER trigger_update_agent_pools_updated_at
    BEFORE UPDATE ON agent_pools
    FOR EACH ROW
    EXECUTE FUNCTION update_agent_pools_updated_at();

-- k8s_configs表触发器
CREATE OR REPLACE FUNCTION update_k8s_configs_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_k8s_configs_updated_at ON k8s_configs;
CREATE TRIGGER trigger_update_k8s_configs_updated_at
    BEFORE UPDATE ON k8s_configs
    FOR EACH ROW
    EXECUTE FUNCTION update_k8s_configs_updated_at();

-- ============================================
-- 7. 插入示例数据（可选）
-- ============================================

-- 示例Agent Pool
INSERT INTO agent_pools (name, description, pool_type, selection_strategy, required_labels)
VALUES 
    ('default-pool', 'Default agent pool', 'static', 'round_robin', '[]'),
    ('prod-pool', 'Production agent pool', 'static', 'least_busy', '["prod"]')
ON CONFLICT (name) DO NOTHING;

-- 示例K8s配置
INSERT INTO k8s_configs (
    name, 
    description, 
    namespace, 
    pod_template, 
    service_account_name,
    is_default
)
VALUES (
    'default-k8s',
    'Default Kubernetes configuration',
    'terraform',
    '{
        "image": "hashicorp/terraform:1.6.0",
        "serviceAccountName": "terraform-runner",
        "resources": {
            "requests": {"cpu": "500m", "memory": "512Mi"},
            "limits": {"cpu": "1000m", "memory": "1Gi"}
        },
        "env": [
            {"name": "TF_LOG", "value": "INFO"}
        ]
    }'::jsonb,
    'terraform-runner',
    true
)
ON CONFLICT (name) DO NOTHING;

-- ============================================
-- 8. 验证
-- ============================================

-- 验证表是否创建成功
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'agents') THEN
        RAISE NOTICE 'agents表创建成功';
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'agent_pools') THEN
        RAISE NOTICE 'agent_pools表创建成功';
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'k8s_configs') THEN
        RAISE NOTICE 'k8s_configs表创建成功';
    END IF;
    
    RAISE NOTICE '数据库迁移完成！';
END $$;
