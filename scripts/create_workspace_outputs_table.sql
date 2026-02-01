-- 创建workspace_outputs表
-- 用于存储用户为workspace资源配置的outputs

CREATE TABLE IF NOT EXISTS workspace_outputs (
    id SERIAL PRIMARY KEY,
    workspace_id VARCHAR(50) NOT NULL,
    output_id VARCHAR(50) NOT NULL UNIQUE,  -- 语义化ID，如 out-xxxx
    
    -- 资源关联
    resource_name VARCHAR(200) NOT NULL,     -- 资源名称（module名称）
    
    -- Output配置
    output_name VARCHAR(200) NOT NULL,       -- output名称（用户输入的参数名）
    output_value VARCHAR(500) NOT NULL,      -- output值表达式，如 module.abc.xxxx
    description VARCHAR(500),                -- output描述
    sensitive BOOLEAN DEFAULT FALSE,         -- 是否敏感
    
    -- 元数据
    created_by VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- 索引
    CONSTRAINT fk_workspace_outputs_workspace FOREIGN KEY (workspace_id) 
        REFERENCES workspaces(workspace_id) ON DELETE CASCADE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_outputs_workspace_id ON workspace_outputs(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_outputs_resource_name ON workspace_outputs(resource_name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_outputs_unique ON workspace_outputs(workspace_id, resource_name, output_name);

-- 添加注释
COMMENT ON TABLE workspace_outputs IS '工作空间outputs配置表';
COMMENT ON COLUMN workspace_outputs.workspace_id IS '工作空间语义化ID';
COMMENT ON COLUMN workspace_outputs.output_id IS 'Output语义化ID';
COMMENT ON COLUMN workspace_outputs.resource_name IS '资源名称（module名称）';
COMMENT ON COLUMN workspace_outputs.output_name IS 'Output名称（用户输入的参数名）';
COMMENT ON COLUMN workspace_outputs.output_value IS 'Output值表达式';
COMMENT ON COLUMN workspace_outputs.description IS 'Output描述';
COMMENT ON COLUMN workspace_outputs.sensitive IS '是否敏感';
