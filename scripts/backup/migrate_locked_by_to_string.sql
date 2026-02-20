-- 迁移脚本：将workspaces表的locked_by字段从integer改为varchar
-- 用于记录锁定workspace的用户名而不是用户ID

-- 1. 删除外键约束（如果存在）
ALTER TABLE workspaces 
DROP CONSTRAINT IF EXISTS workspaces_locked_by_fkey;

-- 2. 修改locked_by字段类型为varchar
ALTER TABLE workspaces 
ALTER COLUMN locked_by TYPE VARCHAR(255) USING locked_by::VARCHAR;

-- 3. 清空现有的locked_by数据（因为之前存的是数字ID，现在需要用户名）
UPDATE workspaces 
SET locked_by = NULL 
WHERE locked_by IS NOT NULL AND locked_by != '';

-- 4. 添加注释
COMMENT ON COLUMN workspaces.locked_by IS '锁定workspace的用户名';
