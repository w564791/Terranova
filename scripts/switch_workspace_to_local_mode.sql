-- Switch workspace to Local mode for testing
-- This allows us to test the plan_data/plan_json save fix

UPDATE workspaces 
SET execution_mode = 'local', 
    current_pool_id = NULL
WHERE workspace_id = 'ws-0yrm628p8h3f9mw0';

-- Verify the change
SELECT workspace_id, name, execution_mode, current_pool_id 
FROM workspaces 
WHERE workspace_id = 'ws-0yrm628p8h3f9mw0';
