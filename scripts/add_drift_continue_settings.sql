-- 添加 drift 检测继续设置字段到 workspace_drift_results 表
-- 这些设置对当天有效，默认都是不继续

-- 添加失败后继续字段
ALTER TABLE workspace_drift_results 
ADD COLUMN IF NOT EXISTS continue_on_failure BOOLEAN DEFAULT FALSE;

-- 添加成功后继续字段
ALTER TABLE workspace_drift_results 
ADD COLUMN IF NOT EXISTS continue_on_success BOOLEAN DEFAULT FALSE;

-- 添加注释
COMMENT ON COLUMN workspace_drift_results.continue_on_failure IS '失败后继续检测（当天有效，每天重置为 false）';
COMMENT ON COLUMN workspace_drift_results.continue_on_success IS '成功后继续检测（当天有效，每天重置为 false）';
