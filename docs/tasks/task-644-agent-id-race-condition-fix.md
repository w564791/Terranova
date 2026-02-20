# Task 644: Agent ID Race Condition Fix

## Problem Summary

Apply tasks were not skipping init even when running on the same agent as the plan task, because `task.AgentID` was nil during execution.

## Root Cause Analysis

### The Race Condition

In `TaskQueueManager.pushTaskToAgent()`, there was a critical timing issue:

```go
// OLD CODE (WRONG ORDER):
// 1. Send task to agent via C&C channel
SendTaskToAgent(agentID, taskID, ...)

// 2. Set agent_id and save to database
task.AgentID = &selectedAgent.AgentID
m.db.Save(task)
```

**The Problem:**
1. Server sends task notification to agent via C&C channel
2. Agent receives notification and **immediately** calls `GetTaskData` API
3. At this moment, `agent_id` hasn't been saved to database yet
4. Agent gets task data with `agent_id = nil`
5. Server then saves `agent_id` to database (too late!)

### Why This Caused Init to Run

In `terraform_executor.go`, the logic checks:
```go
if task.AgentID == nil || planTask.AgentID == nil || *task.AgentID != *planTask.AgentID {
    // Different agent, must run init
}
```

Since `task.AgentID` was nil, it always triggered init even on the same agent.

## Solution

### Fix Applied

Reordered the operations in `TaskQueueManager.pushTaskToAgent()`:

```go
// NEW CODE (CORRECT ORDER):
// 1. Set agent_id and save to database FIRST
task.Status = models.TaskStatusRunning
task.AgentID = &selectedAgent.AgentID
if err := m.db.Save(task).Error; err != nil {
    // Handle error and retry
    return nil
}

// 2. THEN send task to agent via C&C channel
if err := m.agentCCHandler.SendTaskToAgent(...); err != nil {
    // Rollback task status if send fails
    task.Status = models.TaskStatusPending
    task.AgentID = nil
    m.db.Save(task)
    return nil
}
```

### Key Changes

1. **Save Before Send**: Agent ID is now saved to database BEFORE sending task notification
2. **Rollback on Failure**: If sending fails, task status is rolled back to pending
3. **Atomic Operation**: Ensures agent always gets correct agent_id when calling GetTaskData

## Files Modified

1. `backend/services/task_queue_manager.go`
   - Moved agent_id assignment and DB save before SendTaskToAgent
   - Added rollback logic if send fails
   - Added detailed logging for debugging

## Previous Fixes (All Required)

This fix builds on 6 previous fixes that were all necessary:

1.  `backend/services/remote_data_accessor.go` - UpdateTask adds plan_hash and plan_task_id
2.  `backend/services/terraform_executor.go` - ExecuteApply adds Agent ID check
3.  `backend/internal/handlers/agent_handler.go` - UpdateTaskStatus receives fields
4.  `backend/internal/handlers/agent_handler.go` - GetPlanTask returns plan_hash and agent_id
5.  `backend/services/agent_api_client.go` - GetPlanTask parses fields
6.  `backend/internal/handlers/agent_handler.go` - GetTaskData returns agent_id
7.  **THIS FIX** - TaskQueueManager saves agent_id before sending task

## Testing Instructions

1. Deploy the updated backend
2. Create a plan+apply task
3. Check agent logs during apply phase:
   - Should see: `Same agent detected, can skip init`
   - Should NOT see: `Different agent detected, must run init`
4. Verify init is skipped and working directory is reused

## Expected Behavior

### Before Fix
```
[INFO] Different agent detected, must run init:
[INFO]   - Plan agent: agent-pool-xxx
[INFO]   - Apply agent: (none)
[INFO] Running terraform init...
```

### After Fix
```
[INFO] Same agent detected, can skip init
[INFO]   - Plan agent: agent-pool-xxx
[INFO]   - Apply agent: agent-pool-xxx
[INFO] Reusing working directory: /tmp/terraform-xxx
[INFO] Skipping terraform init (optimization)
```

## Impact

- **Performance**: Eliminates unnecessary init during apply phase
- **Reliability**: Ensures agent_id is always available when needed
- **Correctness**: Fixes race condition in task assignment flow

## Completion Status

 **COMPLETE** - All 7 fixes applied, race condition resolved
