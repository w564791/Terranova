-- 添加workspace表缺失的字段
-- 执行前请备份数据库

-- 添加k8s_config_id字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS k8s_config_id INTEGER;

-- 添加auto_apply字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS auto_apply BOOLEAN DEFAULT false;

-- 添加plan_only字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS plan_only BOOLEAN DEFAULT false;

-- 添加workdir字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS workdir VARCHAR(500) DEFAULT '/workspace';

-- 添加tags字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tags JSONB;

-- 添加variables字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS variables JSONB;

-- 添加system_variables字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS system_variables JSONB;

-- 添加provider_config字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS provider_config JSONB;

-- 添加init_config字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS init_config JSONB;

-- 添加notify_settings字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS notify_settings JSONB;

-- 添加log_config字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS log_config JSONB;

-- 添加state字段（生命周期状态）
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS state VARCHAR(20) DEFAULT 'created';

-- 添加is_locked字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT false;

-- 添加locked_by字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS locked_by INTEGER REFERENCES users(id);

-- 添加locked_at字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS locked_at TIMESTAMP;

-- 添加lock_reason字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS lock_reason TEXT;

-- 添加tf_code字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tf_code JSONB;

-- 添加tf_state字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tf_state JSONB;

-- 添加retry_enabled字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS retry_enabled BOOLEAN DEFAULT true;

-- 添加max_retries字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS max_retries INTEGER DEFAULT 3;

-- 添加overview统计字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS resource_count INTEGER DEFAULT 0;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS last_plan_at TIMESTAMP;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS last_apply_at TIMESTAMP;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS drift_count INTEGER DEFAULT 0;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS last_drift_check TIMESTAMP;

-- 修改execution_mode字段，将'server'改为'local'
UPDATE workspaces SET execution_mode = 'local' WHERE execution_mode = 'server';

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspaces_k8s_config ON workspaces(k8s_config_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_state ON workspaces(state);
CREATE INDEX IF NOT EXISTS idx_workspaces_locked ON workspaces(is_locked);
CREATE INDEX IF NOT EXISTS idx_workspaces_last_plan ON workspaces(last_plan_at);
CREATE INDEX IF NOT EXISTS idx_workspaces_last_apply ON workspaces(last_apply_at);
CREATE INDEX IF NOT EXISTS idx_workspaces_tags_gin ON workspaces USING GIN(tags);

-- 验证迁移
SELECT 
    column_name, 
    data_type, 
    is_nullable, 
    column_default
FROM information_schema.columns 
WHERE table_name = 'workspaces' 
ORDER BY ordinal_position;
