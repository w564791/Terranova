-- Add Agent Pool Management Permissions
-- These permissions are under APPLICATION resource type at ORGANIZATION scope level

-- Insert agent pool management permissions
INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    -- Agent Pool 管理权限
    ('orgpm-7k3m9p2x5w8q', 'agent_pool_create', 'APPLICATION_REGISTRATION', 'ORGANIZATION', 'Agent Pool 创建', '创建 Agent Pool', true, CURRENT_TIMESTAMP),
    ('orgpm-4n6r8t1v3z7c', 'agent_pool_read', 'APPLICATION_REGISTRATION', 'ORGANIZATION', 'Agent Pool 查看', '查看 Agent Pool 列表和详情', true, CURRENT_TIMESTAMP),
    ('orgpm-2h5j9m4p8s6w', 'agent_pool_update', 'APPLICATION_REGISTRATION', 'ORGANIZATION', 'Agent Pool 更新', '更新 Agent Pool 配置', true, CURRENT_TIMESTAMP),
    ('orgpm-9x2k7n3q5t8v', 'agent_pool_delete', 'APPLICATION_REGISTRATION', 'ORGANIZATION', 'Agent Pool 删除', '删除 Agent Pool', true, CURRENT_TIMESTAMP),
    
    -- Agent 管理权限
    ('orgpm-6w4m8r2p5k9n', 'agent_read', 'APPLICATION_REGISTRATION', 'ORGANIZATION', 'Agent 查看', '查看 Agent 列表和状态', true, CURRENT_TIMESTAMP),
    ('orgpm-3t7v1x4z8c2h', 'agent_manage', 'APPLICATION_REGISTRATION', 'ORGANIZATION', 'Agent 管理', '管理 Agent 注册和注销', true, CURRENT_TIMESTAMP)
ON CONFLICT (name) DO NOTHING;

-- Verification
SELECT '=== 新增的 Agent Pool 权限 ===' as info;
SELECT name, resource_type, scope_level, display_name, description 
FROM permission_definitions 
WHERE name LIKE 'agent_%' 
ORDER BY name;
