-- Register skill_quality_assessment AI config capability
-- Uses Haiku model for cost-effective Layer 2/3 evaluation
-- Copies settings from the wildcard (*) config if available

INSERT INTO ai_configs (service_type, aws_region, model_id, enabled, capabilities, capability_prompts, use_inference_profile, rate_limit_seconds, priority)
SELECT service_type, aws_region, model_id, true,
       '["skill_quality_assessment"]'::jsonb,
       '{}'::jsonb,
       use_inference_profile, rate_limit_seconds, 0
FROM ai_configs
WHERE enabled = true AND capabilities @> '["*"]'::jsonb
LIMIT 1
ON CONFLICT DO NOTHING;
