-- Manifest 可视化编排器数据库表
-- 创建时间: 2026-01-01

-- 1. manifests 表（Organization 级别）
CREATE TABLE IF NOT EXISTS manifests (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mf-{ulid}
    organization_id INTEGER NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    status          VARCHAR(20) DEFAULT 'draft',  -- draft, published, archived
    created_by      VARCHAR(20) NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(organization_id, name),
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(user_id)
);

CREATE INDEX IF NOT EXISTS idx_manifests_organization_id ON manifests(organization_id);
CREATE INDEX IF NOT EXISTS idx_manifests_status ON manifests(status);

-- 2. manifest_versions 表
CREATE TABLE IF NOT EXISTS manifest_versions (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mfv-{ulid}
    manifest_id     VARCHAR(36) NOT NULL,
    version         VARCHAR(50) NOT NULL,     -- 如 v1.0.0, draft
    canvas_data     JSONB NOT NULL DEFAULT '{}',  -- 画布数据（节点位置、缩放等）
    nodes           JSONB NOT NULL DEFAULT '[]',  -- 节点配置
    edges           JSONB NOT NULL DEFAULT '[]',  -- 连接关系
    variables       JSONB DEFAULT '[]',           -- 可配置的变量定义
    hcl_content     TEXT,                         -- 生成的 HCL 内容
    is_draft        BOOLEAN DEFAULT true,         -- 是否为草稿
    created_by      VARCHAR(20) NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(manifest_id, version),
    FOREIGN KEY (manifest_id) REFERENCES manifests(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(user_id)
);

CREATE INDEX IF NOT EXISTS idx_manifest_versions_manifest_id ON manifest_versions(manifest_id);
CREATE INDEX IF NOT EXISTS idx_manifest_versions_is_draft ON manifest_versions(is_draft);

-- 3. manifest_deployments 表（部署记录）
CREATE TABLE IF NOT EXISTS manifest_deployments (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mfd-{ulid}
    manifest_id     VARCHAR(36) NOT NULL,
    version_id      VARCHAR(36) NOT NULL,
    workspace_id    INTEGER NOT NULL,
    variable_overrides JSONB DEFAULT '{}',    -- 部署时覆盖的变量
    status          VARCHAR(20) DEFAULT 'pending',  -- pending, deploying, deployed, failed, archived
    last_task_id    INTEGER,                  -- 最后一次部署的任务 ID
    deployed_by     VARCHAR(20) NOT NULL,
    deployed_at     TIMESTAMP,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- 注意：不再使用唯一约束，因为支持废弃后重新部署
    -- 通过代码逻辑检查：同一 Manifest+Workspace 只能有一个活跃部署（status != 'archived'）
    FOREIGN KEY (manifest_id) REFERENCES manifests(id) ON DELETE CASCADE,
    FOREIGN KEY (version_id) REFERENCES manifest_versions(id),
    FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    FOREIGN KEY (deployed_by) REFERENCES users(user_id)
);

CREATE INDEX IF NOT EXISTS idx_manifest_deployments_manifest_id ON manifest_deployments(manifest_id);
CREATE INDEX IF NOT EXISTS idx_manifest_deployments_workspace_id ON manifest_deployments(workspace_id);
CREATE INDEX IF NOT EXISTS idx_manifest_deployments_status ON manifest_deployments(status);

-- 4. manifest_deployment_resources 表（部署资源关联）
CREATE TABLE IF NOT EXISTS manifest_deployment_resources (
    id              VARCHAR(36) PRIMARY KEY,  -- 格式: mdr-{ulid}
    deployment_id   VARCHAR(36) NOT NULL,
    node_id         VARCHAR(50) NOT NULL,     -- manifest 中的节点 ID
    resource_id     INTEGER NOT NULL,         -- workspace_resources.id
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(deployment_id, node_id),
    FOREIGN KEY (deployment_id) REFERENCES manifest_deployments(id) ON DELETE CASCADE,
    FOREIGN KEY (resource_id) REFERENCES workspace_resources(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_manifest_deployment_resources_deployment_id ON manifest_deployment_resources(deployment_id);
CREATE INDEX IF NOT EXISTS idx_manifest_deployment_resources_resource_id ON manifest_deployment_resources(resource_id);

-- 添加注释
COMMENT ON TABLE manifests IS 'Manifest 可视化编排模板（Organization 级别）';
COMMENT ON TABLE manifest_versions IS 'Manifest 版本管理';
COMMENT ON TABLE manifest_deployments IS 'Manifest 部署记录';
COMMENT ON TABLE manifest_deployment_resources IS 'Manifest 部署资源关联';

COMMENT ON COLUMN manifests.status IS 'draft: 草稿, published: 已发布, archived: 已归档';
COMMENT ON COLUMN manifest_versions.canvas_data IS '画布数据（节点位置、缩放、视口等）';
COMMENT ON COLUMN manifest_versions.nodes IS '节点配置（Module 节点、变量节点等）';
COMMENT ON COLUMN manifest_versions.edges IS '连接关系（依赖关系、变量绑定）';
COMMENT ON COLUMN manifest_versions.variables IS '可配置的变量定义（部署时可覆盖）';
COMMENT ON COLUMN manifest_deployments.status IS 'pending: 待部署, deploying: 部署中, deployed: 已部署, failed: 部署失败, archived: 已废弃';
