-- 添加CMDB权限定义
-- CMDB功能：READ（只读查看）、ADMIN（同步数据）

-- 1. 生成唯一ID
DO $$
DECLARE
    v_perm_id VARCHAR(32);
    v_admin_role_id INTEGER;
BEGIN
    -- 生成权限ID
    v_perm_id := 'perm_cmdb_' || substr(md5(random()::text), 1, 16);
    
    -- 检查是否已存在
    IF NOT EXISTS (SELECT 1 FROM permission_definitions WHERE resource_type = 'cmdb') THEN
        -- 插入CMDB权限定义
        INSERT INTO permission_definitions (
            id,
            name, 
            resource_type,
            scope_level,
            display_name,
            description,
            is_system,
            created_at
        ) VALUES (
            v_perm_id,
            'cmdb',
            'cmdb',
            'ORGANIZATION',
            'CMDB资源索引',
            '查看CMDB资源索引、资源树和全局搜索功能。ADMIN级别可同步资源数据。',
            true,
            NOW()
        );
        
        RAISE NOTICE 'CMDB permission definition created with ID: %', v_perm_id;
    ELSE
        -- 获取已存在的权限ID
        SELECT id INTO v_perm_id FROM permission_definitions WHERE resource_type = 'cmdb';
        RAISE NOTICE 'CMDB permission already exists with ID: %', v_perm_id;
    END IF;
    
    -- 获取admin角色ID
    SELECT id INTO v_admin_role_id FROM iam_roles WHERE name = 'admin' LIMIT 1;
    
    IF v_admin_role_id IS NOT NULL THEN
        -- 检查是否已有该权限策略
        IF NOT EXISTS (
            SELECT 1 FROM iam_role_policies 
            WHERE role_id = v_admin_role_id AND permission_id = v_perm_id
        ) THEN
            -- 为admin角色添加CMDB ADMIN权限
            INSERT INTO iam_role_policies (
                role_id,
                permission_id,
                permission_level,
                scope_type,
                created_at
            ) VALUES (
                v_admin_role_id,
                v_perm_id,
                'ADMIN',
                'ORGANIZATION',
                NOW()
            );
            
            RAISE NOTICE 'CMDB ADMIN permission granted to admin role';
        ELSE
            RAISE NOTICE 'Admin role already has CMDB permission';
        END IF;
    ELSE
        RAISE NOTICE 'Admin role not found, skipping role assignment';
    END IF;
END $$;

-- 2. 验证权限创建
SELECT 
    id,
    name,
    resource_type,
    scope_level,
    display_name,
    description
FROM permission_definitions
WHERE resource_type = 'cmdb';

-- 3. 验证角色策略
SELECT 
    rp.id,
    r.name as role_name,
    pd.name as permission_name,
    rp.permission_level,
    rp.scope_type
FROM iam_role_policies rp
JOIN iam_roles r ON rp.role_id = r.id
JOIN permission_definitions pd ON rp.permission_id = pd.id
WHERE pd.resource_type = 'cmdb';

-- 输出结果
DO $$
BEGIN
    RAISE NOTICE 'CMDB permission setup completed';
    RAISE NOTICE 'Permission levels: READ (view only), ADMIN (sync data)';
END $$;
