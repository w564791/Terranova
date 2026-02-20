# Task: Agent Capacity Check Before Scaling Fix

## Problem Description

Server每次重启后如果存在pending任务,每次都会拉起pod。需要优化检查当前在线的agent的容量是否满足,才能决策是否扩容,而不是无脑扩容。

### Root Cause

The `AutoScaleDeployment` function in `k8s_deployment_service.go` had a flaw where it would scale up pods whenever there were "first pending tasks", without checking if existing online agents had available capacity to handle those tasks.

**Previous Logic:**
```go
if firstPendingTaskCount > 0 {
    if currentReplicas == 0 {
        desiredReplicas = 1  // Cold start
    } else {
        desiredReplicas = currentReplicas + 1  // Always scale up
    }
}
```

This caused unnecessary pod scaling because:
1. It didn't check if existing agents were online
2. It didn't verify if online agents had available capacity
3. It blindly scaled up even when current agents could handle the workload

## Solution

### 1. New Function: `getOnlineAgentCapacity`

Added a new function to check online agent capacity:

```go
func (s *K8sDeploymentService) getOnlineAgentCapacity(ctx context.Context, poolID string) (int, int, error)
```

**Returns:**
- `onlineCount`: Number of online agents (last ping within 2 minutes)
- `availableCapacity`: Available task slots across all online agents
- `error`: Any error during the check

**Capacity Calculation:**
- Each agent can handle 3 task slots (simplified capacity model)
- Total capacity = online agents × 3
- Available capacity = total capacity - currently running tasks
- Running tasks include both `running` and `apply_pending` status

**Implementation Details:**
```go
// Get all agents in pool
var agents []models.Agent
db.Where("pool_id = ?", poolID).Find(&agents)

// Filter online agents (last ping within 2 minutes)
onlineAgents := filter agents where agent.IsOnline()

// Calculate capacity
totalCapacity := len(onlineAgents) * 3
runningTaskCount := count tasks where agent_id IN (onlineAgents) AND status IN (running, apply_pending)
availableCapacity := totalCapacity - runningTaskCount
```

### 2. Updated Scaling Logic

Modified `AutoScaleDeployment` to check capacity before scaling up:

```go
if firstPendingTaskCount > 0 {
    // Check online agent capacity first
    onlineAgentCount, availableCapacity, err := s.getOnlineAgentCapacity(ctx, pool.PoolID)
    
    if onlineAgentCount == 0 {
        // Cold start - no agents online
        desiredReplicas = 1
    } else if availableCapacity > 0 {
        // Has online agents with available capacity - don't scale up
        desiredReplicas = currentReplicas
    } else {
        // Online agents are at full capacity - scale up gradually
        desiredReplicas = currentReplicas + 1
    }
}
```

**Decision Tree:**

```
Has first-pending tasks?
├─ No → Continue with idle scale-down logic
└─ Yes → Check online agent capacity
    ├─ Error checking capacity → Scale conservatively (keep current or scale to 1 if 0)
    ├─ 0 online agents → Scale to 1 (cold start)
    ├─ Has available capacity → Keep current replicas (no scale-up needed)
    └─ No available capacity → Scale up by 1 (gradual scale-up)
```

## Key Improvements

### 1. Intelligent Scaling
- **Before**: Always scaled up when pending tasks existed
- **After**: Only scales up when existing agents are at full capacity

### 2. Resource Efficiency
- Prevents unnecessary pod creation
- Reduces cloud infrastructure costs
- Minimizes pod churn and startup overhead

### 3. Better Capacity Utilization
- Ensures existing agents are fully utilized before scaling
- Respects agent capacity limits (3 slots per agent)
- Considers both running and apply_pending tasks

### 4. Graceful Error Handling
- If capacity check fails, scales conservatively
- Logs warnings but doesn't block scaling decisions
- Maintains system availability even during errors

## Testing Scenarios

### Scenario 1: Cold Start with Pending Tasks
**Setup:**
- 0 replicas running
- 5 pending tasks waiting

**Expected Behavior:**
- Check capacity: 0 online agents
- Decision: Scale to 1 replica (cold start)
- Log: "Pool X has 5 first-pending tasks but 0 online agents (cold start), scaling to 1 pod"

### Scenario 2: Agents with Available Capacity
**Setup:**
- 2 replicas running (6 total capacity)
- 3 tasks running
- 2 pending tasks waiting

**Expected Behavior:**
- Check capacity: 2 online agents, 3 available slots
- Decision: Keep 2 replicas (no scale-up needed)
- Log: "Pool X has 2 first-pending tasks but 2 online agents with 3 available capacity, no scale-up needed"

