-- Add process_log column to ai_error_analyses for storing Agent Loop execution trace
ALTER TABLE ai_error_analyses ADD COLUMN IF NOT EXISTS process_log text;

COMMENT ON COLUMN ai_error_analyses.process_log IS 'Agent Loop 过程日志：记录每轮工具调用、thinking、结果等';
