-- Drop agent-level authorization tables
-- These tables are no longer used as the system has migrated to pool-level authorization

-- Drop agent_allowed_workspaces table
DROP TABLE IF EXISTS agent_allowed_workspaces CASCADE;

-- Drop workspace_allowed_agents table  
DROP TABLE IF EXISTS workspace_allowed_agents CASCADE;

-- Verification
SELECT 'Agent authorization tables dropped successfully' AS status;
