-- 添加workspace_tasks表的description字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS description TEXT;

-- 添加注释
COMMENT ON COLUMN workspace_tasks.description IS '任务描述，用于标识和说明任务目的';
