-- Add extended thinking configuration to ai_configs
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS thinking_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE ai_configs ADD COLUMN IF NOT EXISTS thinking_budget_tokens integer NOT NULL DEFAULT 10000;

COMMENT ON COLUMN ai_configs.thinking_enabled IS 'Enable extended thinking (Claude thinking mode)';
COMMENT ON COLUMN ai_configs.thinking_budget_tokens IS 'Max tokens for thinking budget (min 1024, default 10000)';

-- Add thinking_content to summary tables for debugging
ALTER TABLE ai_plan_summaries ADD COLUMN IF NOT EXISTS thinking_content jsonb;
ALTER TABLE ai_apply_summaries ADD COLUMN IF NOT EXISTS thinking_content jsonb;
