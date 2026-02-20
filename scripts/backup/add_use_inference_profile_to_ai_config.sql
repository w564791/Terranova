-- 添加是否使用 inference profile 的配置字段
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS use_inference_profile BOOLEAN DEFAULT false;

COMMENT ON COLUMN ai_configs.use_inference_profile IS '是否使用 cross-region inference profile（某些新模型需要）';

-- 更新现有配置的默认值
UPDATE ai_configs SET use_inference_profile = false WHERE use_inference_profile IS NULL;
