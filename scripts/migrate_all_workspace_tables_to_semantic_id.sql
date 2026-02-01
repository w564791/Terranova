-- 统一迁移所有 workspace 关联表使用语义化 workspace_id
-- 将 workspace_id 从 INTEGER 改为 VARCHAR(50)

BEGIN;

-- ============================================================================
-- 1. workspace_variables 表
-- ============================================================================

-- 删除外键
ALTER TABLE workspace_variables 
DROP CONSTRAINT IF EXISTS workspace_variables_workspace_id_fkey;

-- 添加临时列
ALTER TABLE workspace_variables 
ADD COLUMN workspace_id_temp VARCHAR(50);

-- 填充数据
UPDATE workspace_variables wv
SET workspace_id_temp = w.workspace_id
FROM workspaces w
WHERE wv.workspace_id = w.id;

-- 验证
DO $$
DECLARE
    null_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO null_count FROM workspace_variables WHERE workspace_id_temp IS NULL;
    IF null_count > 0 THEN
        RAISE EXCEPTION 'workspace_variables: % 条记录未填充', null_count;
    END IF;
END $$;

-- 替换字段
ALTER TABLE workspace_variables DROP COLUMN workspace_id;
ALTER TABLE workspace_variables RENAME COLUMN workspace_id_temp TO workspace_id;
ALTER TABLE workspace_variables ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX idx_workspace_variables_workspace_semantic ON workspace_variables(workspace_id);

-- ============================================================================
-- 2. workspace_tasks 表
-- ============================================================================

-- 删除外键
ALTER TABLE workspace_tasks 
DROP CONSTRAINT IF EXISTS workspace_tasks_workspace_id_fkey;

-- 添加临时列
ALTER TABLE workspace_tasks 
ADD COLUMN workspace_id_temp VARCHAR(50);

-- 填充数据
UPDATE workspace_tasks wt
SET workspace_id_temp = w.workspace_id
FROM workspaces w
WHERE wt.workspace_id = w.id;

-- 验证
DO $$
DECLARE
    null_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO null_count FROM workspace_tasks WHERE workspace_id_temp IS NULL;
    IF null_count > 0 THEN
        RAISE EXCEPTION 'workspace_tasks: % 条记录未填充', null_count;
    END IF;
END $$;

-- 替换字段
ALTER TABLE workspace_tasks DROP COLUMN workspace_id;
ALTER TABLE workspace_tasks RENAME COLUMN workspace_id_temp TO workspace_id;
ALTER TABLE workspace_tasks ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX idx_workspace_tasks_workspace_semantic ON workspace_tasks(workspace_id);

-- ============================================================================
-- 3. workspace_state_versions 表
-- ============================================================================

-- 删除外键
ALTER TABLE workspace_state_versions 
DROP CONSTRAINT IF EXISTS workspace_state_versions_workspace_id_fkey;

-- 添加临时列
ALTER TABLE workspace_state_versions 
ADD COLUMN workspace_id_temp VARCHAR(50);

-- 填充数据
UPDATE workspace_state_versions wsv
SET workspace_id_temp = w.workspace_id
FROM workspaces w
WHERE wsv.workspace_id = w.id;

-- 验证
DO $$
DECLARE
    null_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO null_count FROM workspace_state_versions WHERE workspace_id_temp IS NULL;
    IF null_count > 0 THEN
        RAISE EXCEPTION 'workspace_state_versions: % 条记录未填充', null_count;
    END IF;
END $$;

-- 替换字段
ALTER TABLE workspace_state_versions DROP COLUMN workspace_id;
ALTER TABLE workspace_state_versions RENAME COLUMN workspace_id_temp TO workspace_id;
ALTER TABLE workspace_state_versions ALTER COLUMN workspace_id SET NOT NULL;
CREATE INDEX idx_workspace_state_versions_workspace_semantic ON workspace_state_versions(workspace_id);

COMMIT;

-- 最终验证
SELECT 
    'workspace_variables' as table_name,
    COUNT(*) as total,
    COUNT(DISTINCT workspace_id) as unique_workspaces,
    MIN(workspace_id) as sample_id
FROM workspace_variables
UNION ALL
SELECT 
    'workspace_tasks',
    COUNT(*),
    COUNT(DISTINCT workspace_id),
    MIN(workspace_id)
FROM workspace_tasks
UNION ALL
SELECT 
    'workspace_state_versions',
    COUNT(*),
    COUNT(DISTINCT workspace_id),
    MIN(workspace_id)
FROM workspace_state_versions;
