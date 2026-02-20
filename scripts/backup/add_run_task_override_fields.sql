-- 添加 Run Task Override 相关字段
-- 用于记录 Advisory Run Task 失败后的 Override 操作

ALTER TABLE run_task_results 
ADD COLUMN IF NOT EXISTS is_overridden BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS override_by VARCHAR(255),
ADD COLUMN IF NOT EXISTS override_at TIMESTAMP;

-- 添加注释
COMMENT ON COLUMN run_task_results.is_overridden IS '是否已被 Override（仅对 Advisory 类型有效）';
COMMENT ON COLUMN run_task_results.override_by IS 'Override 操作的用户ID';
COMMENT ON COLUMN run_task_results.override_at IS 'Override 操作的时间';

-- 更新 status 字段的 CHECK 约束，添加 'overridden' 状态
-- 先删除旧约束（如果存在）
ALTER TABLE run_task_results DROP CONSTRAINT IF EXISTS run_task_results_status_check;

-- 添加新约束，包含 'overridden' 状态
ALTER TABLE run_task_results ADD CONSTRAINT run_task_results_status_check 
  CHECK (status IN ('pending', 'running', 'passed', 'failed', 'error', 'timeout', 'skipped', 'overridden'));
