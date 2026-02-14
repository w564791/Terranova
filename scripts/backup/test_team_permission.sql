-- 测试ken用户的团队权限继承

-- 1. 确认ken在ops团队
SELECT 'Ken in ops team:' as check_point, * FROM team_members WHERE user_id = 3 AND team_id = 12;

-- 2. 确认ops团队的角色
SELECT 'Ops team roles:' as check_point, tr.*, r.name, r.display_name 
FROM iam_team_roles tr 
JOIN iam_roles r ON tr.role_id = r.id 
WHERE tr.team_id = 12;

-- 3. 确认运维角色的WORKSPACE_EXECUTION策略
SELECT 'Ops role WORKSPACE_EXECUTION policy:' as check_point, rp.*, pd.display_name, pd.resource_type
FROM iam_role_policies rp
JOIN permission_definitions pd ON rp.permission_id = pd.id
WHERE rp.role_id = 25 AND pd.resource_type = 'WORKSPACE_EXECUTION';

-- 4. 检查ken用户是否有直接的WORKSPACE_EXECUTION权限
SELECT 'Ken direct permissions:' as check_point, COUNT(*) as count
FROM org_permissions 
WHERE principal_type = 'USER' AND principal_id = 3;

-- 5. 检查ken用户是否有直接的角色
SELECT 'Ken direct roles:' as check_point, COUNT(*) as count
FROM iam_user_roles
WHERE user_id = 3;

-- 总结
SELECT 
  'Summary' as check_point,
  (SELECT COUNT(*) FROM team_members WHERE user_id = 3) as is_in_team,
  (SELECT COUNT(*) FROM iam_team_roles WHERE team_id = 12) as team_roles_count,
  (SELECT COUNT(*) FROM iam_role_policies WHERE role_id = 25 AND permission_id LIKE '%WORKSPACE_EXECUTION%') as ops_role_has_exec_perm;
