-- 添加 capability_prompts 字段到 ai_configs 表
-- 用于存储每个场景的自定义 prompt

ALTER TABLE ai_configs 
ADD COLUMN IF NOT EXISTS capability_prompts JSONB DEFAULT '{}';

-- 添加注释
COMMENT ON COLUMN ai_configs.capability_prompts IS '每个能力场景的自定义 prompt，格式: {"error_analysis": "...", "form_generation": "..."}';

-- 示例数据结构:
-- {
--   "error_analysis": "自定义的错误分析 prompt...",
--   "change_analysis": "自定义的变更分析 prompt...",
--   "result_analysis": "自定义的结果分析 prompt...",
--   "resource_generation": "自定义的资源生成 prompt...",
--   "form_generation": "自定义的表单生成 prompt..."
-- }
