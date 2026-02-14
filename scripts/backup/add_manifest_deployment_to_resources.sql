-- 添加 manifest_deployment_id 字段到 workspace_resources 表
-- 用于关联资源和 Manifest 部署

ALTER TABLE workspace_resources 
ADD COLUMN IF NOT EXISTS manifest_deployment_id VARCHAR(36) DEFAULT NULL;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_resources_manifest_deployment 
ON workspace_resources(manifest_deployment_id);

-- 添加外键约束（可选，如果需要强制关联）
-- ALTER TABLE workspace_resources 
-- ADD CONSTRAINT fk_workspace_resources_manifest_deployment 
-- FOREIGN KEY (manifest_deployment_id) REFERENCES manifest_deployments(id) ON DELETE SET NULL;

COMMENT ON COLUMN workspace_resources.manifest_deployment_id IS 'Manifest 部署 ID，用于标识资源来源';
