-- Plan+Apply Flow Redesign Migration
-- 添加新字段支持Plan+Apply单任务流程

-- 1. 添加新字段
ALTER TABLE workspace_tasks 
ADD COLUMN IF NOT EXISTS snapshot_id VARCHAR(64),
ADD COLUMN IF NOT EXISTS apply_description TEXT;

-- 2. 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_snapshot_id 
ON workspace_tasks(snapshot_id);

-- 3. 添加注释
COMMENT ON COLUMN workspace_tasks.snapshot_id IS '资源版本快照ID（Plan完成时创建）';
COMMENT ON COLUMN workspace_tasks.apply_description IS 'Apply描述（用户确认Apply时输入）';

-- 4. 验证
SELECT 
    column_name, 
    data_type, 
    character_maximum_length,
    is_nullable
FROM information_schema.columns 
WHERE table_name = 'workspace_tasks' 
AND column_name IN ('snapshot_id', 'apply_description')
ORDER BY column_name;
