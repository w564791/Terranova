# Task 515: Agent Auto-Scaling Bug Analysis

## Bug Description

When 2 workspaces have plan+apply tasks simultaneously, only 1 agent is auto-scaled instead of 2.

**Expected Behavior**: If there are 2 pending tasks from 2 different workspaces, the K8s deployment should scale to 2 replicas.

**Actual Behavior**: Only 1 agent is created, causing one workspace's task to wait.

## Root Cause Analysis

### The Problem

In `backend/services/k8s_deployment_service.go`, the `CountPendingTasksForPool` function has a critical flaw:

```go
func (s *K8sDeploymentService) CountPendingTasksForPool(ctx context.Context, poolID string) (int64, error) {
	var count int64

	err := s.db.WithContext(ctx).
		Model(&models.WorkspaceTask{}).
		Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
		Where("workspace_tasks.status = ?", models.TaskStatusPending).
		Where("workspaces.current_pool_id = ?", poolID).
		Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
		// Exclude tasks that are blocked/waiting for user action
		Where("workspace_tasks.status NOT IN (?)", []models.TaskStatus{
			models.TaskStatusWaiting,      // 等待前置任务完成
			models.TaskStatusApplyPending, // Plan完成,等待用户确认Apply
		}).
		Count(&count).Error

	return count, nil
}
```

**The Issue**: This query only counts tasks with `status = 'pending'`, but it ALSO excludes tasks with status in `['waiting', 'apply_pending']`. This is redundant and correct.

However, the real problem is in the **task queue logic**:

### Task Queue Blocking Logic

In `backend/services/task_queue_manager.go`, the `GetNextExecutableTask` function has this logic:

```go
// 2. 检查plan_and_apply pending任务（如果可以执行）
var planAndApplyTask models.WorkspaceTask
err := m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
    workspaceID, models.TaskTypePlanAndApply, models.TaskStatusPending).
    Order("created_at ASC").
    First(&planAndApplyTask).Error

if err == nil {
    // 找到plan_and_apply任务
    // 检查是否有其他plan_and_apply任务正在执行中（排除pending状态）
    // 只有running、apply_pending状态才会阻塞
    var blockingTasks []models.WorkspaceTask
    m.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND task_type = ? AND id != ? AND status IN (?)",
            workspaceID,
            models.TaskTypePlanAndApply,
            planAndApplyTask.ID,
            []models.TaskStatus{models.TaskStatusRunning, models.TaskStatusApplyPending}).
        Find(&blockingTasks)

    if len(blockingTasks) > 0 {
        // Cannot execute - blocked by other tasks
        return nil, nil
    }
}
```

**The Real Problem**: When workspace A has a plan_and_apply task in `pending` status and workspace B also has a plan_and_apply task in `pending` status:

1. Auto-scaler counts: 2 pending tasks → should scale to 2 agents
2. Agent 1 connects and picks up workspace A's task → task becomes `running`
3. Agent 2 should connect, but the auto-scaler runs BEFORE agent 2 connects
4. When auto-scaler runs again, it counts:
   - Workspace A: 0 pending tasks (task is now `running`)
   - Workspace B: 1 pending task
   - Total: 1 pending task → scales to 1 agent

**The issue is timing**: The auto-scaler counts pending tasks, but by the time it scales up, some tasks may have already been picked up by agents, causing the count to be lower than needed.

### The Correct Fix

The auto-scaler should count **both pending AND running tasks** to determine the required number of agents, because:

1. Running tasks need agents to continue executing
2. Pending tasks need agents to start executing
3. The total number of agents should be: `pending_count + running_count`

However, we need to be careful:
- We should NOT count `apply_pending` tasks (they're waiting for user confirmation)
- We should NOT count `waiting` tasks (they're blocked by other tasks)
- We SHOULD count `pending` and `running` tasks

## Solution

Modify `CountPendingTasksForPool` to count both `pending` and `running` tasks:

```go
func (s *K8sDeploymentService) CountPendingTasksForPool(ctx context.Context, poolID string) (int64, error) {
	var count int64

	// Count tasks that need agents:
	// 1. pending - waiting to be picked up
	// 2. running - currently being executed
	// Exclude:
	// - waiting: blocked by other tasks
	// - apply_pending: waiting for user confirmation
	// - success/applied/failed/cancelled: completed
	err := s.db.WithContext(ctx).
		Model(&models.WorkspaceTask{}).
		Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
		Where("workspaces.current_pool_id = ?", poolID).
		Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
		Where("workspace_tasks.status IN (?)", []models.TaskStatus{
			models.TaskStatusPending,
			models.TaskStatusRunning,
		}).
		Count(&count).Error

	if err != nil {
		return 0, fmt.Errorf("failed to count active tasks: %w", err)
	}

	return count, nil
}
```

## Testing Plan

1. Create 2 workspaces (ws1, ws2) both using the same K8s agent pool
2. Create plan+apply tasks for both workspaces simultaneously
3. Verify that the auto-scaler scales to 2 agents
4. Verify that both tasks are executed concurrently
5. Verify that after both tasks complete, the auto-scaler scales down to min_replicas

## Impact Analysis

- **Positive**: Fixes the auto-scaling bug, ensures sufficient agents are available
- **Risk**: May cause slight over-provisioning if tasks complete very quickly
- **Mitigation**: The auto-scaler runs periodically and will scale down when tasks complete
