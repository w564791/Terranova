-- IaC平台数据库Schema设计
-- 数据库: PostgreSQL 15+

-- 用户表
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) DEFAULT 'user', -- admin, user
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 模块表
CREATE TABLE modules (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    provider VARCHAR(50) NOT NULL,
    source VARCHAR(500) NOT NULL,
    version VARCHAR(20) NOT NULL,
    description TEXT,
    vcs_provider_id INTEGER REFERENCES vcs_providers(id),
    repository_url VARCHAR(500),
    branch VARCHAR(100) DEFAULT 'main',
    path VARCHAR(500) DEFAULT '/',
    module_files JSONB, -- 存储module文件内容
    sync_status VARCHAR(20) DEFAULT 'pending', -- pending, syncing, synced, failed
    last_sync_at TIMESTAMP,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(name, provider, version)
);

-- Schema表
CREATE TABLE schemas (
    id SERIAL PRIMARY KEY,
    module_id INTEGER REFERENCES modules(id),
    schema_data JSONB NOT NULL,
    version VARCHAR(20) NOT NULL,
    status VARCHAR(20) DEFAULT 'draft', -- draft, active, deprecated
    ai_generated BOOLEAN DEFAULT false,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 版本控制系统表
CREATE TABLE vcs_providers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL, -- github, gitlab, bitbucket
    base_url VARCHAR(500) NOT NULL,
    api_token_encrypted TEXT,
    webhook_secret VARCHAR(100),
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 工作空间表
CREATE TABLE workspaces (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    state_backend VARCHAR(20) NOT NULL, -- local, s3, remote
    state_config JSONB, -- 状态后端配置
    terraform_version VARCHAR(20) DEFAULT 'latest',
    execution_mode VARCHAR(20) DEFAULT 'server', -- server, agent, k8s
    agent_pool_id INTEGER, -- 关联的agent池
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(name, created_by)
);

-- 工作空间变量表
CREATE TABLE workspace_variables (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id),
    key VARCHAR(100) NOT NULL,
    value TEXT,
    is_sensitive BOOLEAN DEFAULT false,
    is_system BOOLEAN DEFAULT false, -- 系统变量，普通用户不可见
    description TEXT,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(workspace_id, key)
);

