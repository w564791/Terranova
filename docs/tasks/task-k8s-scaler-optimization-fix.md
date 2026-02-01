# K8s Agent Pod Scaler Optimization Fix

## Issue Summary

Two critical issues were identified in the K8s agent pod auto-scaler:

1. **Pending Pod Logic Issue**: The scaler was counting `pending` tasks when determining required agents, but pending tasks may be blocked by workspace locks or other constraints, not necessarily waiting for available agents. This caused unnecessary scale-up operations.

2. **Scale-Down Safety Issue**: When scaling down, K8s terminates pods in an unordered manner, which could terminate agents that are actively executing tasks, causing task failures.

## Root Cause Analysis

### Issue 1: Incorrect Pending Task Counting

**Location**: `backend/services/k8s_deployment_service.go` - `CountPendingTasksForPool()`

**Problem**:
```go
// OLD CODE - Counted both pending AND running tasks
Where("workspace_tasks.status IN (?)", []models.TaskStatus{
    models.TaskStatusPending,  // ❌ Should not count pending
    models.TaskStatusRunning,
})
```

**Why this is wrong**:
- A task in `pending` status may be blocked by:
  - Workspace lock held by another task
  - Resource constraints
  - Scheduling delays
  - Other business logic constraints
- These pending tasks don't need new agents - they're waiting for other conditions to be met
- Counting them causes the scaler to spin up unnecessary agent pods

### Issue 2: Unsafe Scale-Down

**Location**: `backend/services/k8s_deployment_service.go` - `AutoScaleDeployment()`

**Problem**:
- When scaling down (e.g., from 3 replicas to 1), K8s randomly selects which pods to terminate
- No check was performed to ensure the terminated pods weren't actively executing tasks
- This could kill an agent mid-execution, causing task failures

**Example Scenario**:
```
Current state:
- 3 agent pods running
- Pod A: executing task #123 (running)
- Pod B: idle
- Pod C: idle

Scale-down decision: reduce to 1 replica
K8s randomly terminates: Pod A and Pod B
Result: Task #123 fails because Pod A was killed mid-execution ❌
```

## Solution Implementation

### Fix 1: Count Only Running Tasks

**Changes in `CountPendingTasksForPool()`**:

```go
// NEW CODE - Only count RUNNING tasks
// Plan tasks
Where("workspace_tasks.status = ?", models.TaskStatusRunning)

// Plan_and_apply tasks  
Where("workspace_tasks.status IN (?)", []models.TaskStatus{
    models.TaskStatusRunning,      // Active execution
    models.TaskStatusApplyPending, // Waiting for user confirmation
})
```

**Rationale**:
- Only `running` tasks indicate actual agent workload
- `apply_pending` tasks are included because the agent must stay alive to execute the apply after user confirmation
- `pending` tasks are excluded - they'll transition to `running` when conditions are met

### Fix 2: Add Pre-Scale-Down Safety Check

**New function `hasAnyAgentWithRunningTasks()`**:

```go
func (s *K8sDeploymentService) hasAnyAgentWithRunningTasks(ctx context.Context, poolID string) (bool, error) {
    // Check if there are any running tasks assigned to agents in this pool
    var count int64
    err := s.db.WithContext(ctx).
        Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
        Where("workspace_tasks.agent_id IS NOT NULL AND workspace_tasks.agent_id != ''").
        Where("workspace_tasks.status IN (?)", []models.TaskStatus{
            models.TaskStatusRunning,
            models.TaskStatusApplyPending,
        }).
        Count(&count).Error

    if err != nil {
        return false, fmt.Errorf("failed to check for running tasks: %w", err)
    }

    if count > 0 {
        log.Printf("[K8sDeployment] Pool %s has %d tasks with assigned agents in running/apply_pending status", poolID, count)
    }

    return count > 0, nil
}
```

**Integration in `AutoScaleDeployment()`**:

```go
// FIX: Before scaling DOWN, verify no agents have running tasks
// K8s scale-down is unordered and may terminate pods with active tasks
if desiredReplicas < currentReplicas {
    hasRunningTasks, err := s.hasAnyAgentWithRunningTasks(ctx, pool.PoolID)
    if err != nil {
        log.Printf("[K8sDeployment] Warning: failed to check for running tasks before scale-down: %v", err)
        // Don't block scale-down on check failure, but log it
    } else if hasRunningTasks {
        log.Printf("[K8sDeployment] Skipping scale-down for pool %s: agents still have running tasks (current=%d, desired=%d)",
            pool.PoolID, currentReplicas, desiredReplicas)
        return currentReplicas, false, nil
    }
}
```

