-- 更新权限定义名称
-- ORGANIZATION_SETTINGS -> ORGANIZATION
-- ALL_PROJECTS -> PROJECTS

BEGIN;

-- 更新 ORGANIZATION_SETTINGS 为 ORGANIZATION
UPDATE permission_definitions 
SET resource_type = 'ORGANIZATION',
    name = 'organization',
    display_name = '组织管理',
    description = '管理组织的设置和配置'
WHERE resource_type = 'ORGANIZATION_SETTINGS';

-- 更新 ALL_PROJECTS 为 PROJECTS
UPDATE permission_definitions 
SET resource_type = 'PROJECTS',
    name = 'projects',
    display_name = '项目管理',
    description = '管理组织下的所有项目'
WHERE resource_type = 'ALL_PROJECTS';

-- 更新已授予的权限记录中的 resource_type（通过 permission_id 关联）
-- 这一步会自动通过外键关联更新，因为我们已经更新了 permission_definitions 表

COMMIT;

-- 验证更新结果
SELECT id, name, resource_type, display_name, description, scope_level
FROM permission_definitions
WHERE resource_type IN ('ORGANIZATION', 'PROJECTS')
ORDER BY id;
