-- 添加组织级别的MODULES权限定义

BEGIN;

-- 插入MODULES权限定义
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
    'modules',
    'MODULES',
    'ORGANIZATION',
    '模块管理',
    '管理组织下的所有模块（查看、编辑、新增、删除）',
    true,
    NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM permission_definitions 
    WHERE resource_type = 'MODULES' AND scope_level = 'ORGANIZATION'
);

COMMIT;

-- 验证插入结果
SELECT id, name, resource_type, scope_level, display_name, description
FROM permission_definitions
WHERE resource_type = 'MODULES';
