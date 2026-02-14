-- 创建workspace_resources相关表
-- 用于资源级别版本控制功能

-- 1. 工作空间资源表
CREATE TABLE IF NOT EXISTS workspace_resources (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    resource_id VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_name VARCHAR(100) NOT NULL,
    current_version_id INTEGER,
    is_active BOOLEAN DEFAULT true,
    description TEXT,
    tags JSONB,
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_resources_workspace ON workspace_resources(workspace_id);
CREATE INDEX IF NOT EXISTS idx_workspace_resources_type ON workspace_resources(resource_type);
CREATE INDEX IF NOT EXISTS idx_workspace_resources_active ON workspace_resources(is_active);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_resources_unique ON workspace_resources(workspace_id, resource_id) WHERE is_active = true;

-- 2. 资源代码版本表
CREATE TABLE IF NOT EXISTS resource_code_versions (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    is_latest BOOLEAN DEFAULT false,
    tf_code JSONB NOT NULL,
    variables JSONB,
    change_summary TEXT,
    change_type VARCHAR(20),
    diff_from_previous TEXT,
    state_version_id INTEGER REFERENCES workspace_state_versions(id),
    task_id INTEGER REFERENCES workspace_tasks(id),
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_resource_code_versions_resource ON resource_code_versions(resource_id);
CREATE INDEX IF NOT EXISTS idx_resource_code_versions_latest ON resource_code_versions(is_latest);
CREATE UNIQUE INDEX IF NOT EXISTS idx_resource_code_versions_unique ON resource_code_versions(resource_id, version);

-- 3. 工作空间资源快照表
CREATE TABLE IF NOT EXISTS workspace_resources_snapshot (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    snapshot_name VARCHAR(100),
    resources_versions JSONB NOT NULL,
    task_id INTEGER REFERENCES workspace_tasks(id),
    state_version_id INTEGER REFERENCES workspace_state_versions(id),
    created_by INTEGER REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_workspace_resources_snapshot_workspace ON workspace_resources_snapshot(workspace_id);

-- 4. 资源依赖关系表
CREATE TABLE IF NOT EXISTS resource_dependencies (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    depends_on_resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    dependency_type VARCHAR(20) DEFAULT 'explicit',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_resource_dependencies_resource ON resource_dependencies(resource_id);
CREATE INDEX IF NOT EXISTS idx_resource_dependencies_depends_on ON resource_dependencies(depends_on_resource_id);

-- 添加外键约束（如果不存在）
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_workspace_resources_current_version'
    ) THEN
        ALTER TABLE workspace_resources 
        ADD CONSTRAINT fk_workspace_resources_current_version 
        FOREIGN KEY (current_version_id) REFERENCES resource_code_versions(id) ON DELETE SET NULL;
    END IF;
END $$;

-- 添加更新时间触发器
CREATE OR REPLACE FUNCTION update_workspace_resources_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_workspace_resources_updated_at ON workspace_resources;
CREATE TRIGGER trigger_update_workspace_resources_updated_at
    BEFORE UPDATE ON workspace_resources
    FOR EACH ROW
    EXECUTE FUNCTION update_workspace_resources_updated_at();

-- 验证表创建
SELECT 
    table_name,
    (SELECT COUNT(*) FROM information_schema.columns WHERE table_name = t.table_name) as column_count
FROM information_schema.tables t
WHERE table_schema = 'public' 
    AND table_name IN ('workspace_resources', 'resource_code_versions', 'workspace_resources_snapshot', 'resource_dependencies')
ORDER BY table_name;
