-- 添加Slot相关字段以支持Plan-Apply工作目录复用优化
-- 这些字段用于记录Plan执行时的Pod和Slot信息,以便Apply时判断是否可以跳过Init

-- 添加warmup_pod_name字段(记录执行Plan的Pod名称)
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_pod_name VARCHAR(100);

-- 添加warmup_slot_id字段(记录执行Plan的Slot ID)
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_slot_id INTEGER;

-- 添加索引以优化查询性能(查找特定Pod上的apply_pending任务)
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_warmup_pod 
ON workspace_tasks(warmup_pod_name) 
WHERE warmup_pod_name IS NOT NULL;

-- 添加注释
COMMENT ON COLUMN workspace_tasks.warmup_pod_name IS 'Pod name where plan was executed (for workspace reuse optimization)';
COMMENT ON COLUMN workspace_tasks.warmup_slot_id IS 'Slot ID where plan was executed (for workspace reuse optimization)';
