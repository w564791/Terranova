-- Workspace Drift Detection 功能数据库迁移脚本
-- 功能：为 Workspace 添加后台 Terraform Drift 检测功能

-- ============================================
-- 1. workspaces 表添加 drift 检测配置字段
-- ============================================

-- 是否启用 drift 检测（默认启用）
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS drift_check_enabled BOOLEAN DEFAULT true;

-- 每天允许检测的开始时间（默认早上7点）
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS drift_check_start_time TIME DEFAULT '07:00:00';

-- 每天允许检测的结束时间（默认晚上10点）
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS drift_check_end_time TIME DEFAULT '22:00:00';

-- 检测间隔（分钟），默认每天1次（1440分钟）
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS drift_check_interval INT DEFAULT 1440;

-- 添加注释
COMMENT ON COLUMN workspaces.drift_check_enabled IS '是否启用 drift 检测';
COMMENT ON COLUMN workspaces.drift_check_start_time IS '每天允许检测的开始时间';
COMMENT ON COLUMN workspaces.drift_check_end_time IS '每天允许检测的结束时间';
COMMENT ON COLUMN workspaces.drift_check_interval IS '检测间隔（分钟）';

-- ============================================
-- 2. workspace_resources 表添加 last_applied_at 字段
-- ============================================

-- 记录资源最后一次 apply 时间
ALTER TABLE workspace_resources ADD COLUMN IF NOT EXISTS last_applied_at TIMESTAMP;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_resources_last_applied ON workspace_resources(last_applied_at);

-- 添加注释
COMMENT ON COLUMN workspace_resources.last_applied_at IS '资源最后一次成功 apply 的时间，NULL 表示从未 apply 过';

-- ============================================
-- 3. workspace_tasks 表添加 is_background 字段
-- ============================================

-- 后台任务标记（drift_check 等后台任务不显示在任务列表中）
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS is_background BOOLEAN DEFAULT false;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_is_background ON workspace_tasks(is_background);

-- 添加注释
COMMENT ON COLUMN workspace_tasks.is_background IS '是否为后台任务（drift_check 等后台任务不显示在任务列表中）';

-- ============================================
-- 4. 创建 workspace_drift_results 表
-- ============================================

CREATE TABLE IF NOT EXISTS workspace_drift_results (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL,          -- 关联的 workspace
    has_drift BOOLEAN DEFAULT false,            -- 是否存在 drift
    drift_count INT DEFAULT 0,                  -- 有 drift 的资源数量
    total_resources INT DEFAULT 0,              -- 检测的总资源数
    drift_details JSONB,                        -- 详细的 drift 信息
    check_status VARCHAR(20) DEFAULT 'pending', -- pending/running/success/failed/skipped
    error_message TEXT,                         -- 错误信息
    last_check_at TIMESTAMP,                    -- 最后检测时间
    last_check_date DATE,                       -- 最后检测日期（用于每日限制）
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- 每个 workspace 只保留一条记录
    CONSTRAINT uk_drift_results_workspace UNIQUE (workspace_id),
    
    -- 外键约束
    CONSTRAINT fk_drift_results_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE
);

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_drift_results_workspace ON workspace_drift_results(workspace_id);
CREATE INDEX IF NOT EXISTS idx_drift_results_check_date ON workspace_drift_results(last_check_date);
CREATE INDEX IF NOT EXISTS idx_drift_results_has_drift ON workspace_drift_results(has_drift);
CREATE INDEX IF NOT EXISTS idx_drift_results_check_status ON workspace_drift_results(check_status);

-- 添加表注释
COMMENT ON TABLE workspace_drift_results IS 'Workspace Drift 检测结果表，每个 workspace 只保留一条记录';
COMMENT ON COLUMN workspace_drift_results.workspace_id IS '关联的 workspace ID';
COMMENT ON COLUMN workspace_drift_results.has_drift IS '是否存在 drift';
COMMENT ON COLUMN workspace_drift_results.drift_count IS '有 drift 的资源数量';
COMMENT ON COLUMN workspace_drift_results.total_resources IS '检测的总资源数';
COMMENT ON COLUMN workspace_drift_results.drift_details IS '详细的 drift 信息（JSON格式）';
COMMENT ON COLUMN workspace_drift_results.check_status IS '检测状态：pending/running/success/failed/skipped';
COMMENT ON COLUMN workspace_drift_results.error_message IS '错误信息';
COMMENT ON COLUMN workspace_drift_results.last_check_at IS '最后检测时间';
COMMENT ON COLUMN workspace_drift_results.last_check_date IS '最后检测日期（用于每日限制）';

-- ============================================
-- 4. 验证迁移结果
-- ============================================

-- 验证 workspaces 表新增字段
SELECT column_name, data_type, column_default 
FROM information_schema.columns 
WHERE table_name = 'workspaces' 
AND column_name IN ('drift_check_enabled', 'drift_check_start_time', 'drift_check_end_time', 'drift_check_interval')
ORDER BY column_name;

-- 验证 workspace_resources 表新增字段
SELECT column_name, data_type, column_default 
FROM information_schema.columns 
WHERE table_name = 'workspace_resources' 
AND column_name = 'last_applied_at';

-- 验证 workspace_drift_results 表
SELECT column_name, data_type, column_default 
FROM information_schema.columns 
WHERE table_name = 'workspace_drift_results'
ORDER BY ordinal_position;

-- 显示成功信息
DO $$
BEGIN
    RAISE NOTICE 'Workspace Drift Detection 数据库迁移完成！';
    RAISE NOTICE '- workspaces 表添加了 4 个 drift 配置字段';
    RAISE NOTICE '- workspace_resources 表添加了 last_applied_at 字段';
    RAISE NOTICE '- 创建了 workspace_drift_results 表';
END $$;
