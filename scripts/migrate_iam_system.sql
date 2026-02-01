-- =============================================
-- IaC Platform 权限系统数据库迁移脚本
-- 版本: v2.0
-- 基于: iac-platform-permission-system-design-v2.md
-- =============================================

-- =============================================
-- 第一部分: 核心实体表
-- =============================================

-- 1. 组织表 (organizations)
CREATE TABLE IF NOT EXISTS organizations (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    settings JSONB,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_organizations_name ON organizations(name);
CREATE INDEX IF NOT EXISTS idx_organizations_active ON organizations(is_active);

COMMENT ON TABLE organizations IS '组织表（租户边界）';
COMMENT ON COLUMN organizations.name IS '组织唯一标识名称';
COMMENT ON COLUMN organizations.is_active IS '是否启用';

-- 2. 项目表 (projects)
CREATE TABLE IF NOT EXISTS projects (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_default BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    settings JSONB,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_projects_org ON projects(org_id);
CREATE INDEX IF NOT EXISTS idx_projects_org_active ON projects(org_id, is_active);
CREATE INDEX IF NOT EXISTS idx_projects_default ON projects(is_default);

COMMENT ON TABLE projects IS '项目表';
COMMENT ON COLUMN projects.is_default IS '是否为默认项目';

-- 3. 工作空间-项目关联表 (workspace_project_relations)
CREATE TABLE IF NOT EXISTS workspace_project_relations (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(workspace_id)
);

CREATE INDEX IF NOT EXISTS idx_wpr_workspace ON workspace_project_relations(workspace_id);
CREATE INDEX IF NOT EXISTS idx_wpr_project ON workspace_project_relations(project_id);

COMMENT ON TABLE workspace_project_relations IS '工作空间-项目关联表（一个workspace只能属于一个project）';

-- 4. 扩展workspaces表（添加新字段，不修改现有字段）
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS workspace_type VARCHAR(50) DEFAULT 'GENERAL';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS is_locked BOOLEAN DEFAULT FALSE;

COMMENT ON COLUMN workspaces.workspace_type IS '工作空间类型: GENERAL(通用), TASK_POOL(任务池), DATASET(数据集), MODULE(模块库), API_SERVICE(API服务), TRAINING(训练环境), TESTING(测试环境)';

-- 5. 团队表 (teams)
CREATE TABLE IF NOT EXISTS teams (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_system BOOLEAN DEFAULT FALSE,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_teams_org ON teams(org_id);
CREATE INDEX IF NOT EXISTS idx_teams_system ON teams(is_system);

COMMENT ON TABLE teams IS '团队表';
COMMENT ON COLUMN teams.is_system IS '是否为系统预置团队（不可删除）';

-- 6. 团队成员表 (team_members)
CREATE TABLE IF NOT EXISTS team_members (
    id SERIAL PRIMARY KEY,
    team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) DEFAULT 'MEMBER',
    joined_at TIMESTAMP DEFAULT NOW(),
    joined_by INTEGER REFERENCES users(id),
    
    UNIQUE(team_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_team_members_team ON team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_user ON team_members(user_id);

COMMENT ON TABLE team_members IS '团队成员关系表';
COMMENT ON COLUMN team_members.role IS '团队内角色: MEMBER(成员), MAINTAINER(维护者)';

-- 7. 用户-组织关系表 (user_organizations)
CREATE TABLE IF NOT EXISTS user_organizations (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    joined_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(user_id, org_id)
);

CREATE INDEX IF NOT EXISTS idx_user_orgs_user ON user_organizations(user_id);
CREATE INDEX IF NOT EXISTS idx_user_orgs_org ON user_organizations(org_id);

COMMENT ON TABLE user_organizations IS '用户-组织关系表';

-- 8. 应用表 (applications)
CREATE TABLE IF NOT EXISTS applications (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    app_key VARCHAR(500) UNIQUE NOT NULL,
    app_secret VARCHAR(500),
    description TEXT,
    callback_urls JSONB,
    is_active BOOLEAN DEFAULT TRUE,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    
    UNIQUE(org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_applications_org ON applications(org_id);
CREATE INDEX IF NOT EXISTS idx_applications_org_active ON applications(org_id, is_active);
CREATE INDEX IF NOT EXISTS idx_applications_key ON applications(app_key);

COMMENT ON TABLE applications IS '应用表（外部系统/Agent）';
COMMENT ON COLUMN applications.app_secret IS 'API Secret（加密存储）';

-- 9. 扩展users表（添加新字段）
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_system_admin BOOLEAN DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS oauth_provider VARCHAR(50);
ALTER TABLE users ADD COLUMN IF NOT EXISTS oauth_id VARCHAR(200);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_users_oauth ON users(oauth_provider, oauth_id);

COMMENT ON COLUMN users.is_system_admin IS '是否为系统超级管理员';

-- =============================================
-- 第二部分: 权限定义表
-- =============================================

-- 10. 权限定义表 (permission_definitions)
CREATE TABLE IF NOT EXISTS permission_definitions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    scope_level VARCHAR(20) NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_system BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_perm_defs_resource ON permission_definitions(resource_type);
CREATE INDEX IF NOT EXISTS idx_perm_defs_scope ON permission_definitions(scope_level);
CREATE INDEX IF NOT EXISTS idx_perm_defs_system ON permission_definitions(is_system);

COMMENT ON TABLE permission_definitions IS '权限定义表';
COMMENT ON COLUMN permission_definitions.scope_level IS '适用作用域层级: ORGANIZATION, PROJECT, WORKSPACE';

-- 11. 权限预设表 (permission_presets)
CREATE TABLE IF NOT EXISTS permission_presets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL,
    scope_level VARCHAR(20) NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    is_system BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(name, scope_level)
);

CREATE INDEX IF NOT EXISTS idx_presets_scope ON permission_presets(scope_level);

COMMENT ON TABLE permission_presets IS '权限预设表（READ/WRITE/ADMIN）';

-- 12. 权限预设详情表 (preset_permissions)
CREATE TABLE IF NOT EXISTS preset_permissions (
    id SERIAL PRIMARY KEY,
    preset_id INTEGER NOT NULL REFERENCES permission_presets(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permission_definitions(id),
    permission_level INTEGER NOT NULL,
    
    UNIQUE(preset_id, permission_id)
);

CREATE INDEX IF NOT EXISTS idx_preset_perms_preset ON preset_permissions(preset_id);

COMMENT ON TABLE preset_permissions IS '权限预设包含的具体权限';
COMMENT ON COLUMN preset_permissions.permission_level IS '权限等级: 0=NONE, 1=READ, 2=WRITE, 3=ADMIN';

-- =============================================
-- 第三部分: 权限分配表
-- =============================================

-- 13. 组织级权限分配表 (org_permissions)
CREATE TABLE IF NOT EXISTS org_permissions (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    principal_type VARCHAR(20) NOT NULL,
    principal_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL REFERENCES permission_definitions(id),
    permission_level INTEGER NOT NULL,
    granted_by INTEGER REFERENCES users(id),
    granted_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    reason TEXT,
    
    UNIQUE(org_id, principal_type, principal_id, permission_id)
);

CREATE INDEX IF NOT EXISTS idx_org_perms_principal ON org_permissions(principal_type, principal_id);
CREATE INDEX IF NOT EXISTS idx_org_perms_org_principal ON org_permissions(org_id, principal_type, principal_id);
CREATE INDEX IF NOT EXISTS idx_org_perms_permission ON org_permissions(permission_id, permission_level);
CREATE INDEX IF NOT EXISTS idx_org_perms_expires ON org_permissions(expires_at);

COMMENT ON TABLE org_permissions IS '组织级权限分配表';
COMMENT ON COLUMN org_permissions.principal_type IS '主体类型: USER, TEAM, APPLICATION';
COMMENT ON COLUMN org_permissions.permission_level IS '权限等级: 0=NONE, 1=READ, 2=WRITE, 3=ADMIN';

-- 14. 项目级权限分配表 (project_permissions)
CREATE TABLE IF NOT EXISTS project_permissions (
    id SERIAL PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    principal_type VARCHAR(20) NOT NULL,
    principal_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL REFERENCES permission_definitions(id),
    permission_level INTEGER NOT NULL,
    granted_by INTEGER REFERENCES users(id),
    granted_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    reason TEXT,
    
    UNIQUE(project_id, principal_type, principal_id, permission_id)
);

CREATE INDEX IF NOT EXISTS idx_proj_perms_principal ON project_permissions(principal_type, principal_id);
CREATE INDEX IF NOT EXISTS idx_proj_perms_project_principal ON project_permissions(project_id, principal_type, principal_id);
CREATE INDEX IF NOT EXISTS idx_proj_perms_permission ON project_permissions(permission_id, permission_level);
CREATE INDEX IF NOT EXISTS idx_proj_perms_expires ON project_permissions(expires_at);

COMMENT ON TABLE project_permissions IS '项目级权限分配表';
COMMENT ON COLUMN project_permissions.principal_type IS '主体类型: USER, TEAM';

-- 15. 工作空间级权限分配表 (workspace_permissions)
CREATE TABLE IF NOT EXISTS workspace_permissions (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    principal_type VARCHAR(20) NOT NULL,
    principal_id INTEGER NOT NULL,
    permission_id INTEGER NOT NULL REFERENCES permission_definitions(id),
    permission_level INTEGER NOT NULL,
    granted_by INTEGER REFERENCES users(id),
    granted_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    reason TEXT,
    
    UNIQUE(workspace_id, principal_type, principal_id, permission_id)
);

CREATE INDEX IF NOT EXISTS idx_ws_perms_principal ON workspace_permissions(principal_type, principal_id);
CREATE INDEX IF NOT EXISTS idx_ws_perms_workspace_principal ON workspace_permissions(workspace_id, principal_type, principal_id);
CREATE INDEX IF NOT EXISTS idx_ws_perms_permission ON workspace_permissions(permission_id, permission_level);
CREATE INDEX IF NOT EXISTS idx_ws_perms_expires ON workspace_permissions(expires_at);

COMMENT ON TABLE workspace_permissions IS '工作空间级权限分配表';

-- =============================================
-- 第四部分: 临时权限表
-- =============================================

-- 16. 临时任务权限表 (task_temporary_permissions)
CREATE TABLE IF NOT EXISTS task_temporary_permissions (
    id SERIAL PRIMARY KEY,
    task_id INTEGER NOT NULL REFERENCES workspace_tasks(id) ON DELETE CASCADE,
    user_email VARCHAR(200) NOT NULL,
    user_id INTEGER REFERENCES users(id),
    permission_type VARCHAR(50) NOT NULL,
    granted_by VARCHAR(200),
    granted_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    webhook_payload JSONB,
    is_used BOOLEAN DEFAULT FALSE,
    used_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_temp_perms_task_user ON task_temporary_permissions(task_id, user_email);
CREATE INDEX IF NOT EXISTS idx_temp_perms_expires ON task_temporary_permissions(expires_at);
CREATE INDEX IF NOT EXISTS idx_temp_perms_used ON task_temporary_permissions(is_used);

COMMENT ON TABLE task_temporary_permissions IS '临时任务权限表（基于Webhook）';
COMMENT ON COLUMN task_temporary_permissions.permission_type IS '权限类型: APPLY, CANCEL';

-- 17. Webhook配置表 (webhook_configs)
CREATE TABLE IF NOT EXISTS webhook_configs (
    id SERIAL PRIMARY KEY,
    org_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    webhook_url VARCHAR(500) NOT NULL,
    secret_token VARCHAR(200),
    event_types JSONB,
    is_active BOOLEAN DEFAULT TRUE,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    
    UNIQUE(org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_webhook_configs_org ON webhook_configs(org_id);
CREATE INDEX IF NOT EXISTS idx_webhook_configs_active ON webhook_configs(is_active);

COMMENT ON TABLE webhook_configs IS 'Webhook配置表';
COMMENT ON COLUMN webhook_configs.event_types IS '事件类型数组: ["task.plan.completed", "task.approval.needed"]';

-- 18. Webhook日志表 (webhook_logs)
CREATE TABLE IF NOT EXISTS webhook_logs (
    id SERIAL PRIMARY KEY,
    webhook_config_id INTEGER REFERENCES webhook_configs(id),
    event_type VARCHAR(100),
    task_id INTEGER REFERENCES workspace_tasks(id),
    request_payload JSONB,
    response_payload JSONB,
    status_code INTEGER,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_logs_config ON webhook_logs(webhook_config_id);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_task ON webhook_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_created ON webhook_logs(created_at);

COMMENT ON TABLE webhook_logs IS 'Webhook调用日志表';

-- =============================================
-- 第五部分: 审计日志表
-- =============================================

-- 19. 权限审计日志表 (permission_audit_logs)
CREATE TABLE IF NOT EXISTS permission_audit_logs (
    id SERIAL PRIMARY KEY,
    action_type VARCHAR(50) NOT NULL,
    scope_type VARCHAR(20) NOT NULL,
    scope_id INTEGER NOT NULL,
    principal_type VARCHAR(20),
    principal_id INTEGER,
    permission_id INTEGER,
    old_level INTEGER,
    new_level INTEGER,
    performed_by INTEGER REFERENCES users(id),
    performed_at TIMESTAMP DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_perm_audit_scope ON permission_audit_logs(scope_type, scope_id);
CREATE INDEX IF NOT EXISTS idx_perm_audit_principal ON permission_audit_logs(principal_type, principal_id);
CREATE INDEX IF NOT EXISTS idx_perm_audit_performer ON permission_audit_logs(performed_by);
CREATE INDEX IF NOT EXISTS idx_perm_audit_time ON permission_audit_logs(performed_at);
CREATE INDEX IF NOT EXISTS idx_perm_audit_action ON permission_audit_logs(action_type);

COMMENT ON TABLE permission_audit_logs IS '权限变更审计日志';
COMMENT ON COLUMN permission_audit_logs.action_type IS '操作类型: GRANT, REVOKE, MODIFY, EXPIRE';

-- 20. 访问日志表 (access_logs)
CREATE TABLE IF NOT EXISTS access_logs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    resource_type VARCHAR(100) NOT NULL,
    resource_id INTEGER NOT NULL,
    action VARCHAR(100) NOT NULL,
    is_allowed BOOLEAN NOT NULL,
    deny_reason VARCHAR(500),
    effective_level INTEGER,
    accessed_at TIMESTAMP DEFAULT NOW(),
    ip_address INET,
    duration_ms INTEGER
);

CREATE INDEX IF NOT EXISTS idx_access_logs_user ON access_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_access_logs_resource ON access_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_access_logs_time ON access_logs(accessed_at);
CREATE INDEX IF NOT EXISTS idx_access_logs_allowed ON access_logs(is_allowed);

COMMENT ON TABLE access_logs IS '资源访问日志（按月分区）';

-- =============================================
-- 第六部分: 初始化数据
-- =============================================

-- 插入权限定义
INSERT INTO permission_definitions (name, resource_type, scope_level, display_name, description) VALUES
-- 组织级权限
('application_registration', 'APPLICATION_REGISTRATION', 'ORGANIZATION', '应用注册', '管理应用注册权限'),
('organization_settings', 'ORGANIZATION_SETTINGS', 'ORGANIZATION', '组织设置', '管理组织配置'),
('user_management', 'USER_MANAGEMENT', 'ORGANIZATION', '用户管理', '管理组织用户'),
('all_projects', 'ALL_PROJECTS', 'ORGANIZATION', '所有项目', '访问所有项目'),

-- 项目级权限
('project_settings', 'PROJECT_SETTINGS', 'PROJECT', '项目设置', '管理项目配置'),
('project_team_management', 'PROJECT_TEAM_MANAGEMENT', 'PROJECT', '项目团队', '管理项目团队'),
('project_workspaces', 'PROJECT_WORKSPACES', 'PROJECT', '项目工作空间', '管理项目内工作空间'),

-- 工作空间级权限
('task_data_access', 'TASK_DATA_ACCESS', 'WORKSPACE', '任务数据', '访问任务数据'),
('workspace_execution', 'WORKSPACE_EXECUTION', 'WORKSPACE', '工作空间执行', '执行工作空间操作'),
('workspace_state', 'WORKSPACE_STATE', 'WORKSPACE', '状态管理', '管理工作空间状态'),
('workspace_variables', 'WORKSPACE_VARIABLES', 'WORKSPACE', '变量管理', '管理工作空间变量')
ON CONFLICT (name) DO NOTHING;

-- 插入权限预设
INSERT INTO permission_presets (name, scope_level, display_name, description) VALUES
-- 组织级预设
('READ', 'ORGANIZATION', '组织只读', '查看组织信息和项目列表'),
('WRITE', 'ORGANIZATION', '组织编辑', '管理组织资源（不含用户管理）'),
('ADMIN', 'ORGANIZATION', '组织管理员', '完全控制组织'),

-- 项目级预设
('READ', 'PROJECT', '项目只读', '查看项目信息和工作空间'),
('WRITE', 'PROJECT', '项目编辑', '管理项目工作空间'),
('ADMIN', 'PROJECT', '项目管理员', '完全控制项目'),

-- 工作空间级预设
('READ', 'WORKSPACE', '工作空间只读', '查看数据和配置'),
('WRITE', 'WORKSPACE', '工作空间编辑', '读写数据和执行操作'),
('ADMIN', 'WORKSPACE', '工作空间管理员', '完全控制工作空间')
ON CONFLICT (name, scope_level) DO NOTHING;

-- 配置权限预设详情（组织级 READ）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 1
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'READ' AND p.scope_level = 'ORGANIZATION'
  AND pd.scope_level = 'ORGANIZATION'
  AND pd.name = 'all_projects'
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（组织级 WRITE）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 2
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'WRITE' AND p.scope_level = 'ORGANIZATION'
  AND pd.scope_level = 'ORGANIZATION'
  AND pd.name IN ('all_projects', 'organization_settings')
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（组织级 ADMIN）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 3
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'ADMIN' AND p.scope_level = 'ORGANIZATION'
  AND pd.scope_level = 'ORGANIZATION'
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（项目级 READ）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 1
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'READ' AND p.scope_level = 'PROJECT'
  AND pd.scope_level = 'PROJECT'
  AND pd.name = 'project_workspaces'
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（项目级 WRITE）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 2
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'WRITE' AND p.scope_level = 'PROJECT'
  AND pd.scope_level = 'PROJECT'
  AND pd.name IN ('project_workspaces', 'project_settings')
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（项目级 ADMIN）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 3
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'ADMIN' AND p.scope_level = 'PROJECT'
  AND pd.scope_level = 'PROJECT'
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（工作空间级 READ）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 1
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'READ' AND p.scope_level = 'WORKSPACE'
  AND pd.scope_level = 'WORKSPACE'
  AND pd.name IN ('task_data_access', 'workspace_state')
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（工作空间级 WRITE）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 2
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'WRITE' AND p.scope_level = 'WORKSPACE'
  AND pd.scope_level = 'WORKSPACE'
  AND pd.name IN ('task_data_access', 'workspace_execution', 'workspace_state')
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- 配置权限预设详情（工作空间级 ADMIN）
INSERT INTO preset_permissions (preset_id, permission_id, permission_level)
SELECT p.id, pd.id, 3
FROM permission_presets p
CROSS JOIN permission_definitions pd
WHERE p.name = 'ADMIN' AND p.scope_level = 'WORKSPACE'
  AND pd.scope_level = 'WORKSPACE'
ON CONFLICT (preset_id, permission_id) DO NOTHING;

-- =============================================
-- 第七部分: 初始化默认数据
-- =============================================

-- 创建默认组织
INSERT INTO organizations (name, display_name, description)
VALUES ('default', 'Default Organization', 'Default organization for backward compatibility')
ON CONFLICT (name) DO NOTHING;

-- 创建默认项目
INSERT INTO projects (org_id, name, display_name, is_default)
SELECT id, 'default', 'Default Project', TRUE
FROM organizations
WHERE name = 'default'
ON CONFLICT (org_id, name) DO NOTHING;

-- 关联现有工作空间到默认项目
INSERT INTO workspace_project_relations (workspace_id, project_id)
SELECT w.id, p.id
FROM workspaces w
CROSS JOIN projects p
WHERE p.name = 'default' AND p.is_default = TRUE
  AND NOT EXISTS (
    SELECT 1 FROM workspace_project_relations wpr WHERE wpr.workspace_id = w.id
  );

-- 创建系统预置团队
INSERT INTO teams (org_id, name, display_name, is_system)
SELECT o.id, 'owners', 'Organization Owners', TRUE
FROM organizations o
WHERE o.name = 'default'
ON CONFLICT (org_id, name) DO NOTHING;

INSERT INTO teams (org_id, name, display_name, is_system)
SELECT o.id, 'admins', 'Organization Admins', TRUE
FROM organizations o
WHERE o.name = 'default'
ON CONFLICT (org_id, name) DO NOTHING;

-- =============================================
-- 完成
-- =============================================

-- 输出完成信息
DO $$
BEGIN
    RAISE NOTICE '权限系统数据库迁移完成！';
    RAISE NOTICE '已创建:';
    RAISE NOTICE '  - 20个核心表';
    RAISE NOTICE '  - 11个权限定义';
    RAISE NOTICE '  - 9个权限预设';
    RAISE NOTICE '  - 默认组织和项目';
    RAISE NOTICE '  - 系统预置团队';
END $$;
