-- 恢复 0.0.10 版本的部署用于测试卸载功能

-- 1. 将部署状态改回 deployed
UPDATE manifest_deployments 
SET status = 'deployed' 
WHERE id = 'mfd-cbeb07bc249bfm4b';

-- 2. 更新资源关联到这个部署
UPDATE workspace_resources 
SET manifest_deployment_id = 'mfd-cbeb07bc249bfm4b'
WHERE workspace_id = 'ws-5o7movp0e7' 
  AND resource_id IN ('module.ec2-ff-012', 'module.actions-41');

-- 3. 重新创建部署资源关联记录
INSERT INTO manifest_deployment_resources (id, deployment_id, node_id, resource_id, config_hash, created_at)
VALUES 
  ('mdr-test-ec2-ff-012', 'mfd-cbeb07bc249bfm4b', 'node-1767235769736', 'module.ec2-ff-012', '', NOW()),
  ('mdr-test-actions-41', 'mfd-cbeb07bc249bfm4b', 'node-1767236256330', 'module.actions-41', '', NOW())
ON CONFLICT (id) DO UPDATE SET 
  deployment_id = EXCLUDED.deployment_id,
  resource_id = EXCLUDED.resource_id;

-- 4. 验证结果
SELECT 'Deployment status:' as info, id, status FROM manifest_deployments WHERE id = 'mfd-cbeb07bc249bfm4b';
SELECT 'Resources:' as info, id, resource_id, resource_name, is_active, manifest_deployment_id 
FROM workspace_resources 
WHERE workspace_id = 'ws-5o7movp0e7' AND resource_id IN ('module.ec2-ff-012', 'module.actions-41');
SELECT 'Deployment resources:' as info, * FROM manifest_deployment_resources WHERE deployment_id = 'mfd-cbeb07bc249bfm4b';
