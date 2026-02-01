-- 授予admin用户对workspace ws-0yrm628p8h3f9mw0 的ADMIN权限
-- 这将允许admin用户取消任务、确认apply等操作

INSERT INTO workspace_permissions (
    workspace_id,
    principal_type,
    principal_id,
    permission_level,
    granted_by,
    granted_at
) VALUES (
    'ws-0yrm628p8h3f9mw0',
    'USER',
    'user-n8tzt0ldde',  -- admin用户ID
    3,  -- ADMIN权限级别 (1=READ, 2=WRITE, 3=ADMIN)
    'user-n8tzt0ldde',  -- 自己授予自己
    NOW()
) ON CONFLICT (workspace_id, principal_type, principal_id) 
DO UPDATE SET 
    permission_level = 3,
    granted_at = NOW();

-- 验证权限已授予
SELECT * FROM workspace_permissions 
WHERE workspace_id = 'ws-0yrm628p8h3f9mw0' 
AND principal_id = 'user-n8tzt0ldde';
