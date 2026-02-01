# Task 599: Pod Auto-Scaling Cold Start Issue Fix

## Issue Summary

**Problem**: Pod auto-scaling fails to start pods when pool has no existing pods (cold start scenario)

**Symptom**:
```
[TaskQueue] No free slot available for task 599: no free slot available in pool pool-z73eh8ihywlmgx0x, will retry
```

**Root Cause**: The `AutoScalePods()` function has a logic flaw in the cold start path that prevents it from creating the first pod when:
1. Pool has 0 pods (cold start)
2. Pending tasks exist
3. The complex "first pending task" query may fail or return 0

## Detailed Analysis

### Current Code Flow (k8s_deployment_service.go:AutoScalePods)

```go
if totalSlots == 0 || currentPodCount == 0 {
    // Check for pending tasks with complex subquery
    var firstPendingTaskCount int64
    err := m.db.WithContext(ctx).
        Table("workspace_tasks AS wt1").
        Joins("JOIN workspaces ON workspaces.workspace_id = wt1.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
        Where("wt1.status = ?", models.TaskStatusPending).
        Where(`NOT EXISTS (...)`).  // Complex subquery
        Count(&firstPendingTaskCount).Error

    if err == nil && firstPendingTaskCount > 0 {
        desiredPodCount = k8sConfig.MinReplicas
        if desiredPodCount < 1 {
            desiredPodCount = 1
        }
    } else {
        desiredPodCount = 0  // ❌ This prevents cold start!
    }
}
```

### Problems

1. **Complex Query**: The "first pending task" query uses a NOT EXISTS subquery that may be slow or fail
2. **Query Failure Handling**: If the query fails (`err != nil`), it falls through to `desiredPodCount = 0`
3. **No Simple Pending Check**: Doesn't check for ANY pending tasks as a fallback
4. **Min Replicas Ignored**: When query returns 0, it sets `desiredPodCount = 0` even if `min_replicas > 0`

## Solution

### Fix 1: Simplify Cold Start Logic

Replace the complex query with a simple check for ANY pending tasks:

```go
if totalSlots == 0 || currentPodCount == 0 {
    // Simple check: do we have ANY pending tasks?
    var pendingTaskCount int64
    err := m.db.WithContext(ctx).
        Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
        Where("workspace_tasks.status = ?", models.TaskStatusPending).
        Count(&pendingTaskCount).Error

    if err != nil {
        log.Printf("[K8sPodService] Error checking pending tasks: %v", err)
        // On error, respect min_replicas
        desiredPodCount = k8sConfig.MinReplicas
    } else if pendingTaskCount > 0 {
        // Has pending tasks, start with min_replicas or 1
        desiredPodCount = k8sConfig.MinReplicas
        if desiredPodCount < 1 {
            desiredPodCount = 1
        }
        log.Printf("[K8sPodService] Pool %s cold start: %d pending tasks, scaling to %d pods",
            poolID, pendingTaskCount, desiredPodCount)
    } else {
        // No pending tasks, respect min_replicas
        desiredPodCount = k8sConfig.MinReplicas
        log.Printf("[K8sPodService] Pool %s cold start: no pending tasks, scaling to min_replicas=%d",
            poolID, desiredPodCount)
    }
}
```

### Fix 2: Respect min_replicas in All Cases

Ensure `min_replicas` is always respected, even when there are no pending tasks:

```go
// Always respect min_replicas constraint
if desiredPodCount < k8sConfig.MinReplicas {
    desiredPodCount = k8sConfig.MinReplicas
}
```

### Fix 3: Add Heartbeat Grace Period for New Pods

In `FindPodWithFreeSlot()`, add grace period for newly created pods:

```go
// Check Agent heartbeat with grace period for new pods
timeSinceCreation := time.Since(pod.CreatedAt)
timeSinceHeartbeat := time.Since(pod.LastHeartbeat)

// Grace period: 5 minutes for new pods to register
if timeSinceCreation < 5*time.Minute {
    // New pod, allow it even without recent heartbeat
    log.Printf("[PodManager] Pod %s is new (created %v ago), allowing without heartbeat check",
        pod.PodName, timeSinceCreation)
} else if timeSinceHeartbeat > 2*time.Minute {
    // Old pod without recent heartbeat, skip it
    log.Printf("[PodManager] Pod %s is offline (last heartbeat: %v ago), skipping",
        pod.PodName, timeSinceHeartbeat)
    continue
}
```

## Implementation Plan

1.  Update `AutoScalePods()` cold start logic
2.  Simplify pending task query
3.  Always respect min_replicas
4.  Add heartbeat grace period for new pods
5.  Add better logging for debugging

## Testing

### Test Case 1: Cold Start with Pending Tasks
- Pool has 0 pods
- Create a pending task
- Auto-scaler should create min_replicas pods (or 1 if min_replicas=0)

### Test Case 2: Cold Start without Pending Tasks
- Pool has 0 pods
- No pending tasks
- Auto-scaler should create min_replicas pods

### Test Case 3: New Pod Registration
- Pod is created
- Agent hasn't registered yet (no heartbeat)
- Task allocation should succeed within 5-minute grace period

## Files Modified

1. `backend/services/k8s_deployment_service.go` - AutoScalePods() logic
2. `backend/services/k8s_pod_manager.go` - FindPodWithFreeSlot() heartbeat check

## Verification

After fix, check logs for:
```
[K8sPodService] Pool xxx cold start: N pending tasks, scaling to M pods
[PodManager] Pod xxx is new (created Xs ago), allowing without heartbeat check
[K8sPodService] Created pod with new config for pool xxx (now M/N)
```

## Related Issues

- Task 599: Original issue report
- Phase 2 Pod Slot Management: Related feature implementation
