-- 确保系统角色 workspace_admin 存在（按 name，不依赖固定 id）
-- 供创建 Workspace 时绑定创建者使用

INSERT INTO iam_roles (org_id, name, display_name, description, is_system, is_active, created_at, updated_at)
SELECT
	0,
  'workspace_admin',
  'Workspace Admin',
  '工作空间管理员：管理执行/状态/变量/资源/任务数据',
  true,
  true,
  NOW(),
  NOW()
WHERE NOT EXISTS (
  SELECT 1 FROM iam_roles WHERE name = 'workspace_admin' AND is_system = true AND org_id = 0
);

-- 工作空间级 ADMIN 策略（permission_id 以现网定义为准；不存在则跳过）
INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id, pd.id, 'ADMIN', 'WORKSPACE', NOW()
FROM iam_roles r
CROSS JOIN permission_definitions pd
WHERE r.name = 'workspace_admin' AND r.is_system = true AND r.org_id = 0
  AND pd.resource_type IN (
    'WORKSPACE_MANAGEMENT',
    'WORKSPACE_EXECUTION',
    'WORKSPACE_STATE',
    'WORKSPACE_VARIABLES',
    'WORKSPACE_RESOURCES',
    'TASK_DATA_ACCESS'
  )
  AND NOT EXISTS (
    SELECT 1 FROM iam_role_policies rp
    WHERE rp.role_id = r.id AND rp.permission_id = pd.id AND rp.scope_type = 'WORKSPACE'
  );

-- 敏感状态只读（若定义存在）
INSERT INTO iam_role_policies (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id, pd.id, 'READ', 'WORKSPACE', NOW()
FROM iam_roles r
CROSS JOIN permission_definitions pd
WHERE r.name = 'workspace_admin' AND r.is_system = true AND r.org_id = 0
  AND pd.id = 'wspm-workspace-state-sensitive'
  AND NOT EXISTS (
    SELECT 1 FROM iam_role_policies rp
    WHERE rp.role_id = r.id AND rp.permission_id = pd.id AND rp.scope_type = 'WORKSPACE'
  );
