-- Pool级别授权架构调整
-- 创建pool_allowed_workspaces表和修改workspaces表

-- 1. 创建pool_allowed_workspaces表
CREATE TABLE IF NOT EXISTS pool_allowed_workspaces (
    id SERIAL PRIMARY KEY,
    pool_id VARCHAR(50) NOT NULL REFERENCES agent_pools(pool_id) ON DELETE CASCADE,
    workspace_id VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    allowed_by VARCHAR(50),
    allowed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_by VARCHAR(50),
    revoked_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(pool_id, workspace_id)
);

CREATE INDEX idx_pool_allowed_workspaces_pool ON pool_allowed_workspaces(pool_id);
CREATE INDEX idx_pool_allowed_workspaces_workspace ON pool_allowed_workspaces(workspace_id);
CREATE INDEX idx_pool_allowed_workspaces_status ON pool_allowed_workspaces(status);

COMMENT ON TABLE pool_allowed_workspaces IS 'Pool级别的workspace准入控制';
COMMENT ON COLUMN pool_allowed_workspaces.pool_id IS 'Agent Pool ID';
COMMENT ON COLUMN pool_allowed_workspaces.workspace_id IS 'Workspace ID';
COMMENT ON COLUMN pool_allowed_workspaces.status IS '状态: active, revoked';

-- 2. 修改workspaces表,添加current_pool_id
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS current_pool_id VARCHAR(50) REFERENCES agent_pools(pool_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_current_pool ON workspaces(current_pool_id);

COMMENT ON COLUMN workspaces.current_pool_id IS 'Workspace当前使用的Agent Pool';

-- 3. 注释: 旧表保留但不再使用(可选择性删除)
-- agent_allowed_workspaces: Agent级别授权(废弃,改为Pool级别)
-- workspace_allowed_agents: Workspace选择agent(废弃,改为选择pool)

-- 如果确定不需要Agent级别授权,可以执行以下语句删除旧表:
-- DROP TABLE IF EXISTS agent_allowed_workspaces CASCADE;
-- DROP TABLE IF EXISTS workspace_allowed_agents CASCADE;

-- 完成
SELECT 'Pool级别授权表创建完成' AS status;
