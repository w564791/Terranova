# Task 572 Cancelled Status Bug Analysis

## Issue Summary
Task 572 was cancelled by the user during plan execution, but the database status remains "running" instead of being updated to "cancelled".

## Database Evidence

### Task Status Query
```sql
SELECT id, workspace_id, stage, status, agent_id, created_at, updated_at, started_at, completed_at 
FROM workspace_tasks 
WHERE id = '572';
```

**Result:**
- **id**: 572
- **workspace_id**: ws-mb7m9ii5ey
- **stage**: planning
- **status**: running ❌ (Should be "cancelled")
- **agent_id**: agent-pool-z73eh8ihywlmgx0x-1762507433188454000
- **created_at**: 2025-11-07 17:23:58.236979
- **updated_at**: 2025-11-07 17:28:47.707955
- **started_at**: 2025-11-07 17:26:27.792599
- **completed_at**: 2025-11-07 17:28:47.687091 ✓ (Has completion time)

### Task Log Evidence
The last log entry shows:
```
[09:28:47.689] [INFO] Task cancelled by user during plan execution
```

This confirms the task was cancelled at 2025-11-07 09:28:47 (UTC), which matches the `completed_at` timestamp.

## Root Cause Analysis

The issue indicates a **status update failure** during task cancellation. When a task is cancelled:

1.  The `completed_at` timestamp is set correctly
2.  The cancellation is logged to task_logs
3. ❌ The `status` field is NOT updated from "running" to "cancelled"

This creates an **inconsistent state** where:
- The task has a completion timestamp (indicating it's finished)
- The status still shows "running" (indicating it's active)
- The logs show it was cancelled

## Potential Causes

### 1. Race Condition in Status Update
The cancellation handler may be updating `completed_at` but failing to update `status` due to:
- Transaction rollback
- Database connection issues
- Concurrent updates from agent

### 2. Missing Status Update in Cancel Handler
The cancel endpoint may not be properly updating the status field when setting the completion timestamp.

### 3. Agent Not Reporting Final Status
If the task is running in K8s agent mode, the agent may have:
- Received the cancel signal
- Set the completion time
- Failed to report the final "cancelled" status back to the server

## Impact

This bug causes:
1. **UI Confusion**: Users see the task as "running" when it's actually cancelled
2. **Resource Leaks**: The system may think the agent is still busy with this task
3. **Scheduling Issues**: Task queue manager may not schedule new tasks correctly
4. **Metrics Errors**: Task statistics and dashboards show incorrect data

## Related Code Areas to Investigate

1. **Cancel Handler**: `backend/controllers/workspace_task_controller.go`
   - Check the `CancelTask` method
   - Verify status update logic

2. **Agent Cancel Logic**: `backend/services/agent_api_client.go`
   - Check how agent reports cancellation status
   - Verify status synchronization

3. **Task Status Update**: `backend/services/workspace_lifecycle.go`
   - Check `UpdateTaskStatus` method
   - Verify transaction handling

4. **WebSocket Updates**: `backend/internal/websocket/hub.go`
   - Check if status updates are broadcast correctly

## Recommended Fix

### Immediate Fix (Manual)
Update the task status directly:
```sql
UPDATE workspace_tasks 
SET status = 'cancelled' 
WHERE id = 572 
  AND status = 'running' 
  AND completed_at IS NOT NULL;
```

### Code Fix Required
1. Ensure cancel handler updates both `completed_at` AND `status` atomically
2. Add validation: if `completed_at` is set, status must not be "running" or "pending"
3. Add recovery logic: periodic job to fix inconsistent states
4. Add logging to track status update failures

## Testing Recommendations

1. Test task cancellation during different phases (init, plan, apply)
2. Test cancellation in both agent modes (local and k8s)
3. Verify status updates are atomic
4. Check WebSocket broadcasts for status changes
5. Monitor for similar issues in other tasks

## Related Issues

This may be related to previous fixes:
- task-510: Agent cancel task bug fix
- task-518: Agent cancel complete fix
- task-511: Pending issue analysis

The pattern suggests there may be a systemic issue with status updates during task lifecycle transitions.
