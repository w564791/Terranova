-- 迁移 workspace_resources 表的 workspace_id 字段
-- 从 INTEGER 改为 VARCHAR(50) 存储语义化 ID

BEGIN;

-- 1. 删除外键约束
ALTER TABLE workspace_resources 
DROP CONSTRAINT IF EXISTS workspace_resources_workspace_id_fkey;

-- 2. 创建临时列存储语义化 ID
ALTER TABLE workspace_resources 
ADD COLUMN workspace_id_temp VARCHAR(50);

-- 3. 从 workspaces 表填充语义化 ID
UPDATE workspace_resources wr
SET workspace_id_temp = w.workspace_id
FROM workspaces w
WHERE wr.workspace_id = w.id;

-- 4. 验证数据
DO $$
DECLARE
    null_count INTEGER;
    total_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO total_count FROM workspace_resources;
    SELECT COUNT(*) INTO null_count FROM workspace_resources WHERE workspace_id_temp IS NULL;
    
    IF null_count > 0 THEN
        RAISE EXCEPTION '发现 % 条记录的 workspace_id_temp 为 NULL (总共 % 条)', null_count, total_count;
    END IF;
    
    RAISE NOTICE '✓ 所有 % 条记录的 workspace_id_temp 已填充', total_count;
END $$;

-- 5. 删除旧的 workspace_id 列
ALTER TABLE workspace_resources DROP COLUMN workspace_id;

-- 6. 重命名临时列为 workspace_id
ALTER TABLE workspace_resources 
RENAME COLUMN workspace_id_temp TO workspace_id;

-- 7. 设置为 NOT NULL
ALTER TABLE workspace_resources 
ALTER COLUMN workspace_id SET NOT NULL;

-- 8. 创建索引
CREATE INDEX idx_workspace_resources_workspace_semantic 
ON workspace_resources(workspace_id);

-- 9. 重建唯一索引
DROP INDEX IF EXISTS idx_workspace_resources_unique;
CREATE UNIQUE INDEX idx_workspace_resources_unique 
ON workspace_resources(workspace_id, resource_id) 
WHERE is_active = true;

COMMIT;

-- 验证结果
SELECT 
    'workspace_resources' as table_name,
    COUNT(*) as total_records,
    COUNT(DISTINCT workspace_id) as unique_workspaces
FROM workspace_resources;

-- 显示示例数据
SELECT id, workspace_id, resource_id, resource_name 
FROM workspace_resources 
LIMIT 5;
