-- 迁移 iam_team_roles 表的 assigned_by 字段

BEGIN;

ALTER TABLE iam_team_roles 
    ALTER COLUMN assigned_by TYPE varchar(20) USING LPAD(assigned_by::text, 20, '0');

COMMIT;

-- 验证结果
SELECT column_name, data_type, character_maximum_length 
FROM information_schema.columns 
WHERE table_name = 'iam_team_roles' AND column_name = 'assigned_by';
