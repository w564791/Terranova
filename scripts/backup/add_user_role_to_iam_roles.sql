-- 添加 "user" (普通用户) 角色到 iam_roles 表
-- 该角色对应 users.role 字段中的 "user" 值，表示普通用户（无管理员权限）
INSERT INTO iam_roles (name, display_name, description, is_system, is_active, created_at, updated_at)
VALUES ('user', '普通用户', '默认普通用户角色，无管理员权限', true, true, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;