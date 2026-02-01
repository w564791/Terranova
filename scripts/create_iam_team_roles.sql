-- 创建团队角色分配表
CREATE TABLE IF NOT EXISTS iam_team_roles (
    id SERIAL PRIMARY KEY,
    team_id INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
    scope_type VARCHAR(50) NOT NULL,
    scope_id INTEGER NOT NULL,
    assigned_by INTEGER REFERENCES users(id),
    assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP,
    reason TEXT,
    CONSTRAINT iam_team_roles_team_id_role_id_scope_type_scope_id_key 
        UNIQUE (team_id, role_id, scope_type, scope_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_iam_team_roles_team_id ON iam_team_roles(team_id);
CREATE INDEX IF NOT EXISTS idx_iam_team_roles_role_id ON iam_team_roles(role_id);
CREATE INDEX IF NOT EXISTS idx_iam_team_roles_scope ON iam_team_roles(scope_type, scope_id);

-- 添加注释
COMMENT ON TABLE iam_team_roles IS '团队角色分配表';
COMMENT ON COLUMN iam_team_roles.team_id IS '团队ID';
COMMENT ON COLUMN iam_team_roles.role_id IS '角色ID';
COMMENT ON COLUMN iam_team_roles.scope_type IS '作用域类型：ORGANIZATION/PROJECT/WORKSPACE';
COMMENT ON COLUMN iam_team_roles.scope_id IS '作用域ID';
COMMENT ON COLUMN iam_team_roles.assigned_by IS '分配人ID';
COMMENT ON COLUMN iam_team_roles.assigned_at IS '分配时间';
COMMENT ON COLUMN iam_team_roles.expires_at IS '过期时间';
COMMENT ON COLUMN iam_team_roles.reason IS '分配原因';
