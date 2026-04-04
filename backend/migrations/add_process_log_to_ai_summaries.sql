ALTER TABLE ai_plan_summaries ADD COLUMN IF NOT EXISTS process_log text;
ALTER TABLE ai_apply_summaries ADD COLUMN IF NOT EXISTS process_log text;
COMMENT ON COLUMN ai_plan_summaries.process_log IS 'AI analysis process log (thinking, tool calls, results)';
COMMENT ON COLUMN ai_apply_summaries.process_log IS 'AI analysis process log (thinking, tool calls, results)';
