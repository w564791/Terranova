-- 修复TF_CLI_ARGS变量的variable_type
-- 将'system'改为'environment'，因为后端只支持'terraform'和'environment'两种类型

UPDATE workspace_variables 
SET variable_type = 'environment' 
WHERE key = 'TF_CLI_ARGS' 
  AND variable_type = 'system';

-- 查询修复结果
SELECT id, workspace_id, key, value, variable_type, sensitive, description 
FROM workspace_variables 
WHERE key = 'TF_CLI_ARGS';
