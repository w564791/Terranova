-- 存量 USER/TEAM Direct Grant → 合成 Role + 赋值（不删除原 grant 行，Checker 双读）
--
-- 安全约束：
--   * 已过期 grant 不迁移；
--   * expires_at 是分组键，避免把临时 grant 变成永久 Role；
--   * 使用完整哈希作为 Role 名，避免截断哈希把无关主体合并到同一 Role；
--   * legacy custom Role 必须写入来源 org_id，绝不默认成平台 Role；
--   * workspace 必须已绑定唯一 project，无法确定 org 的行保留给 Direct Grant
--     路径并在切换前人工处置。
--
-- 前置：先执行 patch_multitenant_workspace_role_org.sql。

BEGIN;

CREATE TEMP TABLE _legacy_dg_groups ON COMMIT DROP AS
SELECT
  principal_type,
  principal_id,
  'ORGANIZATION'::text AS scope_type,
  org_id AS scope_id,
  org_id,
  expires_at,
  md5(principal_type || ':' || principal_id || ':ORGANIZATION:' || org_id::text || ':' || COALESCE(expires_at::text, 'never')) AS ghash
FROM org_permissions
WHERE principal_type IN ('USER', 'TEAM')
  AND (expires_at IS NULL OR expires_at > NOW())
GROUP BY principal_type, principal_id, org_id, expires_at

UNION

SELECT
  pp.principal_type,
  pp.principal_id,
  'PROJECT'::text,
  pp.project_id,
  p.org_id,
  pp.expires_at,
  md5(pp.principal_type || ':' || pp.principal_id || ':PROJECT:' || pp.project_id::text || ':' || COALESCE(pp.expires_at::text, 'never'))
FROM project_permissions pp
JOIN projects p ON p.id = pp.project_id
WHERE pp.principal_type IN ('USER', 'TEAM')
  AND (pp.expires_at IS NULL OR pp.expires_at > NOW())
GROUP BY pp.principal_type, pp.principal_id, pp.project_id, p.org_id, pp.expires_at

UNION

SELECT
  wp.principal_type,
  wp.principal_id,
  'WORKSPACE'::text,
  w.id,
  p.org_id,
  wp.expires_at,
  md5(wp.principal_type || ':' || wp.principal_id || ':WORKSPACE:' || w.id::text || ':' || COALESCE(wp.expires_at::text, 'never'))
FROM workspace_permissions wp
JOIN workspaces w ON w.workspace_id = wp.workspace_id
JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
JOIN projects p ON p.id = wpr.project_id
WHERE wp.principal_type IN ('USER', 'TEAM')
  AND (wp.expires_at IS NULL OR wp.expires_at > NOW())
GROUP BY wp.principal_type, wp.principal_id, w.id, p.org_id, wp.expires_at;

INSERT INTO iam_roles (org_id, name, display_name, description, is_system, is_active, created_at, updated_at)
SELECT
  g.org_id,
  'legacy_dg_' || g.ghash,
  'Legacy Direct Grant',
  'Auto-synthesized from direct grants; principal=' || g.principal_type || '/' || g.principal_id
    || ' scope=' || g.scope_type || '/' || g.scope_id::text,
  false,
  true,
  NOW(),
  NOW()
FROM _legacy_dg_groups g
WHERE NOT EXISTS (
  SELECT 1 FROM iam_roles r
   WHERE r.org_id = g.org_id
     AND r.name = 'legacy_dg_' || g.ghash
);

-- ORG policies
INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at)
SELECT DISTINCT r.id, op.permission_id, op.permission_level, 'ORGANIZATION', NOW()
FROM org_permissions op
JOIN _legacy_dg_groups g
  ON g.principal_type = op.principal_type
 AND g.principal_id = op.principal_id
 AND g.scope_type = 'ORGANIZATION'
 AND g.scope_id = op.org_id
 AND g.expires_at IS NOT DISTINCT FROM op.expires_at
JOIN iam_roles r
  ON r.org_id = g.org_id
 AND r.name = 'legacy_dg_' || g.ghash
