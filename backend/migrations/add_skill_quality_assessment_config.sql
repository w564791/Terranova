-- Skill quality evaluation AI configs
-- Two separate configs: one for Layer 2 (rule), one for Layer 3 (semantic)
-- enabled=false: 专用 capability config（GetConfigForCapability 优先查 enabled=false）

-- Layer 2: skill_rule_evaluation
INSERT INTO ai_configs (service_type, aws_region, model_id, enabled, capabilities, capability_prompts, use_inference_profile, rate_limit_seconds, priority, mode, skill_composition)
SELECT service_type, aws_region, model_id, false,
       '["skill_rule_evaluation"]'::jsonb,
       '{}'::jsonb,
       use_inference_profile, rate_limit_seconds, 0,
       'skill',
       '{"task_skill": "skill_quality_rule_evaluation", "foundation_skills": [], "domain_skills": [], "domain_skill_mode": "fixed", "conditional_rules": [], "auto_load_module_skill": false}'::jsonb
FROM ai_configs
WHERE enabled = true AND capabilities @> '["*"]'::jsonb
LIMIT 1
ON CONFLICT DO NOTHING;

-- Layer 3: skill_semantic_evaluation
INSERT INTO ai_configs (service_type, aws_region, model_id, enabled, capabilities, capability_prompts, use_inference_profile, rate_limit_seconds, priority, mode, skill_composition)
SELECT service_type, aws_region, model_id, false,
       '["skill_semantic_evaluation"]'::jsonb,
       '{}'::jsonb,
       use_inference_profile, rate_limit_seconds, 0,
       'skill',
       '{"task_skill": "skill_quality_semantic_evaluation", "foundation_skills": [], "domain_skills": [], "domain_skill_mode": "fixed", "conditional_rules": [], "auto_load_module_skill": false}'::jsonb
FROM ai_configs
WHERE enabled = true AND capabilities @> '["*"]'::jsonb
LIMIT 1
ON CONFLICT DO NOTHING;
