# Task 500: Critical Bug Fix - Apply Pending Tasks Auto-Execute After Server Restart

## Issue Description

**CRITICAL BUG:** Tasks in `apply_pending` status were being automatically executed after server restart, bypassing user confirmation. This is a severe security and operational issue.

### Expected Behavior
- Tasks in `apply_pending` status should ONLY execute when user explicitly confirms apply
- Server restart should NOT trigger automatic execution of `apply_pending` tasks
- Only `pending` status tasks should be recovered and executed on server restart

### Actual Behavior (Bug)
- After server restart, `apply_pending` tasks were automatically pushed to agents and executed
- This bypassed the user confirmation step entirely
- Users lost control over when apply operations would run

## Root Cause Analysis

The bug was in the `RecoverPendingTasks()` function in `backend/services/task_queue_manager.go`:

### Problematic Code (Before Fix)

```go
func (m *TaskQueueManager) RecoverPendingTasks() error {
    // 1. Cleanup orphan tasks
    m.CleanupOrphanTasks()
    
    // 2. Get ALL workspaces with pending OR apply_pending tasks
    m.db.Model(&models.WorkspaceTask{}).
        Where("status IN ?", []models.TaskStatus{
            models.TaskStatusPending, 
            models.TaskStatusApplyPending  // ❌ BUG: Including apply_pending!
        }).
        Distinct("workspace_id").
        Pluck("workspace_id", &workspaceIDs)
    
    // 3. For EACH workspace, call TryExecuteNextTask()
    for _, wsID := range workspaceIDs {
        go m.TryExecuteNextTask(wsID)  // ❌ This causes auto-apply!
    }
}
```

### Why This Caused Auto-Execution

1. `RecoverPendingTasks()` included workspaces with `apply_pending` status tasks
2. It then called `TryExecuteNextTask()` for these workspaces
3. `TryExecuteNextTask()` → `GetNextExecutableTask()` found the `apply_pending` task
4. The task was automatically pushed to agent and executed WITHOUT user confirmation

### Task Flow (Buggy Behavior)

```
1. User creates plan_and_apply task
2. Plan completes → status = apply_pending, stage = apply_pending
3. User has NOT confirmed apply yet
4.  Server restarts
5. RecoverPendingTasks() runs
6. ❌ Finds workspace with apply_pending task
7. ❌ Calls TryExecuteNextTask(workspace)
8. ❌ GetNextExecutableTask() returns the apply_pending task
9. ❌ Task is pushed to agent and executed automatically
10. ❌ User confirmation was bypassed!
```

## Solution

Modified `RecoverPendingTasks()` to EXCLUDE `apply_pending` status tasks from recovery:

### Fixed Code

```go
func (m *TaskQueueManager) RecoverPendingTasks() error {
    // 1. Cleanup orphan tasks
    m.CleanupOrphanTasks()
    
    // 2. Get workspaces with ONLY pending tasks (exclude apply_pending)
    var workspaceIDs []string
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", models.TaskStatusPending).  //  Only pending!
        Distinct("workspace_id").
        Pluck("workspace_id", &workspaceIDs)
    
    log.Printf("[TaskQueue] Recovering pending tasks for %d workspaces (excluding apply_pending tasks)", 
        len(workspaceIDs))
    
    // 3. For each workspace, try to execute next task
    for _, wsID := range workspaceIDs {
        go m.TryExecuteNextTask(wsID)
    }
    
    // 4. Log apply_pending tasks (not auto-executed)
    var applyPendingCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", models.TaskStatusApplyPending).
        Count(&applyPendingCount)
    
    if applyPendingCount > 0 {
        log.Printf("[TaskQueue] Found %d apply_pending tasks waiting for user confirmation (will not auto-execute)", 
            applyPendingCount)
    }
    
    return nil
}
```

### Key Changes

1. **Changed WHERE clause**: `status IN (pending, apply_pending)` → `status = pending`
2. **Added logging**: Explicitly log that apply_pending tasks are excluded
3. **Added monitoring**: Count and log apply_pending tasks that are waiting
4. **Added comments**: Clear documentation of the behavior

## Task Flow (Fixed Behavior)

```
1. User creates plan_and_apply task
2. Plan completes → status = apply_pending, stage = apply_pending
3. User has NOT confirmed apply yet
4.  Server restarts
5. RecoverPendingTasks() runs
6.  Skips workspace with apply_pending task (not in recovery list)
7.  Task remains in apply_pending status
8.  User can still confirm apply when ready
9.  Only after user confirmation will task execute
```

## Status and Stage Mapping

| Status | Stage | Recovered on Restart? | Action on Restart |
|--------|-------|----------------------|-------------------|
| `pending` | `pending` |  Yes | Execute task |
| `running` | `planning` | ❌ No | Mark as failed (orphan) |
| `running` | `applying` | ❌ No | Mark as failed (orphan) |
| `apply_pending` | `apply_pending` |  **No (Fixed)** | **Keep waiting for user** |
| `success` | N/A | N/A | No action needed |
| `applied` | N/A | N/A | No action needed |
| `failed` | N/A | N/A | No action needed |

