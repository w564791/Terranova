-- 添加 WORKSPACE_STATE_SENSITIVE 权限定义
-- 用于控制是否可以查看 State 文件的完整内容（包含敏感数据）

INSERT INTO permission_definitions (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
VALUES 
    ('wspm-workspace-state-sensitive', 'WORKSPACE_STATE_SENSITIVE', 'WORKSPACE_STATE_SENSITIVE', 'WORKSPACE', 
     'State 内容查看', '查看 State 文件的完整内容（包含敏感数据）', true, NOW())
ON CONFLICT (id) DO NOTHING;

-- 验证插入结果
SELECT id, name, resource_type, scope_level, display_name, description 
FROM permission_definitions 
WHERE id = 'wspm-workspace-state-sensitive';