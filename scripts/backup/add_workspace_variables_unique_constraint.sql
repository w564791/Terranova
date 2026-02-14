-- 添加workspace_variables表的唯一性约束
-- 确保在同一个workspace中，相同类型的变量名不能重复

-- 1. 首先检查是否有重复数据
SELECT workspace_id, key, variable_type, COUNT(*) as count
FROM workspace_variables
GROUP BY workspace_id, key, variable_type
HAVING COUNT(*) > 1;

-- 如果上面的查询返回结果，说明有重复数据，需要先清理

-- 2. 添加唯一约束
-- 这将确保 (workspace_id, key, variable_type) 的组合是唯一的
ALTER TABLE workspace_variables
ADD CONSTRAINT workspace_variables_workspace_key_type_unique 
UNIQUE (workspace_id, key, variable_type);

-- 3. 验证约束已添加
SELECT conname, contype, pg_get_constraintdef(oid) 
FROM pg_constraint 
WHERE conrelid = 'workspace_variables'::regclass 
AND conname = 'workspace_variables_workspace_key_type_unique';
