# Task 604: Slot Detection Bug Fix - Complete

## Problem Summary

Task 604 was stuck in pending state despite the pool reporting 2 idle slots available. This was caused by a race condition between Pod creation and Agent registration.

## Root Cause

The issue was in `k8s_pod_manager.go`'s `FindPodWithFreeSlot` function:

1. **Grace Period Logic**: New Pods (< 5 minutes old) were allowed without checking if their agent had registered
2. **Agent Connectivity Check**: The task queue checked for connected agents via C&C handler
3. **Race Condition**: 
   - `FindPodWithFreeSlot` found a slot on a new Pod
   - Slot was allocated successfully
   - But `pushTaskToAgent` found no connected agents (agent still starting)
   - Slot was released and task retried indefinitely

## The Fix

Modified `FindPodWithFreeSlot` in `backend/services/k8s_pod_manager.go` to check if the agent has registered before considering a new Pod:

```go
// Grace period: 5 minutes for new pods to register and start sending heartbeats
if timeSinceCreation < 5*time.Minute {
    // New pod - check if agent has registered
    if pod.AgentID == "" {
        log.Printf("[PodManager] Pod %s is new (created %v ago) but agent not registered yet, skipping",
            pod.PodName, timeSinceCreation)
        continue
    }
    log.Printf("[PodManager] Pod %s is new (created %v ago) with registered agent %s, allowing",
        pod.PodName, timeSinceCreation, pod.AgentID)
} else if timeSinceHeartbeat > 2*time.Minute {
    // Old pod without recent heartbeat, skip it
    log.Printf("[PodManager] Pod %s is offline (last heartbeat: %v ago), skipping",
        pod.PodName, timeSinceHeartbeat)
    continue
}
```

## Changes Made

### File: `backend/services/k8s_pod_manager.go`

**Function**: `FindPodWithFreeSlot`

**Change**: Added agent registration check for new Pods within the 5-minute grace period

**Before**:
- New Pods were allowed without checking if agent had registered
- This caused slots to be allocated to Pods whose agents weren't connected yet

**After**:
- New Pods are only considered if `pod.AgentID != ""` (agent has registered)
- This ensures slot allocation and agent assignment are aligned
- Tasks won't be assigned to Pods whose agents haven't connected yet

## Impact

### Positive Effects

1. **Fixes Race Condition**: Tasks no longer get stuck when Pods are starting up
2. **Aligned Logic**: Slot detection now matches agent connectivity requirements
3. **Better Autoscaling**: When no Pods have connected agents, autoscaler will create new Pods
4. **Improved Reliability**: Reduces retry loops and wasted slot allocations

### Risk Assessment

- **Low Risk**: Only affects new Pod detection logic
- **No Breaking Changes**: Existing behavior for established Pods remains unchanged
- **Backward Compatible**: Works with existing agent registration flow

## Expected Behavior After Fix

### Scenario 1: Cold Start (No Pods)
1. Task is created
2. Autoscaler detects pending task
3. Creates new Pod
4. Task waits for agent to register
5. Once agent registers, task is assigned

### Scenario 2: Pod Exists But Agent Not Connected
1. Task is created
2. `FindPodWithFreeSlot` skips Pod (agent not registered)
3. Returns "no free slot available"
4. Autoscaler may create additional Pod if needed
5. Task waits and retries
6. Once agent registers, task is assigned

### Scenario 3: Pod Exists With Connected Agent
1. Task is created
2. `FindPodWithFreeSlot` finds available slot
3. Slot is allocated
4. Agent is confirmed connected
5. Task is assigned immediately

## Testing Recommendations

1. **Cold Start Test**:
   - Create workspace with pending task
   - Verify Pod is created
   - Verify task waits for agent registration
   - Verify task is assigned after agent connects

2. **Concurrent Tasks Test**:
   - Create multiple pending tasks
   - Verify proper slot allocation
   - Verify no tasks get stuck in retry loops

3. **Agent Restart Test**:
   - Stop agent while task is pending
   - Verify task waits for agent to reconnect
   - Verify task is assigned after reconnection

4. **Scale-Up Test**:
   - Create more tasks than available slots
   - Verify autoscaler creates new Pods
   - Verify tasks are distributed across Pods

## Monitoring

Watch for these log messages to verify the fix is working:

### Success Indicators
```
[PodManager] Pod X is new (created Ys ago) with registered agent Z, allowing
[TaskQueue] Allocated slot N on pod X for task Y
[TaskQueue] Successfully pushed task Y to agent Z
```

### Expected During Startup
```
[PodManager] Pod X is new (created Ys ago) but agent not registered yet, skipping
[TaskQueue] No free slot available for task Y: no free slot available in pool Z, will retry
```

### Should Not See (Indicates Problem)
```
[TaskQueue] No connected agents found, task X will retry
```
(This should only appear if truly no agents are connected, not during normal startup)

## Related Issues

This fix addresses the same class of issues as:
- Task 598: Apply pending slot reuse
- Task 599: Pod autoscaling cold start
- Task 601: Apply pending stuck diagnosis

All of these involved timing issues between Pod creation, agent registration, and task assignment.

## Deployment Notes

1. **No Database Changes**: This is a code-only fix
2. **No Configuration Changes**: No changes to K8s configs or agent settings
3. **Restart Required**: Backend server restart required to apply the fix
4. **Zero Downtime**: Can be deployed without affecting running tasks

## Verification Steps

After deployment:

1. Check logs for the new log messages
2. Create a test workspace with a pending task
3. Verify task is assigned after agent connects
4. Monitor for any tasks stuck in pending state
5. Check autoscaler behavior with multiple pending tasks

## Success Criteria

-  No tasks stuck in pending state when slots are available
-  Tasks wait for agent registration before assignment
-  Autoscaler creates new Pods when needed
-  No unnecessary retry loops
-  Proper slot utilization reporting

## Conclusion

This fix resolves the race condition between Pod creation and agent registration by ensuring that `FindPodWithFreeSlot` only considers Pods whose agents have successfully registered. This aligns the slot detection logic with the agent connectivity requirements, preventing tasks from getting stuck in retry loops.

The fix is minimal, low-risk, and addresses the root cause of the issue without requiring changes to the agent registration flow or autoscaling logic.
