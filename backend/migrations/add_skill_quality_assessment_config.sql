-- Register skill_quality_assessment AI config capability
-- Uses Haiku model for cost-effective Layer 2/3 evaluation
-- Copies settings from the wildcard (*) config if available

-- enabled=false: 专用 capability config（GetConfigForCapability 优先查 enabled=false）
-- enabled=true 的 ["*"] 配置是全局兜底，不应被新 config 抢占
INSERT INTO ai_configs (service_type, aws_region, model_id, enabled, capabilities, capability_prompts, use_inference_profile, rate_limit_seconds, priority)
SELECT service_type, aws_region, model_id, false,
       '["skill_quality_assessment"]'::jsonb,
       '{}'::jsonb,
       use_inference_profile, rate_limit_seconds, 0
FROM ai_configs
WHERE enabled = true AND capabilities @> '["*"]'::jsonb
LIMIT 1
ON CONFLICT DO NOTHING;