## Testing Scenarios

### Scenario 1: Server Restart During Plan Execution
1. Create plan_and_apply task
2. Task starts executing plan (status=running, stage=planning)
3. Restart server
4. **Expected:** Task marked as failed 
5. **Actual:** Task marked as failed 

### Scenario 2: Server Restart While Waiting for Apply Confirmation (CRITICAL)
1. Create plan_and_apply task
2. Plan completes (status=apply_pending, stage=apply_pending)
3. **DO NOT confirm apply**
4. Restart server
5. **Expected:** Task remains in apply_pending status, waiting for user 
6. **Actual (Before Fix):** Task auto-executed ❌
7. **Actual (After Fix):** Task remains in apply_pending status 
8. User can still confirm apply 

### Scenario 3: Server Restart During Apply Execution
1. Create plan_and_apply task
2. Plan completes, user confirms apply
3. Task starts executing apply (status=running, stage=applying)
4. Restart server
5. **Expected:** Task marked as failed 
6. **Actual:** Task marked as failed 

### Scenario 4: Server Restart With Pending Task
1. Create plan_and_apply task (status=pending)
2. Task has not started yet
3. Restart server
4. **Expected:** Task is recovered and starts executing 
5. **Actual:** Task is recovered and starts executing 

## Logging Examples

### Before Fix (Buggy Logs)
```
[TaskQueue] Recovering pending tasks for 3 workspaces
[TaskQueue] Attempting to recover tasks for workspace ws_xxx
[TaskQueue] GetNextExecutableTask for workspace ws_xxx
[TaskQueue] Found apply_pending task 500 for workspace ws_xxx
[TaskQueue] Selected agent agent_yyy from pool pool_zzz
[TaskQueue] Successfully pushed task 500 to agent agent_yyy (action: apply)
❌ Task auto-executed without user confirmation!
```

### After Fix (Correct Logs)
```
[TaskQueue] Recovering pending tasks for 2 workspaces (excluding apply_pending tasks)
[TaskQueue] Found 1 apply_pending tasks waiting for user confirmation (will not auto-execute)
[TaskQueue] Attempting to recover tasks for workspace ws_aaa
[TaskQueue] Attempting to recover tasks for workspace ws_bbb
 apply_pending tasks are NOT recovered
 User must explicitly confirm apply
```

## Impact Assessment

### Security Impact
- **HIGH**: Prevented unauthorized apply operations
- **HIGH**: Restored user control over infrastructure changes
- **HIGH**: Eliminated risk of unintended resource modifications

### Operational Impact
- **HIGH**: Server can now be safely restarted without triggering applies
- **MEDIUM**: Users can leave tasks in apply_pending state indefinitely
- **LOW**: No impact on normal task execution flow

### User Experience Impact
- **POSITIVE**: Users maintain full control over apply operations
- **POSITIVE**: No unexpected infrastructure changes after server restart
- **POSITIVE**: Clear logging shows which tasks are waiting for confirmation

## Related Issues

This fix is related to but distinct from Task 497:
- **Task 497**: Fixed `CleanupOrphanTasks()` to not mark apply_pending as failed
- **Task 500**: Fixed `RecoverPendingTasks()` to not auto-execute apply_pending tasks

Both fixes work together to ensure apply_pending tasks are handled correctly:
1. Task 497: Don't mark them as failed on restart
2. Task 500: Don't auto-execute them on restart

## Files Modified

- `backend/services/task_queue_manager.go` - Modified `RecoverPendingTasks()` function

## Deployment Notes

1. **No database migration required**
2. **No configuration changes needed**
3. **Backward compatible** - existing tasks will work correctly
4. **Immediate effect** - fix applies on next server restart
5. **Safe to deploy** - no breaking changes

## Verification Steps

1. Create a plan_and_apply task
2. Wait for plan to complete (status becomes apply_pending)
3. **DO NOT confirm apply**
4. Restart the server
5. Check task status - should still be apply_pending
6. Check server logs - should see "excluding apply_pending tasks"
7. Confirm apply manually - task should execute normally

## Prevention Measures

To prevent similar bugs in the future:

1. **Code Review**: Always review task recovery logic carefully
2. **Testing**: Test server restart scenarios with tasks in all states
3. **Logging**: Add clear logging for task recovery decisions
4. **Documentation**: Document which task states should/shouldn't be recovered
5. **Monitoring**: Monitor for unexpected task executions after restarts

## Conclusion

This was a critical bug that could have caused:
- Unauthorized infrastructure changes
- Loss of user control
- Unexpected costs
- Security violations

The fix ensures that:
-  apply_pending tasks wait for user confirmation
-  Server restarts are safe
-  Users maintain full control
-  No unexpected executions

## Date

2025-01-06
