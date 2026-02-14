-- ============================================================================
-- 为 Workspace 添加语义化 ID 字段
-- 策略: 保留现有的自增 ID,新增 workspace_id 作为语义化标识
-- 这是一个低风险的渐进式方案
-- ============================================================================

-- 第一步: 创建生成语义化 ID 的函数
-- ============================================================================
CREATE OR REPLACE FUNCTION generate_workspace_semantic_id() RETURNS VARCHAR(50) AS $$
DECLARE
    chars TEXT := 'abcdefghijklmnopqrstuvwxyz0123456789';
    result TEXT := 'ws-';
    i INTEGER;
    new_id TEXT;
    id_exists BOOLEAN;
BEGIN
    LOOP
        result := 'ws-';
        FOR i IN 1..16 LOOP
            result := result || substr(chars, floor(random() * length(chars) + 1)::integer, 1);
        END LOOP;
        
        -- 检查是否已存在
        SELECT EXISTS(SELECT 1 FROM workspaces WHERE workspace_id = result) INTO id_exists;
        
        IF NOT id_exists THEN
            RETURN result;
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql;

-- 第二步: 添加 workspace_id 字段到 workspaces 表
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '添加 workspace_id 字段到 workspaces 表...';
END $$;

ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS workspace_id VARCHAR(50);

-- 第三步: 为所有现有 workspace 生成语义化 ID
-- ============================================================================
DO $$
DECLARE
    ws_record RECORD;
    new_id VARCHAR(50);
BEGIN
    RAISE NOTICE '为现有 workspace 生成语义化 ID...';
    
    FOR ws_record IN SELECT id FROM workspaces WHERE workspace_id IS NULL
    LOOP
        new_id := generate_workspace_semantic_id();
        UPDATE workspaces SET workspace_id = new_id WHERE id = ws_record.id;
    END LOOP;
    
    RAISE NOTICE '语义化 ID 生成完成';
END $$;

-- 第四步: 添加唯一约束和索引
-- ============================================================================
DO $$
BEGIN
    RAISE NOTICE '添加唯一约束和索引...';
END $$;

-- 设置为 NOT NULL
ALTER TABLE workspaces ALTER COLUMN workspace_id SET NOT NULL;

-- 添加唯一约束
ALTER TABLE workspaces ADD CONSTRAINT uk_workspaces_workspace_id UNIQUE (workspace_id);

-- 添加索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_workspaces_workspace_id ON workspaces(workspace_id);

-- 第五步: 验证
-- ============================================================================
DO $$
DECLARE
    total_count INTEGER;
    null_count INTEGER;
    duplicate_count INTEGER;
BEGIN
    RAISE NOTICE '验证数据完整性...';
    
    SELECT COUNT(*) INTO total_count FROM workspaces;
    SELECT COUNT(*) INTO null_count FROM workspaces WHERE workspace_id IS NULL;
    
    SELECT COUNT(*) INTO duplicate_count
    FROM (
        SELECT workspace_id, COUNT(*) as cnt
        FROM workspaces
        GROUP BY workspace_id
        HAVING COUNT(*) > 1
    ) duplicates;
    
    IF null_count > 0 THEN
        RAISE EXCEPTION '发现 % 条记录的 workspace_id 为 NULL', null_count;
    END IF;
    
    IF duplicate_count > 0 THEN
        RAISE EXCEPTION '发现 % 个重复的 workspace_id', duplicate_count;
    END IF;
    
    RAISE NOTICE '===========================================';
    RAISE NOTICE '迁移完成!';
    RAISE NOTICE '===========================================';
    RAISE NOTICE 'Workspaces 总数: %', total_count;
    RAISE NOTICE '所有记录都已分配唯一的 workspace_id';
    RAISE NOTICE '===========================================';
    RAISE NOTICE '下一步:';
    RAISE NOTICE '1. 修改后端代码,在 Workspace 模型中添加 WorkspaceID 字段';
    RAISE NOTICE '2. 修改 API,支持使用 workspace_id 访问';
    RAISE NOTICE '3. 逐步迁移前端代码使用 workspace_id';
    RAISE NOTICE '4. 最终可以考虑废弃数字 ID (可选)';
    RAISE NOTICE '===========================================';
END $$;

-- 第六步: 创建触发器,为新创建的 workspace 自动生成 workspace_id
-- ============================================================================
CREATE OR REPLACE FUNCTION auto_generate_workspace_id() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.workspace_id IS NULL THEN
        NEW.workspace_id := generate_workspace_semantic_id();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_auto_generate_workspace_id ON workspaces;

CREATE TRIGGER trigger_auto_generate_workspace_id
    BEFORE INSERT ON workspaces
    FOR EACH ROW
    EXECUTE FUNCTION auto_generate_workspace_id();

DO $$
BEGIN
    RAISE NOTICE '触发器已创建: 新 workspace 将自动生成 workspace_id';
END $$;
