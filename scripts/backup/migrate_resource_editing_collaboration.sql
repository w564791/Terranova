-- 资源编辑协作系统数据库迁移脚本
-- 创建时间: 2025-01-18
-- 说明: 添加资源锁和草稿表，支持多用户协作编辑

-- ============================================================================
-- 1. 创建资源锁表 (resource_locks)
-- ============================================================================

CREATE TABLE IF NOT EXISTS resource_locks (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    editing_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(100) NOT NULL,
    lock_type VARCHAR(20) NOT NULL DEFAULT 'optimistic',
    version INTEGER NOT NULL DEFAULT 1,
    last_heartbeat TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_resource_locks_resource UNIQUE(resource_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_resource_locks_resource ON resource_locks(resource_id);
CREATE INDEX IF NOT EXISTS idx_resource_locks_user ON resource_locks(editing_user_id);
CREATE INDEX IF NOT EXISTS idx_resource_locks_heartbeat ON resource_locks(last_heartbeat);
CREATE INDEX IF NOT EXISTS idx_resource_locks_session ON resource_locks(session_id);

-- 添加注释
COMMENT ON TABLE resource_locks IS '资源锁表，记录当前正在编辑资源的用户和会话信息';
COMMENT ON COLUMN resource_locks.resource_id IS '被锁定的资源ID';
COMMENT ON COLUMN resource_locks.editing_user_id IS '当前编辑用户ID';
COMMENT ON COLUMN resource_locks.session_id IS '浏览器会话ID，用于区分同用户多窗口';
COMMENT ON COLUMN resource_locks.lock_type IS '锁类型：optimistic(乐观锁) / pessimistic(悲观锁)';
COMMENT ON COLUMN resource_locks.version IS '资源版本号，用于乐观锁校验';
COMMENT ON COLUMN resource_locks.last_heartbeat IS '最后心跳时间，用于判断锁是否过期';

-- ============================================================================
-- 2. 创建资源草稿表 (resource_drifts)
-- ============================================================================

CREATE TABLE IF NOT EXISTS resource_drifts (
    id SERIAL PRIMARY KEY,
    resource_id INTEGER NOT NULL REFERENCES workspace_resources(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(100) NOT NULL,
    drift_content JSONB NOT NULL,
    base_version INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    last_heartbeat TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_resource_drifts_resource_user_session UNIQUE(resource_id, user_id, session_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_resource_drifts_resource ON resource_drifts(resource_id);
CREATE INDEX IF NOT EXISTS idx_resource_drifts_user ON resource_drifts(user_id);
CREATE INDEX IF NOT EXISTS idx_resource_drifts_status ON resource_drifts(status);
CREATE INDEX IF NOT EXISTS idx_resource_drifts_heartbeat ON resource_drifts(last_heartbeat);
CREATE INDEX IF NOT EXISTS idx_resource_drifts_session ON resource_drifts(session_id);

-- 添加注释
COMMENT ON TABLE resource_drifts IS '资源草稿表，保存用户的临时编辑内容，支持中断恢复';
COMMENT ON COLUMN resource_drifts.resource_id IS '资源ID';
COMMENT ON COLUMN resource_drifts.user_id IS '编辑用户ID';
COMMENT ON COLUMN resource_drifts.session_id IS '会话ID';
COMMENT ON COLUMN resource_drifts.drift_content IS '草稿内容（JSON格式，包含formData和changeSummary）';
COMMENT ON COLUMN resource_drifts.base_version IS '草稿基于的资源版本号';
COMMENT ON COLUMN resource_drifts.status IS '状态：active(活跃) / expired(过期) / submitted(已提交)';
COMMENT ON COLUMN resource_drifts.last_heartbeat IS '最后心跳时间';

-- ============================================================================
-- 3. 创建自动更新 updated_at 的触发器
-- ============================================================================

-- 为 resource_locks 表创建触发器
CREATE OR REPLACE FUNCTION update_resource_locks_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_resource_locks_updated_at ON resource_locks;
CREATE TRIGGER trigger_update_resource_locks_updated_at
    BEFORE UPDATE ON resource_locks
    FOR EACH ROW
    EXECUTE FUNCTION update_resource_locks_updated_at();

-- 为 resource_drifts 表创建触发器
CREATE OR REPLACE FUNCTION update_resource_drifts_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_resource_drifts_updated_at ON resource_drifts;
CREATE TRIGGER trigger_update_resource_drifts_updated_at
    BEFORE UPDATE ON resource_drifts
    FOR EACH ROW
    EXECUTE FUNCTION update_resource_drifts_updated_at();

-- ============================================================================
-- 4. 创建清理过期记录的函数（可选，用于定期清理）
-- ============================================================================

-- 清理过期的锁（2分钟无心跳）
CREATE OR REPLACE FUNCTION cleanup_expired_resource_locks()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM resource_locks
    WHERE last_heartbeat < NOW() - INTERVAL '2 minutes';
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_expired_resource_locks() IS '清理2分钟无心跳的过期锁';

-- 清理过期的草稿（7天无更新且状态为expired）
CREATE OR REPLACE FUNCTION cleanup_old_resource_drifts()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM resource_drifts
    WHERE status = 'expired' 
    AND updated_at < NOW() - INTERVAL '7 days';
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_old_resource_drifts() IS '清理7天前的过期草稿';

-- ============================================================================
-- 5. 验证迁移
-- ============================================================================

-- 检查表是否创建成功
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'resource_locks') THEN
        RAISE NOTICE '✓ resource_locks 表创建成功';
    ELSE
        RAISE EXCEPTION '✗ resource_locks 表创建失败';
    END IF;
    
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'resource_drifts') THEN
        RAISE NOTICE '✓ resource_drifts 表创建成功';
    ELSE
        RAISE EXCEPTION '✗ resource_drifts 表创建失败';
    END IF;
END $$;

-- 显示表结构
\d resource_locks
\d resource_drifts

-- ============================================================================
-- 6. 回滚脚本（如需回滚，执行以下语句）
-- ============================================================================

/*
-- 删除触发器
DROP TRIGGER IF EXISTS trigger_update_resource_locks_updated_at ON resource_locks;
DROP TRIGGER IF EXISTS trigger_update_resource_drifts_updated_at ON resource_drifts;

-- 删除函数
DROP FUNCTION IF EXISTS update_resource_locks_updated_at();
DROP FUNCTION IF EXISTS update_resource_drifts_updated_at();
DROP FUNCTION IF EXISTS cleanup_expired_resource_locks();
DROP FUNCTION IF EXISTS cleanup_old_resource_drifts();

-- 删除表
DROP TABLE IF EXISTS resource_drifts;
DROP TABLE IF EXISTS resource_locks;
*/
