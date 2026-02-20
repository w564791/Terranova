-- Run Triggers 表
-- 用于配置 workspace 之间的触发关系
-- 当源 workspace 的任务完成后，可以触发目标 workspace 的 plan+apply 任务

-- 创建 run_triggers 表
CREATE TABLE IF NOT EXISTS run_triggers (
    id SERIAL PRIMARY KEY,
    -- 源 workspace（任务完成后触发其他 workspace）
    source_workspace_id VARCHAR(50) NOT NULL REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    -- 目标 workspace（被触发的 workspace）
    target_workspace_id VARCHAR(50) NOT NULL REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    -- 是否启用
    enabled BOOLEAN DEFAULT true,
    -- 触发条件：apply_success（apply成功后触发）
    trigger_condition VARCHAR(50) DEFAULT 'apply_success',
    -- 创建时间
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- 更新时间
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- 创建者
    created_by VARCHAR(50),
    -- 唯一约束：同一对 source-target 只能有一条记录
    UNIQUE(source_workspace_id, target_workspace_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_run_triggers_source ON run_triggers(source_workspace_id);
CREATE INDEX IF NOT EXISTS idx_run_triggers_target ON run_triggers(target_workspace_id);
CREATE INDEX IF NOT EXISTS idx_run_triggers_enabled ON run_triggers(enabled);

-- 创建任务触发执行记录表
-- 记录每次任务触发的执行情况
CREATE TABLE IF NOT EXISTS task_trigger_executions (
    id SERIAL PRIMARY KEY,
    -- 源任务ID（触发任务的任务）
    source_task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    -- 触发配置ID
    run_trigger_id INTEGER NOT NULL REFERENCES run_triggers(id) ON DELETE CASCADE,
    -- 目标任务ID（被触发创建的任务，可能为空如果创建失败）
    target_task_id INTEGER REFERENCES workspace_tasks(id) ON DELETE SET NULL,
    -- 执行状态：pending, triggered, skipped, failed
    status VARCHAR(20) DEFAULT 'pending',
    -- 是否被临时禁用（用户在任务执行期间临时禁用）
    temporarily_disabled BOOLEAN DEFAULT false,
    -- 禁用/启用操作者
    disabled_by VARCHAR(50),
    -- 禁用/启用时间
    disabled_at TIMESTAMP WITH TIME ZONE,
    -- 错误信息（如果触发失败）
    error_message TEXT,
    -- 创建时间
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- 更新时间
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_task_trigger_executions_source_task ON task_trigger_executions(source_task_id);
CREATE INDEX IF NOT EXISTS idx_task_trigger_executions_target_task ON task_trigger_executions(target_task_id);
CREATE INDEX IF NOT EXISTS idx_task_trigger_executions_status ON task_trigger_executions(status);

-- 添加注释
COMMENT ON TABLE run_triggers IS 'Workspace 之间的触发关系配置';
COMMENT ON COLUMN run_triggers.source_workspace_id IS '源 workspace ID，任务完成后触发其他 workspace';
COMMENT ON COLUMN run_triggers.target_workspace_id IS '目标 workspace ID，被触发的 workspace';
COMMENT ON COLUMN run_triggers.trigger_condition IS '触发条件：apply_success 表示 apply 成功后触发';

COMMENT ON TABLE task_trigger_executions IS '任务触发执行记录';
COMMENT ON COLUMN task_trigger_executions.source_task_id IS '源任务ID，触发任务的任务';
COMMENT ON COLUMN task_trigger_executions.temporarily_disabled IS '是否被临时禁用，用户可在任务执行期间临时禁用某个触发';
