-- 为 run_task_results 表添加 run_task_id 字段，用于支持全局 Run Task
-- 全局 Run Task 不在 workspace_run_tasks 表中，所以需要直接关联 run_tasks 表

-- 1. 添加 run_task_id 字段
ALTER TABLE run_task_results 
ADD COLUMN IF NOT EXISTS run_task_id VARCHAR(32);

-- 2. 添加外键约束（指向 run_tasks 表）
ALTER TABLE run_task_results 
ADD CONSTRAINT fk_run_task_results_run_task 
FOREIGN KEY (run_task_id) REFERENCES run_tasks(run_task_id) ON DELETE CASCADE;

-- 3. 修改 workspace_run_task_id 为可空（全局 Run Task 不需要这个字段）
ALTER TABLE run_task_results 
ALTER COLUMN workspace_run_task_id DROP NOT NULL;

-- 4. 添加检查约束：workspace_run_task_id 和 run_task_id 至少有一个不为空
ALTER TABLE run_task_results 
ADD CONSTRAINT chk_run_task_results_has_task 
CHECK (workspace_run_task_id IS NOT NULL OR run_task_id IS NOT NULL);

-- 5. 创建索引
CREATE INDEX IF NOT EXISTS idx_run_task_results_run_task_id ON run_task_results(run_task_id);
