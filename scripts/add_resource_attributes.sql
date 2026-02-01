-- 添加资源属性字段到 workspace_task_resource_changes 表
-- 用于存储Apply完成后从state提取的资源详情

ALTER TABLE workspace_task_resource_changes 
ADD COLUMN IF NOT EXISTS resource_id VARCHAR(500),
ADD COLUMN IF NOT EXISTS resource_attributes JSONB;

-- 添加注释
COMMENT ON COLUMN workspace_task_resource_changes.resource_id IS '资源ID（从terraform state提取，如AWS资源的ID）';
COMMENT ON COLUMN workspace_task_resource_changes.resource_attributes IS '资源属性（从terraform state提取，如ARN等）';

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_task_resource_changes_resource_id 
ON workspace_task_resource_changes(resource_id);
