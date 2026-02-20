-- 创建IAM系统角色
-- 执行日期: 2025-10-25
-- 说明: 创建3个IAM管理相关的系统角色

-- 1. IAM Admin - IAM管理员角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active, created_at, updated_at)
VALUES 
    ('iam_admin', 'IAM管理员', '拥有所有IAM管理权限，可以管理用户、角色、权限、组织等', true, true, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;

-- 2. IAM Write - IAM写入角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active, created_at, updated_at)
VALUES 
    ('iam_write', 'IAM编辑者', '可以创建和修改IAM资源，但不能删除', true, true, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;

-- 3. IAM Read - IAM只读角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active, created_at, updated_at)
VALUES 
    ('iam_read', 'IAM查看者', '只能查看IAM相关信息，不能修改', true, true, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;

-- 获取角色ID
DO $$
DECLARE
    iam_admin_id INT;
    iam_write_id INT;
    iam_read_id INT;
BEGIN
    SELECT id INTO iam_admin_id FROM iam_roles WHERE name = 'iam_admin';
    SELECT id INTO iam_write_id FROM iam_roles WHERE name = 'iam_write';
    SELECT id INTO iam_read_id FROM iam_roles WHERE name = 'iam_read';

    -- 为IAM Admin角色添加所有IAM权限（ADMIN级别）
    INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at) VALUES
    (iam_admin_id, 'orgpm-iam-permissions', 'ADMIN', 'ORGANIZATION', NOW()),
    (iam_admin_id, 'orgpm-iam-teams', 'ADMIN', 'ORGANIZATION', NOW()),
    (iam_admin_id, 'orgpm-iam-organizations', 'ADMIN', 'ORGANIZATION', NOW()),
    (iam_admin_id, 'orgpm-iam-projects', 'ADMIN', 'ORGANIZATION', NOW()),
    (iam_admin_id, 'orgpm-iam-applications', 'ADMIN', 'ORGANIZATION', NOW()),
    (iam_admin_id, 'orgpm-iam-audit', 'ADMIN', 'ORGANIZATION', NOW()),
    (iam_admin_id, 'orgpm-iam-users', 'ADMIN', 'ORGANIZATION', NOW()),
    (iam_admin_id, 'orgpm-iam-roles', 'ADMIN', 'ORGANIZATION', NOW())
    ON CONFLICT DO NOTHING;

    -- 为IAM Write角色添加所有IAM权限（WRITE级别）
    INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at) VALUES
    (iam_write_id, 'orgpm-iam-permissions', 'WRITE', 'ORGANIZATION', NOW()),
    (iam_write_id, 'orgpm-iam-teams', 'WRITE', 'ORGANIZATION', NOW()),
    (iam_write_id, 'orgpm-iam-organizations', 'WRITE', 'ORGANIZATION', NOW()),
    (iam_write_id, 'orgpm-iam-projects', 'WRITE', 'ORGANIZATION', NOW()),
    (iam_write_id, 'orgpm-iam-applications', 'WRITE', 'ORGANIZATION', NOW()),
    (iam_write_id, 'orgpm-iam-audit', 'READ', 'ORGANIZATION', NOW()),
    (iam_write_id, 'orgpm-iam-users', 'WRITE', 'ORGANIZATION', NOW()),
    (iam_write_id, 'orgpm-iam-roles', 'WRITE', 'ORGANIZATION', NOW())
    ON CONFLICT DO NOTHING;

    -- 为IAM Read角色添加所有IAM权限（READ级别）
    INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at) VALUES
    (iam_read_id, 'orgpm-iam-permissions', 'READ', 'ORGANIZATION', NOW()),
    (iam_read_id, 'orgpm-iam-teams', 'READ', 'ORGANIZATION', NOW()),
    (iam_read_id, 'orgpm-iam-organizations', 'READ', 'ORGANIZATION', NOW()),
    (iam_read_id, 'orgpm-iam-projects', 'READ', 'ORGANIZATION', NOW()),
    (iam_read_id, 'orgpm-iam-applications', 'READ', 'ORGANIZATION', NOW()),
    (iam_read_id, 'orgpm-iam-audit', 'READ', 'ORGANIZATION', NOW()),
    (iam_read_id, 'orgpm-iam-users', 'READ', 'ORGANIZATION', NOW()),
    (iam_read_id, 'orgpm-iam-roles', 'READ', 'ORGANIZATION', NOW())
    ON CONFLICT DO NOTHING;
END $$;

-- 验证结果
SELECT 
    r.name as role_name,
    r.display_name,
    COUNT(rp.id) as policy_count
FROM iam_roles r
LEFT JOIN iam_role_policies rp ON r.id = rp.role_id
WHERE r.name IN ('iam_admin', 'iam_write', 'iam_read')
GROUP BY r.id, r.name, r.display_name
ORDER BY r.name;
