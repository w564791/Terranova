-- 添加module_source字段到modules表
-- 用于存储Terraform Module的source地址

-- 添加module_source字段（可选字段）
ALTER TABLE modules ADD COLUMN IF NOT EXISTS module_source VARCHAR(500);

-- 添加注释
COMMENT ON COLUMN modules.module_source IS 'Terraform Module的source地址，用于在main.tf.json中引用此Module';

-- 为已存在的记录设置默认值（如果source字段有值，复制到module_source）
UPDATE modules 
SET module_source = source 
WHERE module_source IS NULL AND source IS NOT NULL AND source != '';

-- 创建索引以提高查询性能
CREATE INDEX IF NOT EXISTS idx_modules_module_source ON modules(module_source);

-- 显示修改结果
SELECT 
    column_name, 
    data_type, 
    character_maximum_length,
    is_nullable,
    column_default
FROM information_schema.columns 
WHERE table_name = 'modules' 
AND column_name IN ('source', 'module_source')
ORDER BY ordinal_position;