WHERE op.principal_type IN ('USER', 'TEAM')
  AND (op.expires_at IS NULL OR op.expires_at > NOW())
  AND NOT EXISTS (
    SELECT 1 FROM iam_role_policies rp
     WHERE rp.role_id = r.id
       AND rp.permission_id = op.permission_id
       AND rp.scope_type = 'ORGANIZATION'
  );

-- PROJECT policies
INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at)
SELECT DISTINCT r.id, pp.permission_id, pp.permission_level, 'PROJECT', NOW()
FROM project_permissions pp
JOIN projects p ON p.id = pp.project_id
JOIN _legacy_dg_groups g
  ON g.principal_type = pp.principal_type
 AND g.principal_id = pp.principal_id
 AND g.scope_type = 'PROJECT'
 AND g.scope_id = pp.project_id
 AND g.org_id = p.org_id
 AND g.expires_at IS NOT DISTINCT FROM pp.expires_at
JOIN iam_roles r
  ON r.org_id = g.org_id
 AND r.name = 'legacy_dg_' || g.ghash
WHERE pp.principal_type IN ('USER', 'TEAM')
  AND (pp.expires_at IS NULL OR pp.expires_at > NOW())
  AND NOT EXISTS (
    SELECT 1 FROM iam_role_policies rp
     WHERE rp.role_id = r.id
       AND rp.permission_id = pp.permission_id
       AND rp.scope_type = 'PROJECT'
  );

-- WORKSPACE policies
INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at)
SELECT DISTINCT r.id, wp.permission_id, wp.permission_level, 'WORKSPACE', NOW()
FROM workspace_permissions wp
JOIN workspaces w ON w.workspace_id = wp.workspace_id
JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
JOIN projects p ON p.id = wpr.project_id
JOIN _legacy_dg_groups g
  ON g.principal_type = wp.principal_type
 AND g.principal_id = wp.principal_id
 AND g.scope_type = 'WORKSPACE'
 AND g.scope_id = w.id
 AND g.org_id = p.org_id
 AND g.expires_at IS NOT DISTINCT FROM wp.expires_at
JOIN iam_roles r
  ON r.org_id = g.org_id
 AND r.name = 'legacy_dg_' || g.ghash
WHERE wp.principal_type IN ('USER', 'TEAM')
  AND (wp.expires_at IS NULL OR wp.expires_at > NOW())
  AND NOT EXISTS (
    SELECT 1 FROM iam_role_policies rp
     WHERE rp.role_id = r.id
       AND rp.permission_id = wp.permission_id
       AND rp.scope_type = 'WORKSPACE'
  );

INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id, assigned_by, assigned_at, expires_at, reason)
SELECT g.principal_id, r.id, g.scope_type, g.scope_id, 'system:legacy_dg', NOW(), g.expires_at,
       'patch_legacy_direct_grants_to_roles'
FROM _legacy_dg_groups g
JOIN iam_roles r
  ON r.org_id = g.org_id
 AND r.name = 'legacy_dg_' || g.ghash
WHERE g.principal_type = 'USER'
  AND NOT EXISTS (
    SELECT 1 FROM iam_user_roles ur
     WHERE ur.user_id = g.principal_id
       AND ur.role_id = r.id
       AND ur.scope_type = g.scope_type
       AND ur.scope_id = g.scope_id
  );

INSERT INTO iam_team_roles (team_id, role_id, scope_type, scope_id, assigned_by, assigned_at, expires_at, reason)
SELECT g.principal_id, r.id, g.scope_type, g.scope_id, 'system:legacy_dg', NOW(), g.expires_at,
       'patch_legacy_direct_grants_to_roles'
FROM _legacy_dg_groups g
JOIN iam_roles r
  ON r.org_id = g.org_id
 AND r.name = 'legacy_dg_' || g.ghash
WHERE g.principal_type = 'TEAM'
  AND NOT EXISTS (
    SELECT 1 FROM iam_team_roles tr
     WHERE tr.team_id = g.principal_id
       AND tr.role_id = r.id
       AND tr.scope_type = g.scope_type
       AND tr.scope_id = g.scope_id
  );

COMMIT;

-- Direct Grant rows remain until a verified, audited cutover removes the
-- legacy evaluator. This script does not silently delete source evidence.
