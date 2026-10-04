-- ============================================================
-- Manifest IAM: 注册组织级 MANIFESTS 资源 + 内置角色策略
-- 创建日期: 2026-10-04
-- 背景:
--   router_manifest.go 此前用 SYSTEM_SETTINGS 临时代替 Manifest 权限。
--   现改为专用 MANIFESTS (ORGANIZATION) 资源；未注册定义/策略会导致所有人 403。
--   VARIABLE_SETS 已被路由使用但从未注册到 permission_definitions，一并补齐
--   (镜像 RUN_TASKS)，否则 manifest 部署选择变量集一步不可达。
-- 策略 (均 @ ORGANIZATION，镜像 MODULES / RUN_TASKS):
--   MANIFESTS:     admin/org_admin ADMIN; developer/viewer/user READ
--   VARIABLE_SETS: admin/org_admin ADMIN; viewer READ
-- 幂等 + 仅新增: 所有 INSERT 使用 WHERE NOT EXISTS，不修改/删除已有行。
-- 自动执行: cmd/migrate 版本 20261004_01_manifest_iam_resource
--   (internal/migration/manifests_iam.go)，本文件供手工 psql 执行，内容需保持一致。
-- ============================================================

\connect iac_platform

BEGIN;

INSERT INTO public.permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
SELECT 'orgpm-manifests', 'MANIFESTS', 'MANIFESTS', 'ORGANIZATION', 'Manifest 管理', '管理组织下的 Manifest 目录（查看、编辑/发布、删除/归档）；部署另需目标 workspace 的 WORKSPACE_RESOURCES WRITE', true, NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM public.permission_definitions
    WHERE id = 'orgpm-manifests' OR name = 'MANIFESTS' OR (resource_type = 'MANIFESTS' AND scope_level = 'ORGANIZATION')
);

INSERT INTO public.permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
SELECT 'orgpm-variable-sets', 'VARIABLE_SETS', 'VARIABLE_SETS', 'ORGANIZATION', '变量集管理', '管理组织变量集（查看、编辑、删除、分配）', true, NOW()
WHERE NOT EXISTS (
    SELECT 1 FROM public.permission_definitions
    WHERE id = 'orgpm-variable-sets' OR name = 'VARIABLE_SETS' OR (resource_type = 'VARIABLE_SETS' AND scope_level = 'ORGANIZATION')
);

INSERT INTO public.iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id, pd.id, g.level, 'ORGANIZATION', NOW()
FROM (VALUES
    ('ADMIN', 'MANIFESTS', 'admin'),
    ('ADMIN', 'MANIFESTS', 'org_admin'),
    ('READ', 'MANIFESTS', 'developer'),
    ('READ', 'MANIFESTS', 'viewer'),
    ('READ', 'MANIFESTS', 'user'),
    ('ADMIN', 'VARIABLE_SETS', 'admin'),
    ('ADMIN', 'VARIABLE_SETS', 'org_admin'),
    ('READ', 'VARIABLE_SETS', 'viewer')
) AS g(level, resource_type, role_name)
JOIN public.iam_roles r ON r.name = g.role_name AND r.is_system = true AND r.org_id = 0
JOIN public.permission_definitions pd ON pd.resource_type = g.resource_type AND pd.scope_level = 'ORGANIZATION'
WHERE NOT EXISTS (
    SELECT 1 FROM public.iam_role_policies rp
    WHERE rp.role_id = r.id AND rp.permission_id = pd.id AND rp.scope_type = 'ORGANIZATION'
);

COMMIT;