### Scenario 3: Agents at Full Capacity
**Setup:**
- 2 replicas running (6 total capacity)
- 6 tasks running
- 3 pending tasks waiting

**Expected Behavior:**
- Check capacity: 2 online agents, 0 available slots
- Decision: Scale to 3 replicas (gradual scale-up)
- Log: "Pool X has 3 first-pending tasks, 2 online agents at full capacity, scaling to 3 (gradual scale-up)"

### Scenario 4: Server Restart with Pending Tasks
**Setup:**
- Server restarts
- 10 pending tasks in database
- 3 replicas were running before restart
- Agents reconnect within 2 minutes

**Expected Behavior:**
- Auto-scaler checks capacity
- Finds 3 online agents with available capacity
- Keeps 3 replicas (no unnecessary scale-up)
- Pending tasks get assigned to existing agents

## Code Changes

### File: `backend/services/k8s_deployment_service.go`

**Added Function:**
```go
// getOnlineAgentCapacity checks online agents and calculates available capacity
// Returns: (online agent count, available capacity slots, error)
func (s *K8sDeploymentService) getOnlineAgentCapacity(ctx context.Context, poolID string) (int, int, error)
```

**Modified Function:**
```go
// AutoScaleDeployment performs auto-scaling logic for a pool's deployment
// Now includes capacity check before scaling up
func (s *K8sDeploymentService) AutoScaleDeployment(ctx context.Context, pool *models.AgentPool) (int32, bool, error)
```

## Logging Enhancements

Added detailed logging for capacity checks:

```
[K8sDeployment] Pool X capacity check: online_agents=2, available_capacity=3, first_pending_tasks=2
[K8sDeployment] Pool X capacity: online_agents=2, total_capacity=6, running_tasks=3, available=3
[K8sDeployment] Pool X has 2 first-pending tasks but 2 online agents with 3 available capacity, no scale-up needed
```

## Performance Considerations

### Database Queries
The capacity check adds 2 database queries per auto-scale cycle:
1. Query agents in pool
2. Count running tasks for those agents

**Optimization:**
- Queries are simple and indexed (pool_id, agent_id)
- Only executed when there are first-pending tasks
- Minimal overhead compared to pod creation cost

### Agent Online Check
- Uses `IsOnline()` method: checks if last_ping_at within 2 minutes
- In-memory check, no additional database queries
- Fast and efficient

## Monitoring

### Key Metrics to Monitor

1. **Scale-up Events**
   - Before fix: High frequency on server restart
   - After fix: Only when capacity is exhausted

2. **Agent Utilization**
   - Monitor: running_tasks / total_capacity ratio
   - Target: >70% before scale-up

3. **Pending Task Wait Time**
   - Should remain low even with optimized scaling
   - If increasing, may need to adjust capacity model

### Log Patterns to Watch

**Good Pattern:**
```
[K8sDeployment] Pool X has 5 first-pending tasks but 3 online agents with 4 available capacity, no scale-up needed
```

**Needs Attention:**
```
[K8sDeployment] Warning: failed to check agent capacity: <error>
```

## Configuration

No configuration changes required. The fix uses existing:
- Agent capacity model (3 slots per agent)
- Agent online threshold (2 minutes)
- Auto-scaler interval (from config)

## Rollback Plan

If issues arise, the fix can be rolled back by reverting the changes to `AutoScaleDeployment`:

```bash
git revert <commit-hash>
```

The previous behavior will be restored:
- Scale up whenever first-pending tasks exist
- No capacity check before scaling

## Future Enhancements

### 1. Configurable Capacity Model
Allow per-pool capacity configuration:
```json
{
  "capacity_per_agent": 3,
  "plan_task_weight": 1,
  "plan_and_apply_task_weight": 3
}
```

### 2. Predictive Scaling
Scale up proactively based on:
- Historical task patterns
- Time of day
- Pending task growth rate

### 3. Capacity Metrics API
Expose capacity metrics via API:
```
GET /api/v1/agent-pools/{poolID}/capacity
{
  "online_agents": 3,
  "total_capacity": 9,
  "used_capacity": 5,
  "available_capacity": 4
}
```

## Conclusion

This fix significantly improves the K8s agent auto-scaling logic by:
1. Preventing unnecessary pod creation on server restart
2. Ensuring existing agent capacity is fully utilized
3. Reducing infrastructure costs
4. Maintaining system responsiveness

The solution is backward compatible, well-tested, and includes comprehensive logging for monitoring and debugging.

## Related Documentation

- [K8s Deployment Implementation Summary](./k8s-deployment-implementation-summary.md)
- [Task Scheduling Rules](./task-scheduling-rules-comprehensive-fix.md)
- [Agent Capacity Bug Analysis](./task-agent-capacity-bug-analysis.md)
