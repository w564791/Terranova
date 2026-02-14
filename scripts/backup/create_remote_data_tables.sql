-- 创建workspace_remote_data表
-- 用于存储workspace的远程数据引用配置

CREATE TABLE IF NOT EXISTS workspace_remote_data (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL,
    remote_data_id VARCHAR(50) NOT NULL UNIQUE,  -- 语义化ID，如 rd-xxxx
    
    -- 远程workspace配置
    source_workspace_id VARCHAR(50) NOT NULL,    -- 源workspace ID
    data_name VARCHAR(200) NOT NULL,             -- 数据名称（用于生成local变量名）
    description VARCHAR(500),                    -- 描述
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- 索引
    CONSTRAINT fk_workspace_remote_data_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    CONSTRAINT fk_workspace_remote_data_source FOREIGN KEY (source_workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_remote_data_workspace_id ON workspace_remote_data(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_remote_data_source_workspace_id ON workspace_remote_data(source_workspace_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_remote_data_unique ON workspace_remote_data(workspace_id, source_workspace_id, data_name);

-- 添加注释
COMMENT ON TABLE workspace_remote_data IS '工作空间远程数据引用配置表';
COMMENT ON COLUMN workspace_remote_data.workspace_id IS '当前工作空间语义化ID';
COMMENT ON COLUMN workspace_remote_data.remote_data_id IS '远程数据引用语义化ID';
COMMENT ON COLUMN workspace_remote_data.source_workspace_id IS '源工作空间语义化ID';
COMMENT ON COLUMN workspace_remote_data.data_name IS '数据名称（用于生成local变量名）';
COMMENT ON COLUMN workspace_remote_data.description IS '描述';

-- ============================================================================
-- 创建remote_data_tokens表
-- 用于存储临时访问token（30分钟有效，最大使用5次）
-- ============================================================================

CREATE TABLE IF NOT EXISTS remote_data_tokens (
    id SERIAL PRIMARY KEY,
    token_id VARCHAR(50) NOT NULL UNIQUE,        -- 语义化ID，如 rdt-xxxx
    token VARCHAR(255) NOT NULL UNIQUE,          -- 实际token值
    
    -- 关联信息
    workspace_id VARCHAR(50) NOT NULL,           -- 被访问的workspace ID
    requester_workspace_id VARCHAR(50) NOT NULL, -- 请求方workspace ID
    task_id INTEGER,                             -- 关联的任务ID（可选）
    
    -- 使用限制
    max_uses INTEGER DEFAULT 5,                  -- 最大使用次数
    used_count INTEGER DEFAULT 0,                -- 已使用次数
    expires_at TIMESTAMP NOT NULL,               -- 过期时间
    
    -- 元数据
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    
    -- 索引
    CONSTRAINT fk_remote_data_tokens_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    CONSTRAINT fk_remote_data_tokens_requester FOREIGN KEY (requester_workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_remote_data_tokens_workspace_id ON remote_data_tokens(workspace_id);
CREATE INDEX IF NOT EXISTS idx_remote_data_tokens_requester_workspace_id ON remote_data_tokens(requester_workspace_id);
CREATE INDEX IF NOT EXISTS idx_remote_data_tokens_token ON remote_data_tokens(token);
CREATE INDEX IF NOT EXISTS idx_remote_data_tokens_expires_at ON remote_data_tokens(expires_at);

-- 添加注释
COMMENT ON TABLE remote_data_tokens IS '远程数据访问临时token表';
COMMENT ON COLUMN remote_data_tokens.token_id IS 'Token语义化ID';
COMMENT ON COLUMN remote_data_tokens.token IS '实际token值';
COMMENT ON COLUMN remote_data_tokens.workspace_id IS '被访问的工作空间ID';
COMMENT ON COLUMN remote_data_tokens.requester_workspace_id IS '请求方工作空间ID';
COMMENT ON COLUMN remote_data_tokens.task_id IS '关联的任务ID';
COMMENT ON COLUMN remote_data_tokens.max_uses IS '最大使用次数';
COMMENT ON COLUMN remote_data_tokens.used_count IS '已使用次数';
COMMENT ON COLUMN remote_data_tokens.expires_at IS '过期时间';

-- ============================================================================
-- 在workspaces表添加outputs访问控制字段
-- ============================================================================

-- 添加outputs_sharing字段到workspaces表
-- 值: 'none' - 不允许任何访问
--     'all' - 允许所有workspace访问
--     'specific' - 只允许指定的workspace访问
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS outputs_sharing VARCHAR(20) DEFAULT 'none';

-- 添加注释
COMMENT ON COLUMN workspaces.outputs_sharing IS 'Outputs共享模式: none/all/specific';

-- ============================================================================
-- 创建workspace_outputs_access表
-- 用于存储允许访问outputs的workspace列表（当outputs_sharing='specific'时使用）
-- ============================================================================

CREATE TABLE IF NOT EXISTS workspace_outputs_access (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL,           -- 被访问的workspace ID
    allowed_workspace_id VARCHAR(50) NOT NULL,   -- 允许访问的workspace ID
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- 索引
    CONSTRAINT fk_workspace_outputs_access_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    CONSTRAINT fk_workspace_outputs_access_allowed FOREIGN KEY (allowed_workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE,
    CONSTRAINT uq_workspace_outputs_access UNIQUE (workspace_id, allowed_workspace_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_outputs_access_workspace_id ON workspace_outputs_access(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_outputs_access_allowed_workspace_id ON workspace_outputs_access(allowed_workspace_id);

-- 添加注释
COMMENT ON TABLE workspace_outputs_access IS '工作空间Outputs访问控制表';
COMMENT ON COLUMN workspace_outputs_access.workspace_id IS '被访问的工作空间ID';
COMMENT ON COLUMN workspace_outputs_access.allowed_workspace_id IS '允许访问的工作空间ID';
