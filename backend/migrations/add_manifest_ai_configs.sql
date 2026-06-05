-- Patch: manifest AI 两个 capability 的专用 AIConfig
--   manifest_resource_generation  资源生成/修复
--   manifest_check                草稿检查
--
-- 参照 form_generation(#12): bedrock / opus / mode=skill / enabled=false / priority=0。
-- 服务层(ManifestAIService/ManifestCheckService)优先读取本 skill_composition,
-- 为空时才回退到代码里的硬编码默认。因此这里的组合即生效配置,可在 UI 调整。
--
-- 幂等: 按 capabilities 判重(WHERE NOT EXISTS), 可重复执行。
BEGIN;

-- manifest_resource_generation
INSERT INTO public.ai_configs
  (service_type, aws_region, model_id, enabled, rate_limit_seconds, use_inference_profile,
   capabilities, priority, capability_prompts, mode, skill_composition,
   use_optimized, thinking_enabled, thinking_budget_tokens, created_at, updated_at)
SELECT
  'bedrock', 'ap-southeast-1', 'global.anthropic.claude-opus-4-6-v1', false, 10, true,
  '["manifest_resource_generation"]'::jsonb, 0, '{}'::jsonb, 'skill',
  '{"task_skill": "manifest_resource_generation_workflow", "domain_skills": ["terraform_module_best_practices"], "conditional_rules": [], "domain_skill_mode": "fixed", "foundation_skills": ["output_format_standard"], "auto_load_module_skill": false}'::jsonb,
  false, false, 10000, now(), now()
WHERE NOT EXISTS (
  SELECT 1 FROM public.ai_configs WHERE capabilities @> '["manifest_resource_generation"]'::jsonb
);

-- manifest_check
INSERT INTO public.ai_configs
  (service_type, aws_region, model_id, enabled, rate_limit_seconds, use_inference_profile,
   capabilities, priority, capability_prompts, mode, skill_composition,
   use_optimized, thinking_enabled, thinking_budget_tokens, created_at, updated_at)
SELECT
  'bedrock', 'ap-southeast-1', 'global.anthropic.claude-sonnet-4-6', false, 10, true,
  '["manifest_check"]'::jsonb, 0, '{}'::jsonb, 'skill',
  '{"task_skill": "manifest_check_workflow", "domain_skills": ["terraform_module_best_practices"], "conditional_rules": [], "domain_skill_mode": "fixed", "foundation_skills": ["json_output_format"], "auto_load_module_skill": false}'::jsonb,
  false, false, 10000, now(), now()
WHERE NOT EXISTS (
  SELECT 1 FROM public.ai_configs WHERE capabilities @> '["manifest_check"]'::jsonb
);

COMMIT;
