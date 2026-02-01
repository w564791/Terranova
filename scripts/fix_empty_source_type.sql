-- 修复空字符串的source_type
-- 将空字符串更新为正确的值

-- 对于通过TF文件导入的模块（repository_url包含'tf-file-import'），设置为tf_parse
UPDATE schemas 
SET source_type = 'tf_parse' 
WHERE (source_type IS NULL OR source_type = '') 
  AND module_id IN (
    SELECT id FROM modules WHERE repository_url LIKE '%tf-file-import%'
  );

-- 对于AI生成的Schema，设置为ai_generate
UPDATE schemas 
SET source_type = 'ai_generate' 
WHERE (source_type IS NULL OR source_type = '') 
  AND ai_generated = true;

-- 其他的默认为json_import
UPDATE schemas 
SET source_type = 'json_import' 
WHERE source_type IS NULL OR source_type = '';

-- 查看更新结果
SELECT id, module_id, version, status, ai_generated, source_type, created_at 
FROM schemas 
ORDER BY created_at DESC
LIMIT 10;
