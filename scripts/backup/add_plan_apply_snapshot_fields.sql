-- 添加Plan-Apply快照字段到workspace_tasks表
-- 用于修复Plan-Apply竞态条件bug

-- 添加快照字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS snapshot_resource_versions JSONB;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS snapshot_variables JSONB;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS snapshot_provider_config JSONB;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS snapshot_created_at TIMESTAMP;

-- 添加注释
COMMENT ON COLUMN workspace_tasks.snapshot_resource_versions IS 'Plan阶段的资源版本快照，格式: {"resource_id": {"version_id": 123, "version": 5}}';
COMMENT ON COLUMN workspace_tasks.snapshot_variables IS 'Plan阶段的变量完整快照（变量不支持版本控制，需要保存完整数据）';
COMMENT ON COLUMN workspace_tasks.snapshot_provider_config IS 'Plan阶段的Provider配置快照';
COMMENT ON COLUMN workspace_tasks.snapshot_created_at IS '快照创建时间';

-- 创建索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_snapshot_created_at ON workspace_tasks(snapshot_created_at) WHERE snapshot_created_at IS NOT NULL;
