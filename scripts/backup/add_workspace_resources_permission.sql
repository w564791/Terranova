-- 添加工作空间级别的WORKSPACE_RESOURCES权限定义

BEGIN;

-- 插入WORKSPACE_RESOURCES权限定义
INSERT INTO permission_definitions (
    name,
    resource_type,
    scope_level,
    display_name,
    description,
    is_system,
    created_at
)
SELECT 
    'workspace_resources',
    'WORKSPACE_RESOURCES',
    'WORKSPACE',
    '资源管理',
    '管理工作空间的资源（查看、编辑、新增、删除）',
    true,
    NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM permission_definitions 
    WHERE resource_type = 'WORKSPACE_RESOURCES' AND scope_level = 'WORKSPACE'
);

COMMIT;

-- 验证插入结果
SELECT id, name, resource_type, scope_level, display_name, description
FROM permission_definitions
WHERE resource_type = 'WORKSPACE_RESOURCES';
