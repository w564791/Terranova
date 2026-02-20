-- ============================================================================
-- Agent-Workspace 双向授权系统数据库表
-- 基于 docs/iam/agent-workspace-authorization-final.md 设计
-- ============================================================================

-- 第一步: 创建 Agents 表
-- ============================================================================
CREATE TABLE IF NOT EXISTS agents (
    agent_id VARCHAR(50) PRIMARY KEY,  -- 格式: agent-{16位随机小写字母+数字}
    application_id INTEGER NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    
    -- Agent 信息
    name VARCHAR(100),                 -- Agent 名称(可重复,用于标识)
    ip_address VARCHAR(50),            -- IP 地址
    version VARCHAR(50),               -- Agent 版本
    
    -- 状态
    status VARCHAR(20) NOT NULL DEFAULT 'idle', -- idle, busy, offline
    last_ping_at TIMESTAMP,            -- 最后心跳时间
    
    -- 时间戳
    registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX IF NOT EXISTS idx_agents_application ON agents(application_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_last_ping ON agents(last_ping_at);

COMMENT ON TABLE agents IS 'Agent 运行实例表';
COMMENT ON COLUMN agents.agent_id IS '语义化 ID: agent-{16位随机}';
COMMENT ON COLUMN agents.status IS 'idle: 空闲, busy: 忙碌, offline: 离线';

-- 第二步: 创建 Agent 允许的 Workspace 表 (Agent 侧)
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

-- 第三步: 创建 Workspace 允许的 Agent 表 (Workspace 侧)
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

-- 第四步: 创建访问日志表 (可选,用于审计)
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

-- 第五步: 验证
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '===========================================';
    RAISE NOTICE 'Agent 授权系统表创建完成!';
    RAISE NOTICE '===========================================';
    RAISE NOTICE '已创建的表:';
    RAISE NOTICE '1. agents - Agent 实例表';
    RAISE NOTICE '2. agent_allowed_workspaces - Agent 允许的 Workspace';
    RAISE NOTICE '3. workspace_allowed_agents - Workspace 允许的 Agent';
    RAISE NOTICE '4. agent_access_logs - 访问日志';
    RAISE NOTICE '===========================================';
    RAISE NOTICE '下一步:';
    RAISE NOTICE '1. 创建 Agent 相关的 Go 模型';
    RAISE NOTICE '2. 实现 Agent 注册 API';
    RAISE NOTICE '3. 实现双向授权 API';
    RAISE NOTICE '4. 实现验证中间件';
    RAISE NOTICE '===========================================';
END $$;
