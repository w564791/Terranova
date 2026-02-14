-- 添加工作空间级别的WORKSPACE_MANAGEMENT权限定义

BEGIN;

-- 插入WORKSPACE_MANAGEMENT权限定义
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
    'workspace_management',
    'WORKSPACE_MANAGEMENT',
    'WORKSPACE',
    '工作空间管理',
    '管理工作空间配置（查看、修改、删除工作空间）',
    true,
    NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM permission_definitions 
    WHERE resource_type = 'WORKSPACE_MANAGEMENT' AND scope_level = 'WORKSPACE'
);

COMMIT;

-- 验证插入结果
SELECT id, name, resource_type, scope_level, display_name, description
FROM permission_definitions
WHERE resource_type = 'WORKSPACE_MANAGEMENT';
