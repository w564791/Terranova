-- 添加组织级别的WORKSPACES权限定义

BEGIN;

-- 插入WORKSPACES权限定义（如果不存在）
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
    'workspaces',
    'WORKSPACES',
    'ORGANIZATION',
    '工作空间管理',
    '管理组织下的所有工作空间',
    true,
    NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM permission_definitions 
    WHERE resource_type = 'WORKSPACES' AND scope_level = 'ORGANIZATION'
);

COMMIT;

-- 验证插入结果
SELECT id, name, resource_type, scope_level, display_name, description
FROM permission_definitions
WHERE resource_type = 'WORKSPACES';
