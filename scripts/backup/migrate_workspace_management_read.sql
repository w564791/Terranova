-- 工作空间权限体系优化：统一只读权限迁移
-- 将所有查看操作统一到workspace_management READ权限下

-- 说明：
-- 1. 为所有拥有workspace_variables/workspace_state/workspace_resources READ权限的用户
--    授予workspace_management READ权限
-- 2. 保留旧的READ权限以确保向后兼容（可以在确认无问题后手动删除）

BEGIN;

-- 1. 为所有拥有workspace_variables READ的用户授予workspace_management READ
INSERT INTO workspace_permissions (principal_type, principal_id, permission_id, workspace_id, permission_level, granted_by, granted_at, reason)
SELECT 
    principal_type,
    principal_id,
    26, -- workspace_management的ID
    workspace_id,
    1, -- READ级别
    1, -- 系统管理员
    NOW(),
    '权限体系优化：统一只读权限迁移（从workspace_variables READ）'
FROM workspace_permissions
WHERE permission_id = 11 -- workspace_variables
  AND permission_level = 1 -- READ
  AND NOT EXISTS (
    SELECT 1 FROM workspace_permissions wp2
    WHERE wp2.principal_type = workspace_permissions.principal_type
      AND wp2.principal_id = workspace_permissions.principal_id
      AND wp2.workspace_id = workspace_permissions.workspace_id
      AND wp2.permission_id = 26
  );

-- 2. 为所有拥有workspace_state READ的用户授予workspace_management READ
INSERT INTO workspace_permissions (principal_type, principal_id, permission_id, workspace_id, permission_level, granted_by, granted_at, reason)
SELECT 
    principal_type,
    principal_id,
    26, -- workspace_management的ID
    workspace_id,
    1, -- READ级别
    1, -- 系统管理员
    NOW(),
    '权限体系优化：统一只读权限迁移（从workspace_state READ）'
FROM workspace_permissions
WHERE permission_id = 10 -- workspace_state
  AND permission_level = 1 -- READ
  AND NOT EXISTS (
    SELECT 1 FROM workspace_permissions wp2
    WHERE wp2.principal_type = workspace_permissions.principal_type
      AND wp2.principal_id = workspace_permissions.principal_id
      AND wp2.workspace_id = workspace_permissions.workspace_id
      AND wp2.permission_id = 26
  );

-- 3. 为所有拥有workspace_resources READ的用户授予workspace_management READ
INSERT INTO workspace_permissions (principal_type, principal_id, permission_id, workspace_id, permission_level, granted_by, granted_at, reason)
SELECT 
    principal_type,
    principal_id,
    26, -- workspace_management的ID
    workspace_id,
    1, -- READ级别
    1, -- 系统管理员
    NOW(),
    '权限体系优化：统一只读权限迁移（从workspace_resources READ）'
FROM workspace_permissions
WHERE permission_id = 24 -- workspace_resources
  AND permission_level = 1 -- READ
  AND NOT EXISTS (
    SELECT 1 FROM workspace_permissions wp2
    WHERE wp2.principal_type = workspace_permissions.principal_type
      AND wp2.principal_id = workspace_permissions.principal_id
      AND wp2.workspace_id = workspace_permissions.workspace_id
      AND wp2.permission_id = 26
  );

-- 查看迁移结果
SELECT 
    '迁移完成' as status,
    COUNT(*) as new_permissions_count
FROM workspace_permissions
WHERE permission_id = 26
  AND permission_level = 1
  AND reason LIKE '权限体系优化：统一只读权限迁移%';

-- 查看迁移前后的权限对比
SELECT 
    pd.name as permission_name,
    pd.display_name,
    COUNT(*) as count
FROM workspace_permissions wp
JOIN permission_definitions pd ON wp.permission_id = pd.id
WHERE wp.permission_level = 1 -- READ级别
  AND pd.name IN ('workspace_variables', 'workspace_state', 'workspace_resources', 'workspace_management')
GROUP BY pd.name, pd.display_name
ORDER BY pd.name;

COMMIT;

-- 可选：在确认迁移成功且系统运行正常后，可以删除旧的READ权限
-- 建议保留至少1-2周的观察期
-- 
-- BEGIN;
-- 
-- -- 删除workspace_variables READ权限
-- DELETE FROM workspace_permissions 
-- WHERE permission_id = 11 -- workspace_variables
--   AND permission_level = 1; -- READ
-- 
-- -- 删除workspace_state READ权限
-- DELETE FROM workspace_permissions 
-- WHERE permission_id = 10 -- workspace_state
--   AND permission_level = 1; -- READ
-- 
-- -- 删除workspace_resources READ权限
-- DELETE FROM workspace_permissions 
-- WHERE permission_id = 24 -- workspace_resources
--   AND permission_level = 1; -- READ
-- 
-- COMMIT;
