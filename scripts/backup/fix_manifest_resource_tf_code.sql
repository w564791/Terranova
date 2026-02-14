-- 修复 Manifest 部署创建的资源的 tf_code 格式
-- 将 { "main.tf": "HCL字符串" } 转换为 Terraform JSON 格式

-- 查看需要修复的资源
SELECT 
    wr.id as resource_id,
    wr.resource_name,
    wr.manifest_deployment_id,
    rcv.id as version_id,
    rcv.tf_code
FROM workspace_resources wr
JOIN resource_code_versions rcv ON rcv.resource_id = wr.id AND rcv.is_latest = true
WHERE wr.manifest_deployment_id IS NOT NULL;

-- 注意：由于 tf_code 格式复杂，需要手动或通过后端代码修复
-- 这里提供一个示例，实际修复需要根据具体情况处理
