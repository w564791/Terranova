-- 批量迁移所有剩余的 user 相关 integer 字段

BEGIN;

-- 1. access_logs
ALTER TABLE access_logs ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 2. agent_pools
ALTER TABLE agent_pools ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 3. ai_parse_tasks
ALTER TABLE ai_parse_tasks ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 4. applications
ALTER TABLE applications ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 5. audit_logs
ALTER TABLE audit_logs ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 6. deployments
ALTER TABLE deployments ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 7. iam_roles
ALTER TABLE iam_roles ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 8. module_demo_versions
ALTER TABLE module_demo_versions ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 9. module_demos
ALTER TABLE module_demos ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 10. organizations
ALTER TABLE organizations ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 11. permission_audit_logs
ALTER TABLE permission_audit_logs ALTER COLUMN performed_by TYPE varchar(20) USING LPAD(performed_by::text, 20, '0');

-- 12. projects
ALTER TABLE projects ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 13. resource_code_versions
ALTER TABLE resource_code_versions ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 14. schemas
ALTER TABLE schemas ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 15. system_configs
ALTER TABLE system_configs ALTER COLUMN updated_by TYPE varchar(20) USING LPAD(updated_by::text, 20, '0');

-- 16. task_temporary_permissions
ALTER TABLE task_temporary_permissions ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 17. vcs_providers
ALTER TABLE vcs_providers ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 18. webhook_configs
ALTER TABLE webhook_configs ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 19. workspace_members
ALTER TABLE workspace_members ALTER COLUMN user_id TYPE varchar(20) USING LPAD(user_id::text, 20, '0');

-- 20. workspace_resources
ALTER TABLE workspace_resources ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 21. workspace_resources_snapshot
ALTER TABLE workspace_resources_snapshot ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 22. workspace_state_versions
ALTER TABLE workspace_state_versions ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 23. workspace_variables
ALTER TABLE workspace_variables ALTER COLUMN created_by TYPE varchar(20) USING LPAD(created_by::text, 20, '0');

-- 24. workspaces
ALTER TABLE workspaces ALTER COLUMN locked_by TYPE varchar(20) USING LPAD(locked_by::text, 20, '0');

COMMIT;

-- 验证
SELECT table_name, column_name, data_type 
FROM information_schema.columns 
WHERE (column_name LIKE '%user%' OR column_name LIKE '%by') 
  AND data_type = 'integer' 
  AND table_schema = 'public'
  AND table_name NOT LIKE '%backup%'
ORDER BY table_name, column_name;
