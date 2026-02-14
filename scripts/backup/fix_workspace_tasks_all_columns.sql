-- 修复workspace_tasks表所有缺失的列
-- 创建日期: 2025-10-11

-- 添加所有可能缺失的列
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS k8s_config_id INTEGER;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS execution_node VARCHAR(255) DEFAULT '';
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_task_id INTEGER;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_data BYTEA;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_json JSONB;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS outputs JSONB;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS stage VARCHAR(30) DEFAULT 'pending';
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS context JSONB;

-- 验证所有列
SELECT column_name, data_type, column_default
FROM information_schema.columns 
WHERE table_name = 'workspace_tasks' 
AND column_name IN (
  'k8s_config_id', 
  'execution_node', 
  'plan_task_id', 
  'plan_data', 
  'plan_json', 
  'outputs', 
  'stage', 
  'context'
)
ORDER BY column_name;

SELECT ' workspace_tasks表所有列已添加' AS status;

-- 添加Agent相关的列
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS locked_by VARCHAR(255) DEFAULT '';
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS locked_at TIMESTAMP;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS lock_expires_at TIMESTAMP;

-- 再次验证
SELECT ' 所有列已添加完成' AS final_status;
