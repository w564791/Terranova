-- 工作空间权限整合：统一到workspace_management
-- 将所有工作空间权限整合为单一的workspace_management权限

-- 说明：
-- 1. 为所有拥有任何工作空间权限的用户授予相应级别的workspace_management权限
-- 2. 迁移规则：
--    - 任何ADMIN级别 → workspace_management ADMIN
--    - 任何WRITE级别 → workspace_management WRITE  
--    - 只有READ级别 → workspace_management READ
-- 3. 保留旧权限以确保向后兼容（可以在确认无问题后手动删除）

BEGIN;

-- 创建临时表来存储需要迁移的权限
CREATE TEMP TABLE temp_workspace_permission_migration AS
SELECT 
    principal_type,
    principal_id,
    workspace_id,
    -- 确定最高权限级别
    MAX(permission_level) as max_level,
    -- 收集所有权限ID用于审计
    array_agg(DISTINCT permission_id ORDER BY permission_id) as old_permission_ids,
    array_agg(DISTINCT pd.name ORDER BY pd.name) as old_permission_names
FROM workspace_permissions wp
JOIN permission_definitions pd ON wp.permission_id = pd.id
WHERE permission_id IN (8, 9, 10, 11, 24) -- task_data_access, workspace_execution, workspace_state, workspace_variables, workspace_resources
GROUP BY principal_type, principal_id, workspace_id;

-- 查看迁移计划
SELECT 
    principal_type,
    principal_id,
    workspace_id,
    CASE max_level
        WHEN 1 THEN 'READ'
        WHEN 2 THEN 'WRITE'
        WHEN 3 THEN 'ADMIN'
    END as new_level,
    old_permission_names
FROM temp_workspace_permission_migration
ORDER BY workspace_id, principal_id;

-- 执行迁移：为每个用户授予workspace_management权限
INSERT INTO workspace_permissions (principal_type, principal_id, permission_id, workspace_id, permission_level, granted_by, granted_at, reason)
SELECT 
    principal_type,
    principal_id,
    26, -- workspace_management的ID
    workspace_id,
    max_level, -- 使用最高权限级别
    1, -- 系统管理员
    NOW(),
    '权限整合：从 ' || array_to_string(old_permission_names, ', ') || ' 迁移到 workspace_management'
FROM temp_workspace_permission_migration
WHERE NOT EXISTS (
    SELECT 1 FROM workspace_permissions wp2
    WHERE wp2.principal_type = temp_workspace_permission_migration.principal_type
      AND wp2.principal_id = temp_workspace_permission_migration.principal_id
      AND wp2.workspace_id = temp_workspace_permission_migration.workspace_id
      AND wp2.permission_id = 26
);

-- 查看迁移结果统计
SELECT 
    '迁移完成' as status,
    COUNT(*) as migrated_count,
    COUNT(CASE WHEN permission_level = 1 THEN 1 END) as read_count,
    COUNT(CASE WHEN permission_level = 2 THEN 1 END) as write_count,
    COUNT(CASE WHEN permission_level = 3 THEN 1 END) as admin_count
FROM workspace_permissions
WHERE permission_id = 26
  AND reason LIKE '权限整合：%';

-- 查看迁移前后的权限对比
SELECT 
    pd.name as permission_name,
    pd.display_name,
    COUNT(*) as count,
    COUNT(CASE WHEN wp.permission_level = 1 THEN 1 END) as read_count,
    COUNT(CASE WHEN wp.permission_level = 2 THEN 1 END) as write_count,
    COUNT(CASE WHEN wp.permission_level = 3 THEN 1 END) as admin_count
FROM workspace_permissions wp
JOIN permission_definitions pd ON wp.permission_id = pd.id
WHERE pd.name IN ('task_data_access', 'workspace_execution', 'workspace_state', 'workspace_variables', 'workspace_resources', 'workspace_management')
GROUP BY pd.name, pd.display_name
ORDER BY pd.name;

-- 清理临时表
DROP TABLE temp_workspace_permission_migration;

COMMIT;

-- ============================================
-- 可选：在确认迁移成功且系统运行正常后执行
-- 建议保留至少1-2周的观察期
-- ============================================

-- BEGIN;
-- 
-- -- 备份旧权限到审计表（如果需要）
-- INSERT INTO permission_audit_logs (
--     action, principal_type, principal_id, permission_id, 
--     scope_type, scope_id, old_level, new_level, 
--     performed_by, performed_at, reason
-- )
-- SELECT 
--     'REVOKE' as action,
--     principal_type,
--     principal_id,
--     permission_id,
--     'WORKSPACE' as scope_type,
--     workspace_id as scope_id,
--     permission_level as old_level,
--     NULL as new_level,
--     1 as performed_by,
--     NOW() as performed_at,
--     '权限整合清理：移除旧的工作空间权限' as reason
-- FROM workspace_permissions
-- WHERE permission_id IN (8, 9, 10, 11, 24);
-- 
-- -- 删除旧的工作空间权限
-- DELETE FROM workspace_permissions 
-- WHERE permission_id IN (
--     8,  -- task_data_access
--     9,  -- workspace_execution
--     10, -- workspace_state
--     11, -- workspace_variables
--     24  -- workspace_resources
-- );
-- 
-- -- 查看清理结果
-- SELECT 
--     '清理完成' as status,
--     (SELECT COUNT(*) FROM workspace_permissions WHERE permission_id = 26) as remaining_workspace_management_count,
--     (SELECT COUNT(*) FROM workspace_permissions WHERE permission_id IN (8, 9, 10, 11, 24)) as remaining_old_permissions_count;
-- 
-- COMMIT;

-- ============================================
-- 可选：标记旧权限为已废弃
-- ============================================

-- UPDATE permission_definitions
-- SET description = '[已废弃] ' || description
-- WHERE id IN (8, 9, 10, 11, 24)
--   AND description NOT LIKE '[已废弃]%';
