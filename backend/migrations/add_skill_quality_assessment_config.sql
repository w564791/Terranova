-- Register skill_quality_assessment AI config capability
-- Uses Haiku model for cost-effective Layer 2/3 evaluation
-- Copies settings from the wildcard (*) config if available

-- enabled=false: 专用 capability config（GetConfigForCapability 优先查 enabled=false）
-- enabled=true 的 ["*"] 配置是全局兜底，不应被新 config 抢占
-- skill_composition.task_skill = Layer 2 评估 Skill
-- capability_prompts.semantic_skill = Layer 3 评估 Skill
INSERT INTO ai_configs (service_type, aws_region, model_id, enabled, capabilities, capability_prompts, use_inference_profile, rate_limit_seconds, priority, mode, skill_composition)
SELECT service_type, aws_region, model_id, false,
       '["skill_quality_assessment"]'::jsonb,
       '{"semantic_skill": "skill_quality_semantic_evaluation"}'::jsonb,
       use_inference_profile, rate_limit_seconds, 0,
       'skill',
       '{"task_skill": "skill_quality_rule_evaluation", "foundation_skills": [], "domain_skills": [], "domain_skill_mode": "fixed", "conditional_rules": [], "auto_load_module_skill": false}'::jsonb
FROM ai_configs
WHERE enabled = true AND capabilities @> '["*"]'::jsonb
LIMIT 1
ON CONFLICT DO NOTHING;
