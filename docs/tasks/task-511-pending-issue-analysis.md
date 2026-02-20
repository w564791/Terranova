# Task 511 Pending Issue Analysis

## Issue Summary
Task 511 in workspace `ws-mb7m9ii5ey` is stuck in pending status and cannot be executed.

## Root Cause
The workspace is configured with `execution_mode = k8s`, but the `k8s_config_id` field is **empty/null**.

### Database Evidence
```sql
SELECT workspace_id, name, execution_mode, k8s_config_id, is_locked 
FROM workspaces 
WHERE workspace_id = 'ws-mb7m9ii5ey';

-- Result:
workspace_id  | name | execution_mode | k8s_config_id | is_locked
--------------+------+----------------+---------------+-----------
ws-mb7m9ii5ey | asas | k8s            |               | f
```

### Task Status
```sql
SELECT id, workspace_id, task_type, status, stage, execution_mode, agent_id, k8s_pod_name
FROM workspace_tasks 
WHERE id = 511;

-- Result:
id  | workspace_id  | task_type      | status  | stage   | execution_mode | agent_id | k8s_pod_name
----+---------------+----------------+---------+---------+----------------+----------+--------------
511 | ws-mb7m9ii5ey | plan_and_apply | pending | pending | k8s            |          |
```

### Log Evidence
```
2025/11/07 10:23:06 [K8sDeployment] Error auto-scaling pool pool-ppnx8qkk9utci10w: 
pool pool-ppnx8qkk9utci10w does not have K8s configuration
```

This error repeats every 30 seconds, showing the system is trying to schedule the task but failing.

## Solutions

### Option 1: Configure K8s for the Workspace (Recommended)
1. Go to the workspace settings page
2. Navigate to the "Execution" or "Agent Pool" section
3. Configure a K8s configuration for the workspace
4. The task should automatically start executing once K8s is configured

### Option 2: Change Execution Mode to Agent
If you don't need K8s execution, change the workspace to use agent mode:

```sql
-- Update workspace to use agent mode
UPDATE workspaces 
SET execution_mode = 'agent' 
WHERE workspace_id = 'ws-mb7m9ii5ey';
```

After this change, you'll need to:
1. Assign an agent pool to the workspace
2. The task should then be picked up by an available agent

### Option 3: Cancel the Task
If you don't need this task anymore:

```sql
-- Cancel the stuck task
UPDATE workspace_tasks 
SET status = 'cancelled',
    completed_at = NOW(),
    error_message = 'Cancelled - workspace missing K8s configuration'
WHERE id = 511;
```

## Prevention
To prevent this issue in the future:
1. Always configure K8s settings before setting `execution_mode = 'k8s'`
2. Add validation in the UI to prevent saving K8s execution mode without K8s configuration
3. Add a database constraint or trigger to enforce this relationship

## Related Code
- `backend/services/k8s_deployment_service.go` - K8s deployment logic
- `backend/services/task_queue_manager.go` - Task scheduling logic
- `backend/controllers/workspace_task_controller.go` - Task creation logic
