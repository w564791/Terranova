-- IAM Role System
-- 类似AWS IAM Role的概念，一个Role包含多个权限策略

-- 1. 创建 iam_roles 表
CREATE TABLE IF NOT EXISTS iam_roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(200) NOT NULL,
    description TEXT,
    is_system BOOLEAN DEFAULT FALSE, -- 系统预定义的角色不可删除
    is_active BOOLEAN DEFAULT TRUE,
    created_by INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 2. 创建 iam_role_policies 表（Role包含的权限策略）
CREATE TABLE IF NOT EXISTS iam_role_policies (
    id SERIAL PRIMARY KEY,
    role_id INTEGER NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permission_definitions(id) ON DELETE CASCADE,
    permission_level VARCHAR(20) NOT NULL CHECK (permission_level IN ('NONE', 'READ', 'WRITE', 'ADMIN')),
    scope_type VARCHAR(20) NOT NULL CHECK (scope_type IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(role_id, permission_id, scope_type)
);

-- 3. 创建 iam_user_roles 表（用户分配的角色）
CREATE TABLE IF NOT EXISTS iam_user_roles (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    role_id INTEGER NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
    scope_type VARCHAR(20) NOT NULL CHECK (scope_type IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')),
    scope_id INTEGER NOT NULL,
    assigned_by INTEGER,
    assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    reason TEXT,
    UNIQUE(user_id, role_id, scope_type, scope_id)
);

-- 4. 创建索引
CREATE INDEX IF NOT EXISTS idx_role_policies_role_id ON iam_role_policies(role_id);
CREATE INDEX IF NOT EXISTS idx_role_policies_permission_id ON iam_role_policies(permission_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON iam_user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON iam_user_roles(role_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_scope ON iam_user_roles(scope_type, scope_id);

-- 5. 创建系统预定义角色

-- 5.1 超级管理员角色（完全权限）
INSERT INTO iam_roles (name, display_name, description, is_system, is_active)
VALUES ('admin', '超级管理员', '拥有系统所有权限的超级管理员角色', TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- 5.2 组织管理员角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active)
VALUES ('org_admin', '组织管理员', '组织级别的管理员，可以管理组织内的所有资源', TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- 5.3 项目管理员角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active)
VALUES ('project_admin', '项目管理员', '项目级别的管理员，可以管理项目内的所有资源', TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- 5.4 工作空间管理员角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active)
VALUES ('workspace_admin', '工作空间管理员', '工作空间级别的管理员，可以完全管理工作空间', TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- 5.5 开发者角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active)
VALUES ('developer', '开发者', '可以创建和管理工作空间、执行任务的开发者角色', TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- 5.6 只读用户角色
INSERT INTO iam_roles (name, display_name, description, is_system, is_active)
VALUES ('viewer', '查看者', '只能查看资源，不能进行任何修改操作', TRUE, TRUE)
ON CONFLICT (name) DO NOTHING;

-- 6. 为超级管理员角色添加所有权限（ADMIN级别）

-- 获取admin角色ID
DO $$
DECLARE
    admin_role_id INTEGER;
    perm RECORD;
BEGIN
    SELECT id INTO admin_role_id FROM iam_roles WHERE name = 'admin';
    
    -- 为所有权限定义添加ADMIN级别的策略
    FOR perm IN SELECT id FROM permission_definitions
    LOOP
        -- 组织级别
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (admin_role_id, perm.id, 'ADMIN', 'ORGANIZATION')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
        
        -- 项目级别
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (admin_role_id, perm.id, 'ADMIN', 'PROJECT')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
        
        -- 工作空间级别
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (admin_role_id, perm.id, 'ADMIN', 'WORKSPACE')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
    END LOOP;
END $$;

-- 7. 为组织管理员角色添加组织级权限

DO $$
DECLARE
    org_admin_role_id INTEGER;
    perm RECORD;
BEGIN
    SELECT id INTO org_admin_role_id FROM iam_roles WHERE name = 'org_admin';
    
    -- 组织相关权限：ADMIN级别
    FOR perm IN 
        SELECT id FROM permission_definitions 
        WHERE resource_type IN ('ORGANIZATION', 'PROJECTS', 'WORKSPACES', 'MODULES')
    LOOP
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (org_admin_role_id, perm.id, 'ADMIN', 'ORGANIZATION')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
    END LOOP;
END $$;

-- 8. 为项目管理员角色添加项目级权限

DO $$
DECLARE
    project_admin_role_id INTEGER;
    perm RECORD;
BEGIN
    SELECT id INTO project_admin_role_id FROM iam_roles WHERE name = 'project_admin';
    
    -- 项目相关权限：ADMIN级别
    FOR perm IN 
        SELECT id FROM permission_definitions 
        WHERE resource_type IN ('WORKSPACES')
    LOOP
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (project_admin_role_id, perm.id, 'ADMIN', 'PROJECT')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
    END LOOP;
END $$;

-- 9. 为工作空间管理员角色添加工作空间级权限

DO $$
DECLARE
    workspace_admin_role_id INTEGER;
    perm RECORD;
BEGIN
    SELECT id INTO workspace_admin_role_id FROM iam_roles WHERE name = 'workspace_admin';
    
    -- 工作空间相关权限：ADMIN级别
    FOR perm IN 
        SELECT id FROM permission_definitions 
        WHERE resource_type IN (
            'WORKSPACE_MANAGEMENT',
            'WORKSPACE_EXECUTION', 
            'WORKSPACE_STATE', 
            'WORKSPACE_VARIABLES', 
            'WORKSPACE_RESOURCES',
            'TASK_DATA_ACCESS'
        )
    LOOP
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (workspace_admin_role_id, perm.id, 'ADMIN', 'WORKSPACE')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
    END LOOP;
END $$;

-- 10. 为开发者角色添加工作空间级权限（WRITE级别）

DO $$
DECLARE
    developer_role_id INTEGER;
    perm RECORD;
BEGIN
    SELECT id INTO developer_role_id FROM iam_roles WHERE name = 'developer';
    
    -- 工作空间相关权限：WRITE级别
    FOR perm IN 
        SELECT id FROM permission_definitions 
        WHERE resource_type IN (
            'WORKSPACE_MANAGEMENT',
            'WORKSPACE_EXECUTION', 
            'WORKSPACE_VARIABLES', 
            'WORKSPACE_RESOURCES'
        )
    LOOP
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (developer_role_id, perm.id, 'WRITE', 'WORKSPACE')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
    END LOOP;
    
    -- State和数据访问：READ级别
    FOR perm IN 
        SELECT id FROM permission_definitions 
        WHERE resource_type IN ('WORKSPACE_STATE', 'TASK_DATA_ACCESS')
    LOOP
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (developer_role_id, perm.id, 'READ', 'WORKSPACE')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
    END LOOP;
END $$;

-- 11. 为查看者角色添加只读权限

DO $$
DECLARE
    viewer_role_id INTEGER;
    perm RECORD;
BEGIN
    SELECT id INTO viewer_role_id FROM iam_roles WHERE name = 'viewer';
    
    -- 所有权限：READ级别
    FOR perm IN SELECT id FROM permission_definitions
    LOOP
        -- 组织级别
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (viewer_role_id, perm.id, 'READ', 'ORGANIZATION')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
        
        -- 项目级别
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (viewer_role_id, perm.id, 'READ', 'PROJECT')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
        
        -- 工作空间级别
        INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type)
        VALUES (viewer_role_id, perm.id, 'READ', 'WORKSPACE')
        ON CONFLICT (role_id, permission_id, scope_type) DO NOTHING;
    END LOOP;
END $$;

-- 12. 为现有的admin用户分配超级管理员角色

DO $$
DECLARE
    admin_role_id INTEGER;
    admin_user RECORD;
    default_org_id INTEGER := 1;
BEGIN
    SELECT id INTO admin_role_id FROM iam_roles WHERE name = 'admin';
    
    -- 为所有role='admin'的用户分配超级管理员角色
    FOR admin_user IN SELECT id FROM users WHERE role = 'admin'
    LOOP
        INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id, reason)
        VALUES (admin_user.id, admin_role_id, 'ORGANIZATION', default_org_id, '系统自动分配：原admin角色用户')
        ON CONFLICT (user_id, role_id, scope_type, scope_id) DO NOTHING;
    END LOOP;
END $$;

-- 13. 创建视图：用户有效角色（包含继承）
CREATE OR REPLACE VIEW v_user_effective_roles AS
SELECT 
    ur.user_id,
    ur.role_id,
    r.name as role_name,
    r.display_name as role_display_name,
    ur.scope_type,
    ur.scope_id,
    ur.assigned_at,
    ur.expires_at,
    CASE 
        WHEN ur.expires_at IS NULL THEN TRUE
        WHEN ur.expires_at > CURRENT_TIMESTAMP THEN TRUE
        ELSE FALSE
    END as is_valid
FROM iam_user_roles ur
JOIN iam_roles r ON ur.role_id = r.id
WHERE r.is_active = TRUE;

-- 14. 创建视图：角色权限详情
CREATE OR REPLACE VIEW v_role_permissions AS
SELECT 
    r.id as role_id,
    r.name as role_name,
    r.display_name as role_display_name,
    rp.scope_type,
    pd.id as permission_id,
    pd.name as permission_name,
    pd.display_name as permission_display_name,
    pd.resource_type,
    rp.permission_level
FROM iam_roles r
JOIN iam_role_policies rp ON r.id = rp.role_id
JOIN permission_definitions pd ON rp.permission_id = pd.id
WHERE r.is_active = TRUE;

COMMENT ON TABLE iam_roles IS 'IAM角色定义表，类似AWS IAM Role';
COMMENT ON TABLE iam_role_policies IS 'IAM角色包含的权限策略';
COMMENT ON TABLE iam_user_roles IS '用户分配的角色';
COMMENT ON COLUMN iam_roles.is_system IS '系统预定义角色，不可删除';
COMMENT ON COLUMN iam_role_policies.scope_type IS '该策略适用的作用域类型';
COMMENT ON COLUMN iam_user_roles.scope_type IS '角色分配的作用域';
COMMENT ON COLUMN iam_user_roles.scope_id IS '角色分配的作用域ID';
