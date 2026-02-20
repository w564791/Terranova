-- 添加新的权限定义
-- 执行日期: 2025-10-24
-- 说明: 为新增的资源类型创建权限定义记录

-- 1. MODULE_DEMOS - 模块Demo管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('MODULE_DEMOS', 'MODULE_DEMOS', 'ORGANIZATION', '模块Demo管理', '管理模块的Demo和版本', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 2. SCHEMAS - Schema管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('SCHEMAS', 'SCHEMAS', 'ORGANIZATION', 'Schema管理', '管理模块的Schema定义', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 3. TASK_LOGS - 任务日志
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('TASK_LOGS', 'TASK_LOGS', 'ORGANIZATION', '任务日志访问', '查看和下载任务执行日志', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 4. AGENTS - Agent管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('AGENTS', 'AGENTS', 'ORGANIZATION', 'Agent管理', '管理Terraform执行Agent', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 5. AGENT_POOLS - Agent Pool管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('AGENT_POOLS', 'AGENT_POOLS', 'ORGANIZATION', 'Agent Pool管理', '管理Agent资源池', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 6. IAM_PERMISSIONS - IAM权限管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_PERMISSIONS', 'IAM_PERMISSIONS', 'ORGANIZATION', 'IAM权限管理', '管理IAM权限授予和撤销', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 7. IAM_TEAMS - IAM团队管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_TEAMS', 'IAM_TEAMS', 'ORGANIZATION', 'IAM团队管理', '管理组织团队和成员', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 8. IAM_ORGANIZATIONS - IAM组织管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_ORGANIZATIONS', 'IAM_ORGANIZATIONS', 'ORGANIZATION', 'IAM组织管理', '管理组织信息和配置', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 9. IAM_PROJECTS - IAM项目管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_PROJECTS', 'IAM_PROJECTS', 'ORGANIZATION', 'IAM项目管理', '管理组织内的项目', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 10. IAM_APPLICATIONS - IAM应用管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_APPLICATIONS', 'IAM_APPLICATIONS', 'ORGANIZATION', 'IAM应用管理', '管理API应用和密钥', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 11. IAM_AUDIT - IAM审计日志
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_AUDIT', 'IAM_AUDIT', 'ORGANIZATION', 'IAM审计日志', '查看和管理审计日志', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 12. IAM_USERS - IAM用户管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_USERS', 'IAM_USERS', 'ORGANIZATION', 'IAM用户管理', '管理用户账号和角色', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 13. IAM_ROLES - IAM角色管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('IAM_ROLES', 'IAM_ROLES', 'ORGANIZATION', 'IAM角色管理', '管理角色和策略', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 14. TERRAFORM_VERSIONS - Terraform版本管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('TERRAFORM_VERSIONS', 'TERRAFORM_VERSIONS', 'ORGANIZATION', 'Terraform版本管理', '管理Terraform版本配置', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 15. AI_CONFIGS - AI配置管理
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('AI_CONFIGS', 'AI_CONFIGS', 'ORGANIZATION', 'AI配置管理', '管理AI模型配置和优先级', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 16. AI_ANALYSIS - AI分析
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('AI_ANALYSIS', 'AI_ANALYSIS', 'ORGANIZATION', 'AI错误分析', '使用AI分析错误和问题', true, NOW())
ON CONFLICT (name) DO NOTHING;

-- 验证插入结果
SELECT 
    name,
    resource_type,
    scope_level,
    display_name,
    is_system,
    created_at
FROM permission_definitions
WHERE resource_type IN (
    'MODULE_DEMOS', 'SCHEMAS', 'TASK_LOGS',
    'AGENTS', 'AGENT_POOLS',
    'IAM_PERMISSIONS', 'IAM_TEAMS', 'IAM_ORGANIZATIONS', 'IAM_PROJECTS',
    'IAM_APPLICATIONS', 'IAM_AUDIT', 'IAM_USERS', 'IAM_ROLES',
    'TERRAFORM_VERSIONS', 'AI_CONFIGS', 'AI_ANALYSIS'
)
ORDER BY name;

-- 统计信息
SELECT 
    '新增权限定义' as description,
    COUNT(*) as count
FROM permission_definitions
WHERE resource_type IN (
    'MODULE_DEMOS', 'SCHEMAS', 'TASK_LOGS',
    'AGENTS', 'AGENT_POOLS',
    'IAM_PERMISSIONS', 'IAM_TEAMS', 'IAM_ORGANIZATIONS', 'IAM_PROJECTS',
    'IAM_APPLICATIONS', 'IAM_AUDIT', 'IAM_USERS', 'IAM_ROLES',
    'TERRAFORM_VERSIONS', 'AI_CONFIGS', 'AI_ANALYSIS'
);