-- Agent池表
CREATE TABLE agent_pools (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    pool_type VARCHAR(20) NOT NULL, -- static, k8s
    k8s_config JSONB, -- k8s配置信息
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Agent表
CREATE TABLE agents (
    id SERIAL PRIMARY KEY,
    pool_id INTEGER REFERENCES agent_pools(id),
    name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'offline', -- online, offline, busy
    last_heartbeat TIMESTAMP,
    capabilities JSONB, -- 支持的terraform版本等
    metadata JSONB, -- IP、版本等信息
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 工作空间成员表
CREATE TABLE workspace_members (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id),
    user_id INTEGER REFERENCES users(id),
    role VARCHAR(20) DEFAULT 'member', -- owner, admin, member, viewer
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(workspace_id, user_id)
);

-- 部署记录表
CREATE TABLE deployments (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER REFERENCES workspaces(id),
    module_id INTEGER REFERENCES modules(id),
    schema_id INTEGER REFERENCES schemas(id),
    name VARCHAR(100) NOT NULL,
    config_data JSONB NOT NULL, -- 用户填写的表单数据
    terraform_config JSONB, -- 生成的terraform配置
    terraform_output JSONB, -- terraform执行输出
    status VARCHAR(20) NOT NULL, -- pending, planning, applying, success, failed, destroying
    execution_mode VARCHAR(20) NOT NULL, -- server, agent, k8s
    agent_id INTEGER REFERENCES agents(id), -- 执行的agent
    k8s_job_name VARCHAR(100), -- k8s job名称
    error_message TEXT,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 检测结果表
CREATE TABLE scan_results (
    id SERIAL PRIMARY KEY,
    deployment_id INTEGER REFERENCES deployments(id),
    scan_type VARCHAR(20) NOT NULL, -- security, cost, compliance
    results JSONB NOT NULL,
    score INTEGER,
    passed BOOLEAN,
    created_at TIMESTAMP DEFAULT NOW()
);

-- AI解析任务表
CREATE TABLE ai_parse_tasks (
    id SERIAL PRIMARY KEY,
    module_id INTEGER REFERENCES modules(id),
    status VARCHAR(20) NOT NULL, -- pending, processing, completed, failed
    input_data JSONB, -- 输入的module文件内容
    output_schema JSONB, -- 生成的schema
    error_message TEXT,
    processing_time INTEGER, -- 处理时间(秒)
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 系统配置表
CREATE TABLE system_configs (
    id SERIAL PRIMARY KEY,
    key VARCHAR(100) UNIQUE NOT NULL,
    value JSONB NOT NULL,
    description TEXT,
    updated_by INTEGER REFERENCES users(id),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- 审计日志表
CREATE TABLE audit_logs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    action VARCHAR(50) NOT NULL, -- create, update, delete, deploy, etc.
    resource_type VARCHAR(50) NOT NULL, -- module, schema, deployment, etc.
    resource_id INTEGER,
    old_values JSONB,
    new_values JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- 创建索引
-- 用户表索引
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_role ON users(role);

-- 模块表索引
CREATE INDEX idx_modules_name_provider ON modules(name, provider);
CREATE INDEX idx_modules_created_by ON modules(created_by);
CREATE INDEX idx_modules_created_at ON modules(created_at);
CREATE INDEX idx_modules_vcs_provider ON modules(vcs_provider_id);
CREATE INDEX idx_modules_sync_status ON modules(sync_status);

-- Schema表索引
CREATE INDEX idx_schemas_module_version ON schemas(module_id, version);
CREATE INDEX idx_schemas_status ON schemas(status);
CREATE INDEX idx_schemas_created_by ON schemas(created_by);
CREATE INDEX idx_schemas_data_gin ON schemas USING GIN(schema_data);

-- VCS提供商表索引
CREATE INDEX idx_vcs_providers_name ON vcs_providers(name);
CREATE INDEX idx_vcs_providers_created_by ON vcs_providers(created_by);

-- 工作空间表索引
CREATE INDEX idx_workspaces_created_by ON workspaces(created_by);
CREATE INDEX idx_workspaces_name ON workspaces(name);
CREATE INDEX idx_workspaces_execution_mode ON workspaces(execution_mode);

-- 工作空间变量表索引
CREATE INDEX idx_workspace_variables_workspace ON workspace_variables(workspace_id);
CREATE INDEX idx_workspace_variables_key ON workspace_variables(key);
CREATE INDEX idx_workspace_variables_sensitive ON workspace_variables(is_sensitive);

-- Agent池表索引
CREATE INDEX idx_agent_pools_type ON agent_pools(pool_type);
CREATE INDEX idx_agent_pools_created_by ON agent_pools(created_by);

-- Agent表索引
CREATE INDEX idx_agents_pool ON agents(pool_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_heartbeat ON agents(last_heartbeat);

-- 部署记录表索引
CREATE INDEX idx_deployments_workspace ON deployments(workspace_id);
CREATE INDEX idx_deployments_status ON deployments(status);
CREATE INDEX idx_deployments_created_by ON deployments(created_by);
CREATE INDEX idx_deployments_created_at ON deployments(created_at);
CREATE INDEX idx_deployments_execution_mode ON deployments(execution_mode);
CREATE INDEX idx_deployments_agent ON deployments(agent_id);
CREATE INDEX idx_deployments_config_gin ON deployments USING GIN(config_data);

-- 检测结果表索引
CREATE INDEX idx_scan_results_deployment ON scan_results(deployment_id);
CREATE INDEX idx_scan_results_type ON scan_results(scan_type);
CREATE INDEX idx_scan_results_created_at ON scan_results(created_at);

-- AI解析任务表索引
CREATE INDEX idx_ai_parse_tasks_module ON ai_parse_tasks(module_id);
CREATE INDEX idx_ai_parse_tasks_status ON ai_parse_tasks(status);
CREATE INDEX idx_ai_parse_tasks_created_at ON ai_parse_tasks(created_at);

-- 审计日志表索引
CREATE INDEX idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);

-- 插入默认系统配置
INSERT INTO system_configs (key, value, description) VALUES
('ai_provider', '"openai"', 'AI服务提供商'),
('ai_model', '"gpt-4"', 'AI模型名称'),
('terraform_default_version', '"1.6.0"', '默认Terraform版本'),
('max_deployment_timeout', '3600', '部署超时时间(秒)'),
('enable_cost_estimation', 'true', '是否启用成本估算'),
('enable_security_scan', 'true', '是否启用安全扫描'),
('default_execution_mode', '"server"', '默认执行模式'),
('k8s_namespace', '"iac-platform"', 'K8s命名空间'),
('agent_heartbeat_timeout', '300', 'Agent心跳超时时间(秒)');

-- 创建默认管理员用户 (密码: admin123)
INSERT INTO users (username, email, password_hash, role) VALUES
('admin', 'admin@iac-platform.com', '$2a$10$92IXUNpkjO0rOQ5byMi.Ye4oKoEa3Ro9llC/.og/at2.uheWG/igi', 'admin');

-- 创建默认VCS提供商 (GitHub)
INSERT INTO vcs_providers (name, base_url, created_by) VALUES
('github', 'https://api.github.com', 1);

-- 创建默认Agent池
INSERT INTO agent_pools (name, description, pool_type, created_by) VALUES
('default-pool', '默认Agent池', 'static', 1);em_configs (key, value, description) VALUES
('ai_provider', '"openai"', 'AI服务提供商'),
('ai_model', '"gpt-4"', 'AI模型名称'),
('terraform_default_version', '"1.6.0"', '默认Terraform版本'),
('max_deployment_timeout', '3600', '部署超时时间(秒)'),
('enable_cost_estimation', 'true', '是否启用成本估算'),
('enable_security_scan', 'true', '是否启用安全扫描'),
('default_execution_mode', '"server"', '默认执行模式'),
('k8s_namespace', '"iac-platform"', 'K8s命名空间'),
('agent_heartbeat_timeout', '300', 'Agent心跳超时时间(秒)');

-- 创建默认管理员用户 (密码需要在应用层加密)
-- INSERT INTO users (username, email, password_hash, role) VALUES
-- ('admin', 'admin@example.com', '$2a$10$...', 'admin');