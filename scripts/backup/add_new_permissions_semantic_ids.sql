-- 添加新的权限定义（使用语义ID）
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at) VALUES 
('orgpm-module-demos', 'MODULE_DEMOS', 'MODULE_DEMOS', 'ORGANIZATION', '模块Demo管理', '管理模块的Demo和版本', true, NOW()),
('orgpm-schemas', 'SCHEMAS', 'SCHEMAS', 'ORGANIZATION', 'Schema管理', '管理模块的Schema定义', true, NOW()),
('orgpm-task-logs', 'TASK_LOGS', 'TASK_LOGS', 'ORGANIZATION', '任务日志', '查看和下载任务执行日志', true, NOW()),
('orgpm-agents', 'AGENTS', 'AGENTS', 'ORGANIZATION', 'Agent管理', '管理Terraform执行Agent', true, NOW()),
('orgpm-agent-pools', 'AGENT_POOLS', 'AGENT_POOLS', 'ORGANIZATION', 'Agent Pool管理', '管理Agent资源池', true, NOW()),
('orgpm-iam-permissions', 'IAM_PERMISSIONS', 'IAM_PERMISSIONS', 'ORGANIZATION', 'IAM权限管理', '管理IAM权限授予和撤销', true, NOW()),
('orgpm-iam-teams', 'IAM_TEAMS', 'IAM_TEAMS', 'ORGANIZATION', 'IAM团队管理', '管理组织团队和成员', true, NOW()),
('orgpm-iam-organizations', 'IAM_ORGANIZATIONS', 'IAM_ORGANIZATIONS', 'ORGANIZATION', 'IAM组织管理', '管理组织信息和配置', true, NOW()),
('orgpm-iam-projects', 'IAM_PROJECTS', 'IAM_PROJECTS', 'ORGANIZATION', 'IAM项目管理', '管理组织内的项目', true, NOW()),
('orgpm-iam-applications', 'IAM_APPLICATIONS', 'IAM_APPLICATIONS', 'ORGANIZATION', 'IAM应用管理', '管理API应用和密钥', true, NOW()),
('orgpm-iam-audit', 'IAM_AUDIT', 'IAM_AUDIT', 'ORGANIZATION', 'IAM审计日志', '查看和管理审计日志', true, NOW()),
('orgpm-iam-users', 'IAM_USERS', 'IAM_USERS', 'ORGANIZATION', 'IAM用户管理', '管理用户账号和角色', true, NOW()),
('orgpm-iam-roles', 'IAM_ROLES', 'IAM_ROLES', 'ORGANIZATION', 'IAM角色管理', '管理角色和策略', true, NOW()),
('orgpm-terraform-versions', 'TERRAFORM_VERSIONS', 'TERRAFORM_VERSIONS', 'ORGANIZATION', 'Terraform版本管理', '管理Terraform版本配置', true, NOW()),
('orgpm-ai-configs', 'AI_CONFIGS', 'AI_CONFIGS', 'ORGANIZATION', 'AI配置管理', '管理AI模型配置和优先级', true, NOW()),
('orgpm-ai-analysis', 'AI_ANALYSIS', 'AI_ANALYSIS', 'ORGANIZATION', 'AI错误分析', '使用AI分析错误和问题', true, NOW())
ON CONFLICT (id) DO NOTHING;

SELECT COUNT(*) as new_permissions_added FROM permission_definitions WHERE id LIKE 'orgpm-%' AND name IN ('MODULE_DEMOS', 'SCHEMAS', 'TASK_LOGS', 'AGENTS', 'AGENT_POOLS', 'IAM_PERMISSIONS', 'IAM_TEAMS', 'IAM_ORGANIZATIONS', 'IAM_PROJECTS', 'IAM_APPLICATIONS', 'IAM_AUDIT', 'IAM_USERS', 'IAM_ROLES', 'TERRAFORM_VERSIONS', 'AI_CONFIGS', 'AI_ANALYSIS');
