-- 迁移权限表的 granted_by 字段

BEGIN;

ALTER TABLE org_permissions 
    ALTER COLUMN granted_by TYPE varchar(20) USING LPAD(granted_by::text, 20, '0');

ALTER TABLE project_permissions 
    ALTER COLUMN granted_by TYPE varchar(20) USING LPAD(granted_by::text, 20, '0');

ALTER TABLE workspace_permissions 
    ALTER COLUMN granted_by TYPE varchar(20) USING LPAD(granted_by::text, 20, '0');

COMMIT;

-- 验证
SELECT table_name, column_name, data_type 
FROM information_schema.columns 
WHERE column_name = 'granted_by' 
  AND table_name IN ('org_permissions', 'project_permissions', 'workspace_permissions')
ORDER BY table_name;
