-- Workspace模块增强 - 数据库迁移脚本
-- 版本: v1.0
-- 日期: 2025-10-09
-- 说明: 为workspaces表添加新字段以支持完整的workspace功能

BEGIN;

-- 1. 备份现有数据
CREATE TABLE IF NOT EXISTS workspaces_backup_20251009 AS SELECT * FROM workspaces;

-- 2. 添加执行模式相关字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS execution_mode VARCHAR(20) DEFAULT 'local';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS agent_id INTEGER REFERENCES agents(id);

-- 3. 添加Apply方法字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS auto_apply BOOLEAN DEFAULT false;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS plan_only BOOLEAN DEFAULT false;

-- 4. 添加Terraform配置字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS workdir VARCHAR(500) DEFAULT '/workspace';

-- 5. 添加锁定状态字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT false;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS locked_by INTEGER REFERENCES users(id);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS locked_at TIMESTAMP;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS lock_reason TEXT;

-- 6. 添加文件存储字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tf_code JSONB;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tf_state JSONB;

-- 7. 添加Provider配置字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS provider_config JSONB;

-- 8. 添加初始化配置字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS init_config JSONB;

-- 9. 添加重试配置字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS retry_enabled BOOLEAN DEFAULT true;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS max_retries INTEGER DEFAULT 3;

-- 10. 添加通知和日志配置字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS notify_settings JSONB;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS log_config JSONB;

-- 11. 添加生命周期状态字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS state VARCHAR(20) DEFAULT 'created';

-- 12. 添加Tags字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS tags JSONB;

-- 13. 添加系统变量字段
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS system_variables JSONB;

-- 14. 创建索引
CREATE INDEX IF NOT EXISTS idx_workspaces_execution_mode ON workspaces(execution_mode);
CREATE INDEX IF NOT EXISTS idx_workspaces_agent_id ON workspaces(agent_id);
CREATE INDEX IF NOT EXISTS idx_workspaces_is_locked ON workspaces(is_locked);
CREATE INDEX IF NOT EXISTS idx_workspaces_locked_by ON workspaces(locked_by);
CREATE INDEX IF NOT EXISTS idx_workspaces_state ON workspaces(state);
CREATE INDEX IF NOT EXISTS idx_workspaces_tf_code_gin ON workspaces USING GIN(tf_code);
CREATE INDEX IF NOT EXISTS idx_workspaces_tf_state_gin ON workspaces USING GIN(tf_state);
CREATE INDEX IF NOT EXISTS idx_workspaces_provider_config_gin ON workspaces USING GIN(provider_config);
CREATE INDEX IF NOT EXISTS idx_workspaces_init_config_gin ON workspaces USING GIN(init_config);
CREATE INDEX IF NOT EXISTS idx_workspaces_tags_gin ON workspaces USING GIN(tags);

-- 15. 创建workspace_tasks表
CREATE TABLE IF NOT EXISTS workspace_tasks (
    -- 基础字段
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 任务类型
    task_type VARCHAR(20) NOT NULL, -- plan, apply
    
    -- 任务状态
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, running, success, failed, cancelled
    
    -- 执行信息
    execution_mode VARCHAR(20) NOT NULL, -- local, agent, k8s
    agent_id INTEGER REFERENCES agents(id),
    k8s_pod_name VARCHAR(100),
    k8s_namespace VARCHAR(100) DEFAULT 'iac-platform',
    
    -- Terraform输出
    plan_output TEXT,
    apply_output TEXT,
    error_message TEXT,
    
    -- 执行时间
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    duration INTEGER, -- 执行时长（秒）
    
    -- 重试信息
    retry_count INTEGER DEFAULT 0,
    max_retries INTEGER DEFAULT 3,
    
    -- 元数据
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 16. 创建workspace_tasks表索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_workspace_id ON workspace_tasks(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_task_type ON workspace_tasks(task_type);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_status ON workspace_tasks(status);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_execution_mode ON workspace_tasks(execution_mode);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_agent_id ON workspace_tasks(agent_id);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_created_by ON workspace_tasks(created_by);
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_created_at ON workspace_tasks(created_at);

-- 17. 创建workspace_state_versions表
CREATE TABLE IF NOT EXISTS workspace_state_versions (
    -- 基础字段
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id) ON DELETE CASCADE,
    
    -- 文件内容
    content JSONB NOT NULL, -- State文件内容
    
    -- 版本信息
    version INTEGER NOT NULL, -- 版本号，从1开始递增
    checksum VARCHAR(64) NOT NULL, -- SHA256校验和
    size_bytes INTEGER, -- 文件大小（字节）
    
    -- 关联任务
    task_id INTEGER REFERENCES workspace_tasks(id),
    
    -- 元数据
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(workspace_id, version)
);

-- 18. 创建workspace_state_versions表索引
CREATE INDEX IF NOT EXISTS idx_workspace_state_versions_workspace_id ON workspace_state_versions(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_state_versions_version ON workspace_state_versions(version);
CREATE INDEX IF NOT EXISTS idx_workspace_state_versions_task_id ON workspace_state_versions(task_id);
CREATE INDEX IF NOT EXISTS idx_workspace_state_versions_created_at ON workspace_state_versions(created_at);

-- 19. 添加注释
COMMENT ON COLUMN workspaces.execution_mode IS '执行模式: local, agent, k8s';
COMMENT ON COLUMN workspaces.auto_apply IS '是否自动Apply';
COMMENT ON COLUMN workspaces.plan_only IS '是否仅执行Plan';
COMMENT ON COLUMN workspaces.is_locked IS '是否锁定';
COMMENT ON COLUMN workspaces.state IS '生命周期状态: created, planning, plan_done, waiting_apply, applying, completed, failed';
COMMENT ON COLUMN workspaces.tags IS '用户自定义标签（JSON数组）';
COMMENT ON COLUMN workspaces.system_variables IS '系统变量（JSON对象）';

COMMENT ON TABLE workspace_tasks IS 'Workspace任务表';
COMMENT ON COLUMN workspace_tasks.task_type IS '任务类型: plan, apply';
COMMENT ON COLUMN workspace_tasks.status IS '任务状态: pending, running, success, failed, cancelled';

COMMENT ON TABLE workspace_state_versions IS 'Workspace State版本控制表';
COMMENT ON COLUMN workspace_state_versions.version IS 'State版本号，从1开始递增';
COMMENT ON COLUMN workspace_state_versions.checksum IS 'State文件SHA256校验和';

COMMIT;

-- 验证迁移
SELECT 
    'workspaces表字段数' as check_item,
    COUNT(*) as count
FROM information_schema.columns 
WHERE table_name = 'workspaces';

SELECT 
    'workspace_tasks表是否存在' as check_item,
    CASE WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'workspace_tasks') 
        THEN 'YES' ELSE 'NO' END as result;

SELECT 
    'workspace_state_versions表是否存在' as check_item,
    CASE WHEN EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'workspace_state_versions') 
        THEN 'YES' ELSE 'NO' END as result;
