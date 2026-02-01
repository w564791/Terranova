# Task 515: Agent Auto-Scaling Bug Fix - Complete

## Summary

Fixed a critical bug in the K8s agent auto-scaling logic where only 1 agent was being provisioned when 2 workspaces had plan+apply tasks simultaneously.

## Problem

When 2 workspaces (ws1, ws2) both had plan+apply tasks in pending status:
1. Auto-scaler counted 2 pending tasks → scaled to 2 agents
2. Agent 1 connected and picked up ws1's task → task became `running`
3. Before agent 2 could connect, auto-scaler ran again
4. Auto-scaler only counted ws2's pending task (ws1's was now `running`)
5. Result: Only 1 agent provisioned, ws2's task had to wait

## Root Cause

The `CountPendingTasksForPool` function only counted tasks with `status = 'pending'`, ignoring `running` tasks. This caused a race condition where:
- Tasks that were picked up by agents (status changed to `running`) were no longer counted
- The auto-scaler would scale down or not scale up enough
- Subsequent tasks had to wait for agents to become available

## Solution

Modified `CountPendingTasksForPool` to count **both pending AND running tasks**:

```go
// Count tasks that need agents:
// 1. pending - waiting to be picked up by an agent
// 2. running - currently being executed by an agent
Where("workspace_tasks.status IN (?)", []models.TaskStatus{
    models.TaskStatusPending,
    models.TaskStatusRunning,
})
```

### Why This Works

1. **Running tasks need agents**: Tasks in `running` status are actively being executed and need their agent to continue
2. **Pending tasks need agents**: Tasks in `pending` status are waiting to be picked up
3. **Total agents needed = pending + running**: This ensures sufficient capacity for all active work

### What We Still Exclude

- `waiting`: Blocked by other tasks in the same workspace (don't need agents yet)
- `apply_pending`: Waiting for user confirmation (not executing)
- `success/applied/failed/cancelled`: Completed tasks

## Changes Made

### File: `backend/services/k8s_deployment_service.go`

1. **Updated `CountPendingTasksForPool` function**:
   - Changed from counting only `pending` tasks
   - Now counts both `pending` and `running` tasks
   - Added comprehensive comments explaining the logic

2. **Updated `AutoScaleDeployment` function**:
   - Renamed variable from `pendingCount` to `activeTaskCount` for clarity
   - Updated log message to reflect "active tasks (includes pending+running)"

## Testing Recommendations

1. **Scenario 1: Simultaneous Tasks**
   - Create 2 workspaces using the same K8s pool
   - Create plan+apply tasks for both simultaneously
   - Verify: 2 agents are provisioned
   - Verify: Both tasks execute concurrently

2. **Scenario 2: Sequential Tasks**
   - Create 1 workspace with 2 plan+apply tasks
   - Verify: Only 1 agent is provisioned (tasks are serialized per workspace)

3. **Scenario 3: Scale Down**
   - After tasks complete, verify agents scale down to `min_replicas`

4. **Scenario 4: Max Replicas**
   - Create more tasks than `max_replicas`
   - Verify: Only `max_replicas` agents are provisioned
   - Verify: Remaining tasks wait in queue

## Impact Analysis

### Positive Impacts
-  Fixes the auto-scaling bug for concurrent workspaces
-  Ensures sufficient agent capacity for all active work
-  Improves task execution throughput
-  Better resource utilization

### Potential Concerns
-  May provision slightly more agents during task transitions
-  Agents may stay up slightly longer during scale-down

### Mitigation
- The auto-scaler runs periodically (default: every 10 seconds)
- Agents will scale down once tasks complete
- The `min_replicas` and `max_replicas` constraints still apply

## Related Documentation

- [Task 515 Bug Analysis](./task-515-agent-autoscaling-bug-analysis.md)
- [K8s Deployment Implementation Summary](./k8s-deployment-implementation-summary.md)

## Deployment Notes

1. **No database changes required**
2. **No configuration changes required**
3. **Backward compatible**: Existing deployments will benefit immediately
4. **Restart required**: Backend service needs restart to apply the fix

## Verification

After deployment, monitor the logs for:
```
[K8sDeployment] Auto-scaled pool <pool-id> from X to Y replicas (active tasks: Z, includes pending+running)
```

The "active tasks" count should now include both pending and running tasks.

## Completion Date

2025-11-07

## Status

 **COMPLETE** - Fix implemented and ready for testing
