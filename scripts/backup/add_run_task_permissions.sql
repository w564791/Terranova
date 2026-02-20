-- Add Run Task Management Permissions
-- These permissions are for managing Run Tasks at ORGANIZATION scope level

-- Insert Run Task management permissions
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    -- Run Task 管理权限（组织级别）
    ('orgpm-rt-create-8k4m', 'RUN_TASKS', 'RUN_TASKS', 'ORGANIZATION', 'Run Task 管理', '管理 Run Task 配置（创建、查看、更新、删除）', true, CURRENT_TIMESTAMP)
ON CONFLICT (id) DO NOTHING;

-- Also insert with name conflict check
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('orgpm-rt-create-8k4m', 'RUN_TASKS', 'RUN_TASKS', 'ORGANIZATION', 'Run Task 管理', '管理 Run Task 配置（创建、查看、更新、删除）', true, CURRENT_TIMESTAMP)
ON CONFLICT (name) DO NOTHING;

-- Verification
SELECT '=== 新增的 Run Task 权限 ===' as info;
SELECT id, name, resource_type, scope_level, display_name, description 
FROM permission_definitions 
WHERE name = 'RUN_TASKS' 
ORDER BY name;
