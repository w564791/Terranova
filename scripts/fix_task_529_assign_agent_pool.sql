-- Fix task 529 pending issue by assigning agent pool to workspace
-- Issue: Workspace ws-mb7m9ii5ey has execution_mode=k8s but no agent_pool_id

-- Step 1: Check current workspace configuration
SELECT workspace_id, name, execution_mode, agent_pool_id, k8s_config_id 
FROM workspaces 
WHERE workspace_id = 'ws-mb7m9ii5ey';

-- Step 2: Assign the K8s agent pool to the workspace
-- Using pool-z73eh8ihywlmgx0x which is a K8s pool that was used successfully in previous tasks
UPDATE workspaces 
SET agent_pool_id = 'pool-z73eh8ihywlmgx0x'
WHERE workspace_id = 'ws-mb7m9ii5ey';

-- Step 3: Verify the update
SELECT workspace_id, name, execution_mode, agent_pool_id, k8s_config_id 
FROM workspaces 
WHERE workspace_id = 'ws-mb7m9ii5ey';

-- Step 4: Check the pending task
SELECT id, workspace_id, task_type, status, stage, execution_mode, agent_id, created_at
FROM workspace_tasks 
WHERE workspace_id = 'ws-mb7m9ii5ey' AND status = 'pending'
ORDER BY created_at DESC;

-- Note: After running this script, you may need to manually trigger task execution
-- by either:
-- 1. Restarting the backend server, OR
-- 2. Creating a new task (which will trigger TryExecuteNextTask), OR  
-- 3. Waiting for the next agent heartbeat/polling cycle
