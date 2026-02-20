-- 添加 current_task_id 字段到 workspace_drift_results 表
-- 用于关联当前正在执行的 drift_check 任务

ALTER TABLE workspace_drift_results ADD COLUMN IF NOT EXISTS current_task_id INTEGER;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_drift_results_current_task ON workspace_drift_results(current_task_id);

-- 清理历史遗留的 running 状态（没有关联任务的）
UPDATE workspace_drift_results 
SET check_status = 'failed', 
    error_message = 'Task lost - cleaned up on migration'
WHERE check_status IN ('pending', 'running') 
  AND current_task_id IS NULL;
