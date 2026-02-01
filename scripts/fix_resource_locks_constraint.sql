-- 修复resource_locks表的唯一约束
-- 问题: UNIQUE(resource_id)导致同一资源只能有一个锁,无法支持多session
-- 解决: 移除resource_id的唯一约束,改为(resource_id, session_id)的唯一约束

-- 1. 删除旧的唯一约束
ALTER TABLE resource_locks DROP CONSTRAINT IF EXISTS uq_resource_locks_resource;

-- 2. 添加新的唯一约束(resource_id + session_id)
ALTER TABLE resource_locks ADD CONSTRAINT uq_resource_locks_resource_session UNIQUE(resource_id, session_id);

-- 3. 验证
\d resource_locks

-- 显示结果
SELECT 'resource_locks表约束已更新: 现在支持同一资源的多个session' AS result;
