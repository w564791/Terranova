-- IAM权限查询优化 - 添加索引
-- 创建日期: 2025-10-25
-- 目的: 优化按主体查询权限的性能

-- 组织权限表索引
CREATE INDEX IF NOT EXISTS idx_org_permissions_principal 
ON org_permissions(principal_type, principal_id);

CREATE INDEX IF NOT EXISTS idx_org_permissions_scope 
ON org_permissions(org_id);

-- 项目权限表索引
CREATE INDEX IF NOT EXISTS idx_project_permissions_principal 
ON project_permissions(principal_type, principal_id);

CREATE INDEX IF NOT EXISTS idx_project_permissions_scope 
ON project_permissions(project_id);

-- 工作空间权限表索引
CREATE INDEX IF NOT EXISTS idx_workspace_permissions_principal 
ON workspace_permissions(principal_type, principal_id);

CREATE INDEX IF NOT EXISTS idx_workspace_permissions_scope 
ON workspace_permissions(workspace_id);

-- 复合索引（可选，用于更复杂的查询）
CREATE INDEX IF NOT EXISTS idx_org_permissions_principal_scope 
ON org_permissions(principal_type, principal_id, org_id);

CREATE INDEX IF NOT EXISTS idx_project_permissions_principal_scope 
ON project_permissions(principal_type, principal_id, project_id);

CREATE INDEX IF NOT EXISTS idx_workspace_permissions_principal_scope 
ON workspace_permissions(principal_type, principal_id, workspace_id);

-- 验证索引创建
SELECT 
    schemaname,
    tablename,
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename IN ('org_permissions', 'project_permissions', 'workspace_permissions')
    AND indexname LIKE 'idx_%_principal%'
ORDER BY tablename, indexname;
