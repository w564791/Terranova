-- 使用业务语义ID初始化权限系统
-- 执行日期: 2025-10-24
-- 说明: 创建所有系统权限定义，使用业务语义ID格式

-- ============================================
-- 清理现有数据（可选，谨慎使用）
-- ============================================

-- TRUNCATE TABLE permission_definitions CASCADE;

-- ============================================
-- 组织级权限定义
-- ============================================

-- 1. ORGANIZATION - 组织设置
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-organization', 'ORGANIZATION', 'ORGANIZATION', 'ORGANIZATION', '组织设置', '管理组织基本信息和配置', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 2. WORKSPACES - 所有工作空间
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-workspaces', 'WORKSPACES', 'WORKSPACES', 'ORGANIZATION', '工作空间列表', '查看和管理组织内所有工作空间', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 3. MODULES - 模块管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-modules', 'MODULES', 'MODULES', 'ORGANIZATION', '模块管理', '管理Terraform模块', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 4. MODULE_DEMOS - 模块Demo管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-module-demos', 'MODULE_DEMOS', 'MODULE_DEMOS', 'ORGANIZATION', '模块Demo管理', '管理模块的Demo和版本', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 5. SCHEMAS - Schema管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-schemas', 'SCHEMAS', 'SCHEMAS', 'ORGANIZATION', 'Schema管理', '管理模块的Schema定义', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 6. TASK_LOGS - 任务日志
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-task-logs', 'TASK_LOGS', 'TASK_LOGS', 'ORGANIZATION', '任务日志访问', '查看和下载任务执行日志', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 7. AGENTS - Agent管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-agents', 'AGENTS', 'AGENTS', 'ORGANIZATION', 'Agent管理', '管理Terraform执行Agent', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 8. AGENT_POOLS - Agent Pool管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-agent-pools', 'AGENT_POOLS', 'AGENT_POOLS', 'ORGANIZATION', 'Agent Pool管理', '管理Agent资源池', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 9. IAM_PERMISSIONS - IAM权限管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-permissions', 'IAM_PERMISSIONS', 'IAM_PERMISSIONS', 'ORGANIZATION', 'IAM权限管理', '管理IAM权限授予和撤销', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 10. IAM_TEAMS - IAM团队管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-teams', 'IAM_TEAMS', 'IAM_TEAMS', 'ORGANIZATION', 'IAM团队管理', '管理组织团队和成员', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 11. IAM_ORGANIZATIONS - IAM组织管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-organizations', 'IAM_ORGANIZATIONS', 'IAM_ORGANIZATIONS', 'ORGANIZATION', 'IAM组织管理', '管理组织信息和配置', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 12. IAM_PROJECTS - IAM项目管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-projects', 'IAM_PROJECTS', 'IAM_PROJECTS', 'ORGANIZATION', 'IAM项目管理', '管理组织内的项目', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 13. IAM_APPLICATIONS - IAM应用管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-applications', 'IAM_APPLICATIONS', 'IAM_APPLICATIONS', 'ORGANIZATION', 'IAM应用管理', '管理API应用和密钥', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 14. IAM_AUDIT - IAM审计日志
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-audit', 'IAM_AUDIT', 'IAM_AUDIT', 'ORGANIZATION', 'IAM审计日志', '查看和管理审计日志', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 15. IAM_USERS - IAM用户管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-users', 'IAM_USERS', 'IAM_USERS', 'ORGANIZATION', 'IAM用户管理', '管理用户账号和角色', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 16. IAM_ROLES - IAM角色管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-iam-roles', 'IAM_ROLES', 'IAM_ROLES', 'ORGANIZATION', 'IAM角色管理', '管理角色和策略', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 17. TERRAFORM_VERSIONS - Terraform版本管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-terraform-versions', 'TERRAFORM_VERSIONS', 'TERRAFORM_VERSIONS', 'ORGANIZATION', 'Terraform版本管理', '管理Terraform版本配置', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 18. AI_CONFIGS - AI配置管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-ai-configs', 'AI_CONFIGS', 'AI_CONFIGS', 'ORGANIZATION', 'AI配置管理', '管理AI模型配置和优先级', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 19. AI_ANALYSIS - AI分析
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-ai-analysis', 'AI_ANALYSIS', 'AI_ANALYSIS', 'ORGANIZATION', 'AI错误分析', '使用AI分析错误和问题', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 20. USER_MANAGEMENT - 用户管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-user-management', 'USER_MANAGEMENT', 'USER_MANAGEMENT', 'ORGANIZATION', '用户管理', '管理用户账号信息', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- ============================================
-- 工作空间级权限定义
-- ============================================

-- 21. WORKSPACE_MANAGEMENT - 工作空间管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('wspm-workspace-management', 'WORKSPACE_MANAGEMENT', 'WORKSPACE_MANAGEMENT', 'WORKSPACE', '工作空间管理', '管理工作空间配置和设置', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 22. WORKSPACE_EXECUTION - 工作空间执行
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('wspm-workspace-execution', 'WORKSPACE_EXECUTION', 'WORKSPACE_EXECUTION', 'WORKSPACE', '工作空间执行', '执行Plan和Apply操作', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 23. WORKSPACE_STATE - 状态管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('wspm-workspace-state', 'WORKSPACE_STATE', 'WORKSPACE_STATE', 'WORKSPACE', '状态管理', '管理Terraform状态文件', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 24. WORKSPACE_VARIABLES - 变量管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('wspm-workspace-variables', 'WORKSPACE_VARIABLES', 'WORKSPACE_VARIABLES', 'WORKSPACE', '变量管理', '管理工作空间变量', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 25. WORKSPACE_RESOURCES - 资源管理
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('wspm-workspace-resources', 'WORKSPACE_RESOURCES', 'WORKSPACE_RESOURCES', 'WORKSPACE', '资源管理', '管理工作空间资源', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- ============================================
-- 验证插入结果
-- ============================================

SELECT 
    id,
    name,
    resource_type,
    scope_level,
    display_name,
    is_system
FROM permission_definitions
ORDER BY 
    CASE scope_level
        WHEN 'ORGANIZATION' THEN 1
        WHEN 'PROJECT' THEN 2
        WHEN 'WORKSPACE' THEN 3
    END,
    name;

-- 统计信息
SELECT 
    scope_level,
    COUNT(*) as count
FROM permission_definitions
GROUP BY scope_level
ORDER BY scope_level;

SELECT 
    '总权限定义数' as metric,
    COUNT(*) as count
FROM permission_definitions;

-- ============================================
-- 说明
-- ============================================

/*
业务语义ID格式说明：

1. 组织级权限: orgpm-{name}
   - orgpm-organization
   - orgpm-workspaces
   - orgpm-modules
   - 等等...

2. 项目级权限: pjpm-{name}
   - pjpm-project-settings
   - 等等...

3. 工作空间级权限: wspm-{name}
   - wspm-workspace-management
   - wspm-workspace-execution
   - 等等...

优点：
- ID在所有环境中保持一致
- 从ID就能识别权限所属作用域
- 数据库恢复不会导致权限错乱
- 审计日志更易理解

使用方法：
1. 在新环境中执行此脚本初始化权限
2. 所有环境使用相同的ID
3. 数据库恢复后权限关系保持正确
*/
