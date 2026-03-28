-- 扩展 cmdb_sync_logs 表，支持 workspace 同步日志
-- 删除外键约束（workspace sync log 的 source_id 是 workspace_id，不在 cmdb_external_sources 表中）
ALTER TABLE cmdb_sync_logs DROP CONSTRAINT IF EXISTS fk_cmdb_sync_logs_source;

ALTER TABLE cmdb_sync_logs ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) NOT NULL DEFAULT 'external';
ALTER TABLE cmdb_sync_logs ADD COLUMN IF NOT EXISTS source_name VARCHAR(200) DEFAULT '';
ALTER TABLE cmdb_sync_logs ADD COLUMN IF NOT EXISTS triggered_by VARCHAR(20) DEFAULT '';

-- 为按类型查询和时间排序添加索引
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_source_type ON cmdb_sync_logs (source_type);
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_completed_at ON cmdb_sync_logs (completed_at DESC);
