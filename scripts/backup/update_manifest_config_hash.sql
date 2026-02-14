-- 更新现有 manifest_deployment_resources 的 config_hash
-- 使用资源当前版本的 TF 代码计算 hash

-- 查看需要更新的记录
SELECT 
    mdr.id,
    mdr.resource_id,
    mdr.config_hash,
    wr.resource_name,
    rcv.id as version_id
FROM manifest_deployment_resources mdr
JOIN manifest_deployments md ON mdr.deployment_id = md.id
JOIN workspaces w ON md.workspace_id = w.id
JOIN workspace_resources wr ON wr.workspace_id = w.workspace_id AND wr.resource_id = mdr.resource_id
LEFT JOIN resource_code_versions rcv ON rcv.id = wr.current_version_id
WHERE mdr.config_hash IS NULL;

-- 由于 PostgreSQL 没有内置的 SHA256 函数，我们需要在后端计算
-- 这里只是标记需要更新的记录
