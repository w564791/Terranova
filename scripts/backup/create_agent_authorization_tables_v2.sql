-- ============================================================================
-- Agent-Workspace 双向授权系统 - 授权关系表
-- 注意: agents 表已通过 migrate_agents_and_pools_to_semantic_id.sql 创建
-- 本脚本只创建授权关系表
-- ============================================================================

-- 第一步: 创建 Agent 允许的 Workspace 表 (Agent 侧)
-- ============================================================================
CREATE TABLE IF NOT EXISTS agent_allowed_workspaces (
    id SERIAL PRIMARY KEY,
    agent_id VARCHAR(50) NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    workspace_id VARCHAR(50) NOT NULL REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    
    -- 状态
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, revoked
    
    -- 审计
    allowed_by VARCHAR(50),            -- 谁允许的(可选)
    allowed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_by VARCHAR(50),
    revoked_at TIMESTAMP,
    
    -- 时间戳
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(agent_id, workspace_id)
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_agent_allowed_ws_agent ON agent_allowed_workspaces(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_allowed_ws_workspace ON agent_allowed_workspaces(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agent_allowed_ws_status ON agent_allowed_workspaces(status);

COMMENT ON TABLE agent_allowed_workspaces IS 'Agent 允许访问的 Workspace 列表';
COMMENT ON COLUMN agent_allowed_workspaces.status IS 'active: 有效, revoked: 已撤销';

-- 第二步: 创建 Workspace 允许的 Agent 表 (Workspace 侧)
-- ============================================================================
CREATE TABLE IF NOT EXISTS workspace_allowed_agents (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    agent_id VARCHAR(50) NOT NULL REFERENCES agents(agent_id) ON DELETE CASCADE,
    
    -- 状态
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- active, revoked
    is_current BOOLEAN DEFAULT false,  -- 是否为当前使用的 agent
    
    -- 审计
    allowed_by VARCHAR(50) NOT NULL,   -- 谁允许的
    allowed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    activated_by VARCHAR(50),          -- 谁激活的
    activated_at TIMESTAMP,
    revoked_by VARCHAR(50),
    revoked_at TIMESTAMP,
    
    -- 时间戳
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(workspace_id, agent_id)
);

-- 约束: 每个 workspace 只能有一个 is_current=true 的 agent
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_one_current_agent 
ON workspace_allowed_agents(workspace_id) 
WHERE is_current = true AND status = 'active';

-- 索引
CREATE INDEX IF NOT EXISTS idx_workspace_allowed_agents_workspace ON workspace_allowed_agents(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_allowed_agents_agent ON workspace_allowed_agents(agent_id);
CREATE INDEX IF NOT EXISTS idx_workspace_allowed_agents_status ON workspace_allowed_agents(status);
CREATE INDEX IF NOT EXISTS idx_workspace_allowed_agents_current ON workspace_allowed_agents(is_current);

COMMENT ON TABLE workspace_allowed_agents IS 'Workspace 允许的 Agent 列表';
COMMENT ON COLUMN workspace_allowed_agents.is_current IS '是否为当前激活的 agent (每个 workspace 只能有一个)';

-- 第三步: 创建访问日志表 (可选,用于审计)
-- ============================================================================
CREATE TABLE IF NOT EXISTS agent_access_logs (
    id SERIAL PRIMARY KEY,
    agent_id VARCHAR(50) NOT NULL,
    workspace_id VARCHAR(50) NOT NULL,
    
    -- 访问信息
    action VARCHAR(50) NOT NULL,       -- 操作类型: task.run, task.query 等
    task_id VARCHAR(100),              -- 关联的任务 ID
    request_ip VARCHAR(50),            -- 请求 IP
    request_path TEXT,                 -- 请求路径
    
    -- 结果
    success BOOLEAN NOT NULL DEFAULT true,
    error_message TEXT,
    response_time_ms INTEGER,
    
    -- 时间戳
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_agent_access_logs_agent ON agent_access_logs(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_access_logs_workspace ON agent_access_logs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_agent_access_logs_created_at ON agent_access_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_agent_access_logs_success ON agent_access_logs(success);

COMMENT ON TABLE agent_access_logs IS 'Agent 访问日志,用于审计';

-- 第四步: 验证
-- ============================================================================
SELECT '=========================================' as info;
SELECT 'Agent 授权关系表创建完成!' as info;
SELECT '=========================================' as info;
SELECT '已创建的表:' as info;
SELECT '1. agent_allowed_workspaces - Agent 允许的 Workspace' as info;
SELECT '2. workspace_allowed_agents - Workspace 允许的 Agent' as info;
SELECT '3. agent_access_logs - 访问日志' as info;
SELECT '=========================================' as info;

-- 显示表结构
SELECT '=== agent_allowed_workspaces 表结构 ===' as info;
SELECT column_name, data_type, character_maximum_length, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'agent_allowed_workspaces' 
ORDER BY ordinal_position;

SELECT '=== workspace_allowed_agents 表结构 ===' as info;
SELECT column_name, data_type, character_maximum_length, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'workspace_allowed_agents' 
ORDER BY ordinal_position;

SELECT '=== agent_access_logs 表结构 ===' as info;
SELECT column_name, data_type, character_maximum_length, is_nullable 
FROM information_schema.columns 
WHERE table_name = 'agent_access_logs' 
ORDER BY ordinal_position;
