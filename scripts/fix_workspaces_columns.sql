-- 修复workspaces表缺失的列
-- 创建日期: 2025-10-11

-- 添加缺失的列
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS current_code_version_id INTEGER;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS workspace_execution_mode VARCHAR(20) DEFAULT 'plan_and_apply';

-- 验证列已添加
SELECT 
    column_name, 
    data_type, 
    column_default,
    is_nullable
FROM information_schema.columns 
WHERE table_name = 'workspaces' 
AND column_name IN ('current_code_version_id', 'workspace_execution_mode')
ORDER BY column_name;

-- 显示成功消息
SELECT ' workspaces表列已成功添加' AS status;
