-- ============================================================
-- Patch: system_admin 用户补齐组织级 IAM Role
-- 日期: 2026-07-16
-- 背景:
--   业务 API 已取消 is_system_admin 中间件旁路（见 docs/iam/32）。
--   仅有 is_system_admin=true、未绑定 IAM Role 的账号会对业务接口 403。
-- 行为:
--   1) 为每个活跃 system_admin 用户，在每个活跃组织上绑定系统 Role `admin`
--      （与 setup 初始化一致；admin 含业务+IAM 管理策略）
--   2) 幂等：已存在相同 (user_id, role_id, scope_type, scope_id) 则跳过
--   3) 不修改 is_system_admin 标志（平台 API 仍用 RequireSystemAdmin）
-- 使用:
--   psql -U <user> -d iac_platform -f backend/migrations/patch_system_admin_iam_roles.sql
--   或在事务中手动 \i
-- ============================================================

-- 默认库名 iac_platform；若用其它库名，请去掉下一行并用: psql -d <dbname> -f ...
\connect iac_platform

BEGIN;

-- 前置检查：系统 admin 角色必须存在
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM iam_roles WHERE name = 'admin' AND is_system = true AND org_id = 0) THEN
    RAISE EXCEPTION 'system role admin not found in iam_roles; seed roles first';
  END IF;
END $$;

-- 诊断：执行前统计
SELECT
  (SELECT COUNT(*) FROM users WHERE is_system_admin = true AND is_active = true) AS active_system_admins,
  (SELECT COUNT(*) FROM organizations WHERE is_active = true OR is_active IS NULL) AS orgs,
  (SELECT id FROM iam_roles WHERE name = 'admin' AND is_system = true AND org_id = 0 LIMIT 1) AS admin_role_id;

-- 主补丁：system_admin × 每个组织 → admin@ORGANIZATION
INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id, assigned_by, assigned_at, reason)
SELECT
  u.user_id,
  r.id,
  'ORGANIZATION',
  o.id,
  u.user_id,
  NOW(),
  'patch_system_admin_iam_roles: restore business access after removing system_admin IAM bypass'
FROM users u
CROSS JOIN organizations o
CROSS JOIN iam_roles r
WHERE u.is_system_admin = true
  AND u.is_active = true
  AND r.name = 'admin'
  AND r.is_system = true
  AND r.org_id = 0
  AND (o.is_active = true OR o.is_active IS NULL)
  AND NOT EXISTS (
    SELECT 1
    FROM iam_user_roles ur
    WHERE ur.user_id = u.user_id
      AND ur.role_id = r.id
      AND ur.scope_type = 'ORGANIZATION'
      AND ur.scope_id = o.id
  );

-- 可选：若 users 表用 id 列而非仅 user_id 语义字段，兼容部分环境
-- （user_id 为空时用 CAST(id) 不可行，语义 ID 必须存在）
-- 跳过 user_id 为空的脏数据
-- （上面 INSERT 已依赖 u.user_id）

-- 诊断：执行后
SELECT
  u.user_id,
  u.username,
  u.is_system_admin,
  COUNT(ur.id) AS org_admin_role_bindings
FROM users u
LEFT JOIN iam_user_roles ur
  ON ur.user_id = u.user_id
 AND ur.scope_type = 'ORGANIZATION'
 AND ur.role_id = (SELECT id FROM iam_roles WHERE name = 'admin' AND is_system = true AND org_id = 0 LIMIT 1)
WHERE u.is_system_admin = true
GROUP BY u.user_id, u.username, u.is_system_admin
ORDER BY u.user_id;

COMMIT;

-- ============================================================
-- 回滚参考（勿默认执行）:
-- DELETE FROM iam_user_roles
-- WHERE reason LIKE 'patch_system_admin_iam_roles:%';
-- ============================================================
