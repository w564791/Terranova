-- 添加Overview API所需的字段到workspaces表
-- 执行: psql -U postgres -d iac_platform -f scripts/add_overview_fields.sql

-- 添加资源统计字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS resource_count INTEGER DEFAULT 0;

-- 添加最后Plan时间
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS last_plan_at TIMESTAMP;

-- 添加最后Apply时间
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS last_apply_at TIMESTAMP;

-- 添加Drift统计字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS drift_count INTEGER DEFAULT 0;

-- 添加最后Drift检测时间
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS last_drift_check TIMESTAMP;

-- 添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_workspaces_last_plan_at ON workspaces(last_plan_at);
CREATE INDEX IF NOT EXISTS idx_workspaces_last_apply_at ON workspaces(last_apply_at);

-- 添加变更统计字段到workspace_tasks表
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS changes_add INTEGER DEFAULT 0;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS changes_change INTEGER DEFAULT 0;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS changes_destroy INTEGER DEFAULT 0;

-- 添加run_id到workspace_state_versions表
ALTER TABLE workspace_state_versions ADD COLUMN IF NOT EXISTS run_id INTEGER REFERENCES workspace_tasks(id);
ALTER TABLE workspace_state_versions ADD COLUMN IF NOT EXISTS resource_count INTEGER DEFAULT 0;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_state_versions_run_id ON workspace_state_versions(run_id);

COMMENT ON COLUMN workspaces.resource_count IS '当前管理的资源数量';
COMMENT ON COLUMN workspaces.last_plan_at IS '最后一次Plan执行时间';
COMMENT ON COLUMN workspaces.last_apply_at IS '最后一次Apply执行时间';
COMMENT ON COLUMN workspaces.drift_count IS 'Drift资源数量';
COMMENT ON COLUMN workspaces.last_drift_check IS '最后一次Drift检测时间';

COMMENT ON COLUMN workspace_tasks.changes_add IS 'Plan显示的新增资源数';
COMMENT ON COLUMN workspace_tasks.changes_change IS 'Plan显示的修改资源数';
COMMENT ON COLUMN workspace_tasks.changes_destroy IS 'Plan显示的删除资源数';

COMMENT ON COLUMN workspace_state_versions.run_id IS '关联的Run任务ID';
COMMENT ON COLUMN workspace_state_versions.resource_count IS 'State中的资源数量';
