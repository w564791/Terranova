-- 创建workspace_variables表
-- 用于存储Workspace的Terraform变量和环境变量

CREATE TABLE IF NOT EXISTS workspace_variables (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    key VARCHAR(100) NOT NULL,
    value TEXT,
    variable_type VARCHAR(20) NOT NULL DEFAULT 'terraform', -- terraform, environment
    value_format VARCHAR(20) NOT NULL DEFAULT 'string', -- string, hcl
    sensitive BOOLEAN DEFAULT false,
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by INTEGER REFERENCES users(id),
    
    -- 确保同一workspace下key唯一
    CONSTRAINT unique_workspace_variable UNIQUE(workspace_id, key)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_variables_workspace_id ON workspace_variables(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_variables_key ON workspace_variables(key);
CREATE INDEX IF NOT EXISTS idx_workspace_variables_type ON workspace_variables(variable_type);

-- 添加注释
COMMENT ON TABLE workspace_variables IS 'Workspace变量表，存储Terraform变量和环境变量';
COMMENT ON COLUMN workspace_variables.workspace_id IS '关联的Workspace ID';
COMMENT ON COLUMN workspace_variables.key IS '变量名称';
COMMENT ON COLUMN workspace_variables.value IS '变量值（sensitive变量会加密存储）';
COMMENT ON COLUMN workspace_variables.variable_type IS '变量类型：terraform（Terraform变量）, environment（环境变量）';
COMMENT ON COLUMN workspace_variables.value_format IS '值格式：string（字符串）, hcl（HCL表达式）';
COMMENT ON COLUMN workspace_variables.sensitive IS '是否为敏感变量（敏感变量在API响应中会被隐藏）';
COMMENT ON COLUMN workspace_variables.description IS '变量描述';
