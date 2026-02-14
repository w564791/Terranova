-- 添加source_type字段到schemas表
ALTER TABLE schemas ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) DEFAULT 'json_import';

-- 更新现有数据
-- 将AI生成的Schema标记为ai_generate
UPDATE schemas SET source_type = 'ai_generate' WHERE ai_generated = true AND source_type IS NULL;

-- 其他的默认为json_import
UPDATE schemas SET source_type = 'json_import' WHERE source_type IS NULL;

-- 查看更新结果
SELECT id, module_id, version, status, ai_generated, source_type, created_at 
FROM schemas 
ORDER BY created_at DESC;
