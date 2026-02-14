-- Add metrics fields to agents table for heartbeat monitoring
-- These fields will be updated by agents during their heartbeat/ping calls

ALTER TABLE agents 
ADD COLUMN IF NOT EXISTS cpu_usage DECIMAL(5,2),
ADD COLUMN IF NOT EXISTS memory_usage DECIMAL(5,2),
ADD COLUMN IF NOT EXISTS running_tasks JSONB DEFAULT '[]'::jsonb,
ADD COLUMN IF NOT EXISTS metrics_updated_at TIMESTAMP;

COMMENT ON COLUMN agents.cpu_usage IS 'CPU usage percentage (0-100)';
COMMENT ON COLUMN agents.memory_usage IS 'Memory usage percentage (0-100)';
COMMENT ON COLUMN agents.running_tasks IS 'Array of currently running task IDs and types';
COMMENT ON COLUMN agents.metrics_updated_at IS 'Last time metrics were updated';
