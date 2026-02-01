-- 添加 Manifest 部署漂移检测字段
-- 在 manifest_deployment_resources 表中添加 config_hash 字段，用于检测资源是否被修改

-- 添加 config_hash 字段
ALTER TABLE manifest_deployment_resources ADD COLUMN IF NOT EXISTS config_hash VARCHAR(64);

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_manifest_deployment_resources_config_hash ON manifest_deployment_resources(config_hash);

-- 更新现有记录的 config_hash（使用资源当前版本的 TF 代码 hash）
-- 这里只是添加字段，实际 hash 值需要在后端计算

-- 验证
SELECT column_name, data_type FROM information_schema.columns 
WHERE table_name = 'manifest_deployment_resources' 
ORDER BY ordinal_position;