**Key Features**:
- Only checks when scaling DOWN (`desiredReplicas < currentReplicas`)
- Looks for tasks with `agent_id` assigned (actively being executed)
- Checks both `running` and `apply_pending` statuses
- Blocks scale-down if ANY agent has active tasks
- Logs warning if check fails but doesn't block (fail-safe)

## Impact Analysis

### Before Fix

**Scale-Up Behavior**:
- ❌ Counted pending tasks → unnecessary agent pods created
- ❌ Wasted resources on idle agents
- ❌ Increased infrastructure costs

**Scale-Down Behavior**:
- ❌ Could terminate agents with running tasks
- ❌ Caused task failures and retries
- ❌ Poor user experience

### After Fix

**Scale-Up Behavior**:
-  Only counts running tasks → accurate capacity planning
-  Agents created only when actually needed
-  Optimized resource utilization

**Scale-Down Behavior**:
-  Verifies no active tasks before scaling down
-  Prevents task failures from premature termination
-  Safe and reliable scaling

## Testing Recommendations

### Test Case 1: Pending Task Handling

**Setup**:
1. Create workspace with lock enabled
2. Start task A (will acquire lock and run)
3. Start task B (will be pending due to lock)

**Expected Behavior**:
- Before fix: 2 agents created (1 for running, 1 for pending)
- After fix: 1 agent created (only for running task)

**Verification**:
```bash
# Check agent count
kubectl get pods -n terraform -l pool-id=<pool-id>

# Check logs
grep "capacity calculation" backend.log
```

### Test Case 2: Scale-Down Safety

**Setup**:
1. Start 3 tasks to scale up to 3 agents
2. Complete 2 tasks (2 agents become idle)
3. Wait for scale-down cycle

**Expected Behavior**:
- Before fix: May scale down to 1 agent, potentially killing the busy agent
- After fix: Waits until all tasks complete before scaling down

**Verification**:
```bash
# Monitor scale-down attempts
grep "Skipping scale-down" backend.log

# Verify no task failures during scale-down
SELECT * FROM workspace_tasks 
WHERE status = 'failed' 
AND error_message LIKE '%agent%terminated%'
ORDER BY created_at DESC;
```

### Test Case 3: Mixed Task Types

**Setup**:
1. Start 2 plan tasks (should share 1 agent, capacity = 3 per agent)
2. Start 1 plan_and_apply task (requires dedicated agent)

**Expected Behavior**:
- Required agents = max(ceil(2/3), 1) = max(1, 1) = 1 agent initially
- When plan_and_apply starts running: 1 agent
- Both types can coexist on same agent if capacity allows

**Verification**:
```bash
# Check capacity calculation logs
grep "capacity calculation" backend.log | tail -5
```

## Monitoring

### Key Metrics to Watch

1. **Agent Pod Count**:
   ```bash
   kubectl get pods -n terraform -l component=agent --watch
   ```

2. **Scale Events**:
   ```bash
   grep "Auto-scaled pool" backend.log | tail -20
   ```

3. **Scale-Down Blocks**:
   ```bash
   grep "Skipping scale-down" backend.log | tail -20
   ```

4. **Task Failures**:
   ```sql
   SELECT COUNT(*) as failure_count, 
          DATE(created_at) as date
   FROM workspace_tasks 
   WHERE status = 'failed'
   AND created_at > NOW() - INTERVAL 7 DAY
   GROUP BY DATE(created_at)
   ORDER BY date DESC;
   ```

### Alert Thresholds

- **High pending task count**: If pending tasks > 10 for > 5 minutes, investigate workspace locks
- **Frequent scale-down blocks**: If scale-down blocked > 5 times in 10 minutes, check task completion
- **Task failures**: Any increase in task failures should trigger investigation

## Rollback Plan

If issues are detected after deployment:

1. **Immediate Rollback**:
   ```bash
   git revert <commit-hash>
   make build
   make restart-backend
   ```

2. **Manual Override**:
   - Temporarily set `min_replicas = max_replicas` in pool config to disable auto-scaling
   - Manually scale deployment: `kubectl scale deployment iac-agent-<pool-id> --replicas=N`

3. **Monitoring During Rollback**:
   - Watch for task failures to stop
   - Verify agent count stabilizes
   - Check logs for errors

## Related Files

- `backend/services/k8s_deployment_service.go` - Main implementation
- `backend/internal/models/workspace.go` - Task status definitions
- `backend/services/task_queue_manager.go` - Task scheduling logic

## References

- Task #515: Agent autoscaling bug analysis
- Task #536: Pending issue diagnosis
- K8s Deployment documentation: `docs/k8s-deployment-implementation-summary.md`
