-- 快速为用户分配超级管理员角色
-- 使用方法：psql -U postgres -d iac_platform -f scripts/assign_admin_role.sql

-- 显示当前所有用户
\echo '=== 当前系统用户列表 ==='
SELECT id, username, email, role, is_active 
FROM users 
ORDER BY id;

\echo ''
\echo '=== 开始为admin角色用户分配超级管理员IAM角色 ==='

-- 为所有role='admin'的用户分配超级管理员角色
DO $$
DECLARE
    admin_role_id INTEGER;
    admin_user RECORD;
    default_org_id INTEGER := 1;
    assigned_count INTEGER := 0;
BEGIN
    -- 获取admin角色ID
    SELECT id INTO admin_role_id FROM iam_roles WHERE name = 'admin';
    
    IF admin_role_id IS NULL THEN
        RAISE EXCEPTION 'Admin role not found. Please run scripts/create_iam_roles.sql first.';
    END IF;
    
    -- 为所有role='admin'的用户分配超级管理员角色
    FOR admin_user IN SELECT id, username FROM users WHERE role = 'admin' AND is_active = TRUE
    LOOP
        BEGIN
            INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id, reason)
            VALUES (
                admin_user.id, 
                admin_role_id, 
                'ORGANIZATION', 
                default_org_id, 
                '系统自动分配：原admin角色用户'
            )
            ON CONFLICT (user_id, role_id, scope_type, scope_id) DO NOTHING;
            
            assigned_count := assigned_count + 1;
            RAISE NOTICE '✓ 已为用户 % (ID: %) 分配超级管理员角色', admin_user.username, admin_user.id;
        EXCEPTION WHEN OTHERS THEN
            RAISE NOTICE '✗ 为用户 % (ID: %) 分配角色失败: %', admin_user.username, admin_user.id, SQLERRM;
        END;
    END LOOP;
    
    RAISE NOTICE '';
    RAISE NOTICE '=== 完成！共为 % 个用户分配了超级管理员角色 ===', assigned_count;
END $$;

\echo ''
\echo '=== 验证角色分配结果 ==='
SELECT 
    u.id as user_id,
    u.username,
    u.email,
    u.role as original_role,
    r.display_name as iam_role,
    ur.scope_type,
    ur.scope_id,
    ur.assigned_at
FROM iam_user_roles ur
JOIN users u ON ur.user_id = u.id
JOIN iam_roles r ON ur.role_id = r.id
WHERE r.name = 'admin'
ORDER BY u.id;

\echo ''
\echo '=== 提示 ==='
\echo '1. 以上用户现在拥有超级管理员IAM角色'
\echo '2. 他们可以通过IAM权限系统访问所有资源'
\echo '3. 建议逐步移除代码中的 role bypass 逻辑'
\echo '4. 查看详细文档：docs/iam/iam-roles-guide.md'
