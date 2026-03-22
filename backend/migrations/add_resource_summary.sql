-- 为 resource_index 添加 AI 资源摘要字段
ALTER TABLE public.resource_index
    ADD COLUMN IF NOT EXISTS resource_summary text,
    ADD COLUMN IF NOT EXISTS summary_hash character varying(32);

COMMENT ON COLUMN public.resource_index.resource_summary IS 'AI 生成的资源配置摘要，用于增强向量搜索和变更影响分析';
COMMENT ON COLUMN public.resource_index.summary_hash IS 'attributes 的 MD5 hash，用于检测属性变更避免重复生成摘要';
