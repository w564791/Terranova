-- 迁移剩余的 user_id 字段从 integer 到 varchar(20)
-- 需要先处理依赖的视图

BEGIN;

-- 1. 删除依赖 iam_user_roles.user_id 的视图
DROP VIEW IF EXISTS v_user_effective_roles CASCADE;

-- 2. resource_locks 表
ALTER TABLE resource_locks 
    ALTER COLUMN editing_user_id TYPE varchar(20) USING LPAD(editing_user_id::text, 20, '0');

-- 3. resource_drifts 表
ALTER TABLE resource_drifts 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 4. iam_user_roles 表
ALTER TABLE iam_user_roles 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

ALTER TABLE iam_user_roles 
    ALTER COLUMN assigned_by TYPE varchar(20) USING CASE 
        WHEN assigned_by IS NULL THEN NULL 
        ELSE LPAD(assigned_by::text, 20, '0') 
    END;

-- 5. ai_error_analyses 表
ALTER TABLE ai_error_analyses 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 6. ai_analysis_rate_limits 表
ALTER TABLE ai_analysis_rate_limits 
    ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 7. 重建视图 v_user_effective_roles (如果之前存在)
CREATE OR REPLACE VIEW v_user_effective_roles AS
SELECT 
    ur.user_id,
    ur.role_id,
    ur.scope_type,
    ur.scope_id,
    r.name as role_name,
    r.display_name as role_display_name,
    ur.assigned_at,
    ur.expires_at
FROM iam_user_roles ur
JOIN iam_roles r ON ur.role_id = r.id
WHERE r.is_active = true
  AND (ur.expires_at IS NULL OR ur.expires_at > NOW());

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
