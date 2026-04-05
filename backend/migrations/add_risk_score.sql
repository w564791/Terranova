-- Migration: add deterministic risk scoring columns (no seed data)

-- Deterministic risk scoring
ALTER TABLE ai_plan_summaries
  ADD COLUMN IF NOT EXISTS risk_score_value FLOAT,
  ADD COLUMN IF NOT EXISTS risk_score_color VARCHAR(10) DEFAULT '',
  ADD COLUMN IF NOT EXISTS risk_score_breakdown JSONB;

-- AI analysis completeness (orthogonal to risk scoring)
ALTER TABLE ai_plan_summaries
  ADD COLUMN IF NOT EXISTS ai_analysis_incomplete BOOLEAN DEFAULT false,
  ADD COLUMN IF NOT EXISTS bypassed_by VARCHAR(20),
  ADD COLUMN IF NOT EXISTS bypassed_at TIMESTAMP,
  ADD COLUMN IF NOT EXISTS bypass_reason TEXT;

COMMENT ON COLUMN ai_plan_summaries.risk_score_value IS 'Deterministic risk score (0-100)';
COMMENT ON COLUMN ai_plan_summaries.risk_score_color IS 'Decision color: green/yellow/orange/red';
COMMENT ON COLUMN ai_plan_summaries.risk_score_breakdown IS 'Deduction breakdown (category/item/points/reason per item)';
COMMENT ON COLUMN ai_plan_summaries.ai_analysis_incomplete IS 'AI analysis incomplete flag (requires admin bypass)';
COMMENT ON COLUMN ai_plan_summaries.bypassed_by IS 'Admin user_id who bypassed';
COMMENT ON COLUMN ai_plan_summaries.bypass_reason IS 'Bypass reason (required)';
