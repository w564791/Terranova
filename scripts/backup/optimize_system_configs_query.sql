-- 优化 system_configs 表查询性能
-- 问题：audit_logger 每分钟查询 system_configs 表3次，每次耗时 500ms+
-- 原因：key 字段没有索引，导致全表扫描

-- 1. 为 key 字段添加索引（如果不存在）
CREATE INDEX IF NOT EXISTS idx_system_configs_key ON system_configs(key);

-- 2. 验证索引是否创建成功
SELECT 
    schemaname,
    tablename,
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename = 'system_configs';

-- 3. 分析表统计信息，帮助查询优化器
ANALYZE system_configs;

-- 4. 查看当前 system_configs 表的数据量
SELECT COUNT(*) as total_rows FROM system_configs;

-- 5. 测试查询性能（应该从 500ms+ 降低到 <1ms）
EXPLAIN ANALYZE
SELECT value::text FROM system_configs WHERE key = 'audit_log_enabled';
