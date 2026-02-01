-- 迁移权限表的 principal_id 字段从 integer 到 varchar(20)

BEGIN;

-- 1. org_permissions 表
ALTER TABLE org_permissions 
    ALTER COLUMN principal_id TYPE varchar(20) USING LPAD(principal_id::text, 20, '0');

-- 2. project_permissions 表
ALTER TABLE project_permissions 
    ALTER COLUMN principal_id TYPE varchar(20) USING LPAD(principal_id::text, 20, '0');

-- 3. workspace_permissions 表
ALTER TABLE workspace_permissions 
    ALTER COLUMN principal_id TYPE varchar(20) USING LPAD(principal_id::text, 20, '0');

-- 4. permission_audit_logs 表
ALTER TABLE permission_audit_logs 
    ALTER COLUMN principal_id TYPE varchar(20) USING LPAD(principal_id::text, 20, '0');

COMMIT;

-- 验证结果
SELECT table_name, column_name, data_type, character_maximum_length 
FROM information_schema.columns 
WHERE table_name IN ('org_permissions', 'project_permissions', 'workspace_permissions', 'permission_audit_logs') 
  AND column_name = 'principal_id'
ORDER BY table_name;
