-- 添加 UI 模式字段到 workspaces 表
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS ui_mode VARCHAR(20) DEFAULT 'console';
COMMENT ON COLUMN workspaces.ui_mode IS 'UI display mode: console or structured';

-- 创建资源变更表
CREATE TABLE IF NOT EXISTS workspace_task_resource_changes (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 资源标识
    resource_address VARCHAR(500) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_name VARCHAR(200) NOT NULL,
    module_address VARCHAR(500),
    
    -- 变更信息
    action VARCHAR(20) NOT NULL,
    changes_before JSONB,
    changes_after JSONB,
    
    -- Apply 阶段状态
    apply_status VARCHAR(20) DEFAULT 'pending',
    apply_started_at TIMESTAMP,
    apply_completed_at TIMESTAMP,
    apply_error TEXT,
    
    -- 元数据
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_task_resource_changes_task_id ON workspace_task_resource_changes(task_id);
CREATE INDEX IF NOT EXISTS idx_workspace_task_resource_changes_workspace_id ON workspace_task_resource_changes(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_task_resource_changes_status ON workspace_task_resource_changes(task_id, apply_status);
CREATE INDEX IF NOT EXISTS idx_workspace_task_resource_changes_action ON workspace_task_resource_changes(action);

-- 添加注释
COMMENT ON TABLE workspace_task_resource_changes IS 'Terraform资源变更记录表，用于Structured Run Output模式';
COMMENT ON COLUMN workspace_task_resource_changes.resource_address IS '资源完整地址';
COMMENT ON COLUMN workspace_task_resource_changes.action IS '变更操作类型: create/update/delete/replace';
COMMENT ON COLUMN workspace_task_resource_changes.changes_before IS '变更前的完整数据';
COMMENT ON COLUMN workspace_task_resource_changes.changes_after IS '变更后的完整数据';
COMMENT ON COLUMN workspace_task_resource_changes.apply_status IS 'Apply阶段状态: pending/applying/completed/failed';
