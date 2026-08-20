-- Application Role 赋值表（D5 / 选项 A）
-- principal 使用 app_key，与 org_permissions / AgentAuth 一致

CREATE TABLE IF NOT EXISTS iam_application_roles (
  id BIGSERIAL PRIMARY KEY,
  application_principal_id VARCHAR(64) NOT NULL,
  role_id INTEGER NOT NULL REFERENCES iam_roles(id),
  scope_type VARCHAR(20) NOT NULL,
  scope_id BIGINT NOT NULL,
  assigned_by VARCHAR(20),
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ,
  reason TEXT
);

CREATE INDEX IF NOT EXISTS idx_app_roles_principal
  ON iam_application_roles (application_principal_id);
CREATE INDEX IF NOT EXISTS idx_app_roles_scope
  ON iam_application_roles (scope_type, scope_id);
-- PostgreSQL forbids NOW() in a partial-index predicate because it is not
-- IMMUTABLE. The application treats a logical assignment as one row, so keep
-- one stable identity and update/revoke that row instead of creating a second
-- row when an expiry changes.
DROP INDEX IF EXISTS uq_app_roles_active;
CREATE UNIQUE INDEX IF NOT EXISTS uq_app_roles_identity
  ON iam_application_roles (application_principal_id, role_id, scope_type, scope_id);

COMMENT ON TABLE iam_application_roles IS
  'Application (app_key) 的 Role 绑定；细粒度 workspace 仍可配合 workspace_tag_filter';
