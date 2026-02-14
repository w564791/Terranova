-- 迁移 iam_team_roles 表的 team_id 字段从 integer 到 varchar(20)

BEGIN;

ALTER TABLE iam_team_roles 
    ALTER COLUMN team_id TYPE varchar(20) USING LPAD(team_id::text, 20, '0');

COMMIT;

-- 验证结果
SELECT column_name, data_type, character_maximum_length 
FROM information_schema.columns 
WHERE table_name = 'iam_team_roles' AND column_name = 'team_id';
