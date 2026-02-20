-- 添加 use_optimized 字段到 ai_configs 表
-- 执行方式: PGPASSWORD=postgres123 psql -h localhost -p 15432 -U postgres -d iac_platform -f scripts/add_use_optimized_to_ai_configs.sql

-- 1. 添加 use_optimized 字段
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS use_optimized BOOLEAN DEFAULT false;

-- 2. 添加注释
COMMENT ON COLUMN ai_configs.use_optimized IS '是否使用优化版（并行执行 CMDB 查询 + AI 智能选择 Domain Skills）';

-- 3. 验证
SELECT id, service_type, capabilities, mode, use_optimized
FROM ai_configs
ORDER BY id;