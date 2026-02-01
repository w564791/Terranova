-- 阶段6: 切换到语义ID
-- 警告: 此操作会修改表结构，已经执行了阶段1-5的准备工作

-- 6.1 删除旧的外键约束
ALTER TABLE org_permissions DROP CONSTRAINT IF EXISTS org_permissions_permission_id_fkey;
ALTER TABLE project_permissions DROP CONSTRAINT IF EXISTS project_permissions_permission_id_fkey;
ALTER TABLE workspace_permissions DROP CONSTRAINT IF EXISTS workspace_permissions_permission_id_fkey;
ALTER TABLE iam_role_policies DROP CONSTRAINT IF EXISTS iam_role_policies_permission_id_fkey;
ALTER TABLE permission_audit_logs DROP CONSTRAINT IF EXISTS permission_audit_logs_permission_id_fkey;
ALTER TABLE preset_permissions DROP CONSTRAINT IF EXISTS preset_permissions_permission_id_fkey;

-- 6.2 删除permission_definitions的旧主键
ALTER TABLE permission_definitions DROP CONSTRAINT IF EXISTS permission_definitions_pkey;

-- 6.3 重命名字段
ALTER TABLE permission_definitions RENAME COLUMN id TO old_id;
ALTER TABLE permission_definitions RENAME COLUMN semantic_id TO id;

ALTER TABLE org_permissions RENAME COLUMN permission_id TO old_permission_id;
ALTER TABLE org_permissions RENAME COLUMN new_permission_id TO permission_id;

ALTER TABLE project_permissions RENAME COLUMN permission_id TO old_permission_id;
ALTER TABLE project_permissions RENAME COLUMN new_permission_id TO permission_id;

ALTER TABLE workspace_permissions RENAME COLUMN permission_id TO old_permission_id;
ALTER TABLE workspace_permissions RENAME COLUMN new_permission_id TO permission_id;

ALTER TABLE iam_role_policies RENAME COLUMN permission_id TO old_permission_id;
ALTER TABLE iam_role_policies RENAME COLUMN new_permission_id TO permission_id;

ALTER TABLE permission_audit_logs RENAME COLUMN permission_id TO old_permission_id;
ALTER TABLE permission_audit_logs RENAME COLUMN new_permission_id TO permission_id;

ALTER TABLE preset_permissions RENAME COLUMN permission_id TO old_permission_id;
ALTER TABLE preset_permissions RENAME COLUMN new_permission_id TO permission_id;

-- 6.4 添加新的主键
ALTER TABLE permission_definitions ADD PRIMARY KEY (id);

-- 6.5 添加新的外键约束
ALTER TABLE org_permissions ADD CONSTRAINT org_permissions_permission_id_fkey 
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

ALTER TABLE project_permissions ADD CONSTRAINT project_permissions_permission_id_fkey 
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

ALTER TABLE workspace_permissions ADD CONSTRAINT workspace_permissions_permission_id_fkey 
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

ALTER TABLE iam_role_policies ADD CONSTRAINT iam_role_policies_permission_id_fkey 
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

ALTER TABLE permission_audit_logs ADD CONSTRAINT permission_audit_logs_permission_id_fkey 
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

ALTER TABLE preset_permissions ADD CONSTRAINT preset_permissions_permission_id_fkey 
    FOREIGN KEY (permission_id) REFERENCES permission_definitions(id);

-- 验证结果
SELECT 'permission_definitions' as table_name, id, name FROM permission_definitions LIMIT 5;
SELECT 'org_permissions' as table_name, id, permission_id FROM org_permissions LIMIT 5;
