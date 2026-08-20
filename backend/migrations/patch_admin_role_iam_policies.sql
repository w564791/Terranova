-- ============================================================
-- Patch: 超级管理员角色 admin 补齐全部 IAM_* 策略
-- 日期: 2026-07-17
-- 背景:
--   业务 API 已取消 is_system_admin 旁路。
--   历史上 admin 角色 DB 策略不全，仅靠代码 bypass；导致 is_system_admin
--   用户虽绑定 admin@ORGANIZATION 仍无法访问 IAM 权限管理
--   （路由要求 IAM_PERMISSIONS/READ 等）。
-- 行为:
--   为系统角色 admin 幂等插入全部 IAM_* 资源 ORGANIZATION/ADMIN 策略。
-- 使用:
--   docker exec -e PGPASSWORD=... iac-platform-postgres \
--     psql -U postgres -d iac_platform -f /tmp/patch_admin_role_iam_policies.sql
-- ============================================================

\connect iac_platform

BEGIN;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM iam_roles WHERE name = 'admin' AND is_system = true AND org_id = 0) THEN
    RAISE EXCEPTION 'system role admin not found';
  END IF;
END $$;

-- 诊断：补齐前 admin 的 IAM 策略
SELECT pd.resource_type, rp.permission_level, rp.scope_type
FROM iam_role_policies rp
JOIN iam_roles r ON r.id = rp.role_id
LEFT JOIN permission_definitions pd ON pd.id = rp.permission_id
WHERE r.name = 'admin' AND r.is_system = true AND r.org_id = 0 AND COALESCE(pd.resource_type, '') LIKE 'IAM%'
ORDER BY pd.resource_type;

-- 为 admin 补齐全部 IAM_* @ ORGANIZATION / ADMIN
INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id, pd.id, 'ADMIN', 'ORGANIZATION', NOW()
FROM iam_roles r
CROSS JOIN permission_definitions pd
WHERE r.name = 'admin'
  AND r.is_system = true
  AND r.org_id = 0
  AND pd.resource_type LIKE 'IAM_%'
  AND pd.scope_level = 'ORGANIZATION'
  AND NOT EXISTS (
    SELECT 1 FROM iam_role_policies rp
    WHERE rp.role_id = r.id
      AND rp.permission_id = pd.id
      AND rp.scope_type = 'ORGANIZATION'
  );

-- 诊断：补齐后
SELECT pd.resource_type, rp.permission_level, rp.scope_type, rp.permission_id
FROM iam_role_policies rp
JOIN iam_roles r ON r.id = rp.role_id
LEFT JOIN permission_definitions pd ON pd.id = rp.permission_id
WHERE r.name = 'admin' AND r.is_system = true AND r.org_id = 0 AND COALESCE(pd.resource_type, '') LIKE 'IAM%'
ORDER BY pd.resource_type;

-- 抽样：user-7ca2psf2ub 是否经 admin 角色持有 IAM_PERMISSIONS
SELECT u.user_id, u.username, r.name AS role_name, pd.resource_type, rp.permission_level
FROM users u
JOIN iam_user_roles ur ON ur.user_id = u.user_id
JOIN iam_roles r ON r.id = ur.role_id
JOIN iam_role_policies rp ON rp.role_id = r.id AND rp.scope_type = ur.scope_type
JOIN permission_definitions pd ON pd.id = rp.permission_id
WHERE u.user_id = 'user-7ca2psf2ub'
  AND pd.resource_type = 'IAM_PERMISSIONS';

COMMIT;
