-- =====================================================
-- 为管理员用户授予完整的IAM权限
-- =====================================================
-- 
-- 用途：解决当前所有用户都能访问所有功能的安全问题
-- 执行后：只有被授予权限的用户才能访问相应功能
--
-- 使用方法：
-- psql postgresql://postgres:postgres123@localhost:5432/iac_platform -f scripts/grant_admin_permissions.sql
--
-- =====================================================

-- 1. 为所有admin角色用户授予组织级别的所有权限（ADMIN级别）
INSERT INTO org_permissions (
    org_id, 
    principal_type, 
    principal_id, 
    permission_id, 
    permission_level, 
    granted_by, 
    granted_at,
    reason
)
SELECT 
    1 as org_id,  -- 假设组织ID为1
    'USER' as principal_type,
    u.id as principal_id,
    pd.id as permission_id,
    3 as permission_level,  -- 3 = ADMIN
    1 as granted_by,  -- 假设系统管理员ID为1
    NOW() as granted_at,
    'Initial admin setup - grant all permissions to admin users' as reason
FROM users u
CROSS JOIN permission_definitions pd
WHERE u.role = 'admin'
  AND u.is_active = true
ON CONFLICT DO NOTHING;  -- 如果已存在则跳过

-- 2. 显示授予的权限数量
SELECT 
    u.username,
    u.email,
    COUNT(op.id) as granted_permissions
FROM users u
LEFT JOIN org_permissions op ON op.principal_id = u.id AND op.principal_type = 'USER'
WHERE u.role = 'admin'
GROUP BY u.id, u.username, u.email
ORDER BY u.id;

-- 3. 显示详细的权限列表
SELECT 
    u.username,
    pd.display_name as permission_name,
    pd.resource_type,
    CASE op.permission_level
        WHEN 1 THEN 'READ'
        WHEN 2 THEN 'WRITE'
        WHEN 3 THEN 'ADMIN'
        ELSE 'NONE'
    END as level
FROM users u
JOIN org_permissions op ON op.principal_id = u.id AND op.principal_type = 'USER'
JOIN permission_definitions pd ON pd.id = op.permission_id
WHERE u.role = 'admin'
ORDER BY u.username, pd.resource_type;

-- 完成提示
\echo ' 管理员权限授予完成！'
\echo '📊 请查看上方的统计信息确认权限已正确授予'
\echo ''
\echo '  下一步：重启后端服务以使权限生效'
