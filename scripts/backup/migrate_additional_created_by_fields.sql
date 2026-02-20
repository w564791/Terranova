-- 迁移 workspace_tasks, workspaces, modules 表的 created_by 字段

BEGIN;

-- 1. workspace_tasks 表
ALTER TABLE workspace_tasks 
    ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 2. workspaces 表
ALTER TABLE workspaces 
    ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 3. modules 表
ALTER TABLE modules 
    ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

COMMIT;

-- 验证结果
SELECT table_name, column_name, data_type, character_maximum_length 
FROM information_schema.columns 
WHERE table_name IN ('workspace_tasks', 'workspaces', 'modules') 
  AND column_name = 'created_by'
ORDER BY table_name;
