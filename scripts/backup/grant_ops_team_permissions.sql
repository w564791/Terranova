-- 为运维团队授予组织级权限
-- 团队ID: 12 (运维团队)
-- 组织ID: 1

-- 1. 授予项目管理权限 (READ级别)
INSERT INTO org_permissions (org_id, principal_type, principal_id, permission_id, permission_level, granted_by, granted_at, reason)
VALUES (1, 'TEAM', 12, 'orgpm-000000000004', 1, 1, NOW(), '为运维团队授予项目管理权限')
ON CONFLICT DO NOTHING;

-- 2. 授予工作空间管理权限 (READ级别)
INSERT INTO org_permissions (org_id, principal_type, principal_id, permission_id, permission_level, granted_by, granted_at, reason)
VALUES (1, 'TEAM', 12, 'orgpm-000000000023', 1, 1, NOW(), '为运维团队授予工作空间管理权限')
ON CONFLICT DO NOTHING;

-- 3. 授予模块管理权限 (READ级别)
INSERT INTO org_permissions (org_id, principal_type, principal_id, permission_id, permission_level, granted_by, granted_at, reason)
VALUES (1, 'TEAM', 12, 'orgpm-000000000025', 1, 1, NOW(), '为运维团队授予模块管理权限')
ON CONFLICT DO NOTHING;

-- 4. 授予组织管理权限 (READ级别)
INSERT INTO org_permissions (org_id, principal_type, principal_id, permission_id, permission_level, granted_by, granted_at, reason)
VALUES (1, 'TEAM', 12, 'orgpm-000000000002', 1, 1, NOW(), '为运维团队授予组织管理权限')
ON CONFLICT DO NOTHING;

-- 查询结果
SELECT op.*, pd.display_name as permission_name, pd.resource_type 
FROM org_permissions op 
JOIN permission_definitions pd ON op.permission_id = pd.id 
WHERE op.principal_type = 'TEAM' AND op.principal_id = 12;
