-- 迁移剩余的 user_id 字段从 integer 到 varchar(20)
-- 影响表: resource_locks, resource_drifts, iam_user_roles, ai_error_analyses, ai_analysis_rate_limits

BEGIN;

-- 1. resource_locks 表
-- 修改 editing_user_id 字段类型
ALTER TABLE resource_locks 
    ALTER COLUMN editing_user_id TYPE varchar(20) USING LPAD(editing_user_id::text, 20, '0');

-- 2. resource_drifts 表
-- 修改 user_id 字段类型
ALTER TABLE resource_drifts 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 3. iam_user_roles 表
-- 修改 user_id 和 assigned_by 字段类型
ALTER TABLE iam_user_roles 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

ALTER TABLE iam_user_roles 
    ALTER COLUMN assigned_by TYPE varchar(20) USING CASE 
        WHEN assigned_by IS NULL THEN NULL 
        ELSE LPAD(assigned_by::text, 20, '0') 
    END;

-- 4. ai_error_analyses 表
-- 修改 user_id 字段类型
ALTER TABLE ai_error_analyses 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 5. ai_analysis_rate_limits 表
-- 修改 user_id 字段类型
ALTER TABLE ai_analysis_rate_limits 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

COMMIT;

-- 验证迁移结果
SELECT 
    table_name, 
    column_name, 
    data_type, 
    character_maximum_length 
FROM information_schema.columns 
WHERE table_name IN ('resource_locks', 'resource_drifts', 'iam_user_roles', 'ai_error_analyses', 'ai_analysis_rate_limits') 
    AND column_name IN ('user_id', 'editing_user_id', 'assigned_by')
ORDER BY table_name, column_name;
