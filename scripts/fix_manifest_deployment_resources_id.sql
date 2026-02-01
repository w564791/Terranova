-- 修复 manifest_deployment_resources 表的 resource_id 字段类型
-- 从 integer 改为 varchar，存储语义 ID 而不是数据库主键

-- 1. 先备份现有数据
CREATE TABLE IF NOT EXISTS manifest_deployment_resources_backup AS 
SELECT * FROM manifest_deployment_resources;

-- 2. 删除外键约束（如果存在）
ALTER TABLE manifest_deployment_resources DROP CONSTRAINT IF EXISTS manifest_deployment_resources_resource_id_fkey;

-- 3. 添加新的字符串类型列
ALTER TABLE manifest_deployment_resources ADD COLUMN IF NOT EXISTS resource_semantic_id VARCHAR(255);

-- 4. 迁移数据：从 workspace_resources 获取语义 ID
UPDATE manifest_deployment_resources mdr
SET resource_semantic_id = wr.resource_id
FROM workspace_resources wr
WHERE mdr.resource_id = wr.id;

-- 5. 删除旧的整数列
ALTER TABLE manifest_deployment_resources DROP COLUMN IF EXISTS resource_id;

-- 6. 重命名新列
ALTER TABLE manifest_deployment_resources RENAME COLUMN resource_semantic_id TO resource_id;

-- 7. 添加索引
CREATE INDEX IF NOT EXISTS idx_manifest_deployment_resources_resource_id ON manifest_deployment_resources(resource_id);

-- 验证
SELECT mdr.id, mdr.deployment_id, mdr.node_id, mdr.resource_id 
FROM manifest_deployment_resources mdr 
ORDER BY mdr.created_at DESC 
LIMIT 5;
