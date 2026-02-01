# Task 497: Server Restart Apply Pending Fix

## Issue Description

When the server restarts, tasks in `apply_pending` status were incorrectly being marked as failed. According to the design requirements:

1. After the plan phase completes, if the task is in `apply_pending` status, the server CAN restart safely
2. Only after the user confirms apply should the task enter the scheduling queue and be executed on an agent
3. Tasks should only be marked as failed if they were actually running (not just waiting for user confirmation)

## Root Cause

The `CleanupOrphanTasks()` function in `task_queue_manager.go` was marking ALL `running` status tasks as failed during server restart, without distinguishing between:
- Tasks that were actually executing (should be marked as failed)
- Tasks that were waiting for user confirmation in `apply_pending` status (should NOT be marked as failed)

## Solution

Modified the `CleanupOrphanTasks()` function to:

1. Check the task's `stage` field in addition to the `status` field
2. Skip tasks with `stage = "apply_pending"` - these are waiting for user confirmation
3. Reset their status to `apply_pending` if it was incorrectly set to `running`
4. Only mark tasks as failed if they were actually executing (not in apply_pending stage)

## Code Changes

### File: `backend/services/task_queue_manager.go`

**Modified Function:** `CleanupOrphanTasks()`

**Key Changes:**
```go
// Check if task is waiting for user confirmation
if task.Stage == "apply_pending" {
    log.Printf("[TaskQueue] Skipping task %d - waiting for user confirmation")
    
    // Reset status to apply_pending if needed
    task.Status = models.TaskStatusApplyPending
    m.db.Save(&task)
    skippedCount++
    continue
}

// Only mark as failed if actually running
task.Status = models.TaskStatusFailed
task.ErrorMessage = "Task interrupted by server restart"
task.CompletedAt = timePtr(time.Now())
m.db.Save(&task)
failedCount++
```

## Task Flow

### Before Fix:
1. User creates plan_and_apply task
2. Plan completes → status becomes `apply_pending`, stage becomes `apply_pending`
3. **Server restarts**
4. ❌ Task incorrectly marked as failed
5. User cannot confirm apply

### After Fix:
1. User creates plan_and_apply task
2. Plan completes → status becomes `apply_pending`, stage becomes `apply_pending`
3. **Server restarts**
4.  Task status remains `apply_pending` (not marked as failed)
5. User can still confirm apply
6. After confirmation, task enters scheduling queue
7. Task is scheduled on agent and executed

## Status and Stage Mapping

| Status | Stage | Can Restart? | Action on Restart |
|--------|-------|--------------|-------------------|
| `pending` | `pending` |  Yes | Keep as pending |
| `running` | `planning` | ❌ No | Mark as failed |
| `running` | `applying` | ❌ No | Mark as failed |
| `apply_pending` | `apply_pending` |  Yes | **Keep as apply_pending** |
| `success` | N/A |  Yes | No action needed |
| `applied` | N/A |  Yes | No action needed |
| `failed` | N/A |  Yes | No action needed |

## Testing Scenarios

### Scenario 1: Server Restart During Plan Execution
1. Create plan_and_apply task
2. Task starts executing plan (status=running, stage=planning)
3. Restart server
4. **Expected:** Task marked as failed 

### Scenario 2: Server Restart While Waiting for Apply Confirmation
1. Create plan_and_apply task
2. Plan completes (status=apply_pending, stage=apply_pending)
3. Restart server
4. **Expected:** Task remains in apply_pending status 
5. User can still confirm apply 

### Scenario 3: Server Restart During Apply Execution
1. Create plan_and_apply task
2. Plan completes, user confirms apply
3. Task starts executing apply (status=running, stage=applying)
4. Restart server
5. **Expected:** Task marked as failed 

## Logging

The fix includes detailed logging:
```
[TaskQueue] Found X orphan tasks to check
[TaskQueue] Skipping task Y (workspace ws_xxx, type plan_and_apply, stage apply_pending) - waiting for user confirmation
[TaskQueue] Marked orphan task Z (workspace ws_xxx, type plan_and_apply, stage planning) as failed
[TaskQueue] Cleanup complete: X tasks marked as failed, Y tasks skipped (waiting for confirmation)
```

## Impact

- **Positive:** Tasks waiting for user confirmation are no longer lost on server restart
- **Positive:** Users can safely restart the server without losing pending apply confirmations
- **No Breaking Changes:** Existing functionality remains intact
- **Backward Compatible:** Works with existing tasks in the database

## Related Files

- `backend/services/task_queue_manager.go` - Main fix
- `backend/controllers/workspace_task_controller.go` - Task creation and confirmation logic
- `backend/services/terraform_executor.go` - Task execution logic

## Deployment Notes

1. No database migration required
2. No configuration changes needed
3. Server restart will automatically apply the fix
4. Existing tasks in apply_pending status will be preserved

## Date

2025-01-06
