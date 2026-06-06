-- Patch: manifest AI 两个 capability 的专用 AIConfig
--   manifest_resource_generation  资源生成/修复
--   manifest_check                草稿检查
--
-- 参照 form_generation(#12): bedrock / mode=skill / enabled=false / priority=0。
-- 服务层优先读取本 skill_composition。已启用 meta_rules(冲突以 Foundation 为准 +
-- 按层分段标注 prompt),domain_skill_mode 见各行,foundation 含 aws_resource_tagging。
-- use_optimized=false(默认不跑 domain skill AI 选择);可在 UI 调整。
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
  '{"meta_rules": {"enabled": true}, "task_skill": "manifest_resource_generation_workflow", "domain_skills": ["terraform_module_best_practices"], "conditional_rules": [], "domain_skill_mode": "hybrid", "foundation_skills": ["output_format_standard", "aws_resource_tagging", "infrastructure_risk_baseline"], "auto_load_module_skill": false}'::jsonb,
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
  '{"meta_rules": {"enabled": true}, "task_skill": "manifest_check_workflow", "domain_skills": ["terraform_module_best_practices"], "conditional_rules": [], "domain_skill_mode": "auto", "foundation_skills": ["json_output_format", "aws_resource_tagging", "infrastructure_risk_baseline"], "auto_load_module_skill": false}'::jsonb,
  false, false, 10000, now(), now()
WHERE NOT EXISTS (
  SELECT 1 FROM public.ai_configs WHERE capabilities @> '["manifest_check"]'::jsonb
);

COMMIT;
