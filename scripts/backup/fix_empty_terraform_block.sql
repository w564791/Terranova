-- 修复workspace中空的terraform块
-- 这个空块会导致Terraform init尝试读取backend state，从而在首次运行时失败

UPDATE workspaces 
SET provider_config = jsonb_set(
    provider_config,
    '{terraform}',
    'null'::jsonb
) - 'terraform'
WHERE provider_config ? 'terraform' 
  AND (
    provider_config->'terraform' = '[]'::jsonb 
    OR provider_config->'terraform' = '{}'::jsonb
  );

-- 验证修复结果
SELECT workspace_id, name, provider_config 
FROM workspaces 
WHERE workspace_id = 'ws-mb7m9ii5ey';
