-- Add one-time unfreeze fields to agent_pools table
-- This allows emergency bypass of freeze schedules without deleting them

ALTER TABLE agent_pools 
ADD COLUMN IF NOT EXISTS one_time_unfreeze_until TIMESTAMP,
ADD COLUMN IF NOT EXISTS one_time_unfreeze_by VARCHAR(50),
ADD COLUMN IF NOT EXISTS one_time_unfreeze_at TIMESTAMP;

COMMENT ON COLUMN agent_pools.one_time_unfreeze_until IS 'Temporary unfreeze expiration time (single-use emergency bypass)';
COMMENT ON COLUMN agent_pools.one_time_unfreeze_by IS 'User who activated the one-time unfreeze';
COMMENT ON COLUMN agent_pools.one_time_unfreeze_at IS 'When the one-time unfreeze was activated';
