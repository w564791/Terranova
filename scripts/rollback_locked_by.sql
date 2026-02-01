-- 回滚脚本：将locked_by字段恢复为integer类型并重新添加外键

-- 1. 清空locked_by数据
UPDATE workspaces 
SET locked_by = NULL, 
    locked_at = NULL,
    lock_reason = NULL
WHERE locked_by IS NOT NULL;

-- 2. 修改locked_by字段类型回integer
ALTER TABLE workspaces 
ALTER COLUMN locked_by TYPE INTEGER USING locked_by::INTEGER;

-- 3. 重新添加外键约束
ALTER TABLE workspaces 
ADD CONSTRAINT workspaces_locked_by_fkey 
FOREIGN KEY (locked_by) REFERENCES users(id);

-- 4. 更新注释
COMMENT ON COLUMN workspaces.locked_by IS '锁定workspace的用户ID（外键关联users表）';
