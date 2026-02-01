# Task 681 Auto-Apply Bug Analysis

## Issue Summary

**Critical Bug**: Task 681 (plan_and_apply) was automatically executed in apply phase WITHOUT user confirmation via the ConfirmApply API.

## Evidence from Logs

```
[GIN] 2025/11/13 - 21:28:50 | 200 | 7.706875ms | ::1 | GET "/api/v1/workspaces/ws-mb7m9ii5ey/tasks/681"
[GIN] 2025/11/13 - 21:28:50 | 200 | 34.039167ms | ::1 | GET "/api/v1/workspaces/ws-mb7m9ii5ey/tasks/681/comments"
[GIN] 2025/11/13 - 21:28:50 | 200 | 23.237209ms | ::1 | GET "/api/v1/workspaces/ws-mb7m9ii5ey/tasks/681"
...
[GIN] 2025/11/13 - 21:31:05 | 200 | 13.292959ms | ::1 | GET "/api/v1/workspaces/ws-mb7m9ii5ey/tasks/681"
[GIN] 2025/11/13 - 21:31:05 | 200 | 17.678792ms | ::1 | GET "/api/v1/workspaces/ws-mb7m9ii5ey/tasks/681/comments"
[GIN] 2025/11/13 - 21:31:05 | 200 | 33.173ms | ::1 | GET "/api/v1/workspaces/ws-mb7m9ii5ey/tasks/681/resource-changes"
[GIN] 2025/11/13 - 21:36:00 | 204 | 15.583µs | ::1 | OPTIONS "/api/v1/workspaces/ws-mb7m9ii5ey/tasks/681"
```

**Key Observations**:
1. Only GET requests visible (polling task status)
2. NO POST request to `/api/v1/workspaces/{id}/tasks/{task_id}/confirm-apply`
3. Task transitioned from `apply_pending` to `applying` automatically
4. 5-minute gap (21:31:05 to 21:36:00) suggests server restart or recovery

## Database Evidence

Query result from `workspace_tasks` table:

```sql
SELECT id, workspace_id, task_type, status, stage, created_at, started_at, completed_at, apply_description 
FROM workspace_tasks WHERE id = 681;
```

```
 id  | workspace_id  |   task_type    | status  |  stage  |         created_at         |         started_at         |        completed_at        | apply_description
-----+---------------+----------------+---------+---------+----------------------------+----------------------------+----------------------------+-------------------
 681 | ws-mb7m9ii5ey | plan_and_apply | applied | applied | 2025-11-13 21:28:50.370278 | 2025-11-13 21:31:33.088806 | 2025-11-13 13:32:05.329211 |
```

**Critical Finding**: `apply_description` is **NULL/empty**!

This field is ONLY set when user calls the ConfirmApply API with the required `apply_description` parameter. The fact that it's empty proves the task was executed WITHOUT going through the ConfirmApply endpoint.

**Timeline Analysis**:
- **21:28:50** - Task created (plan phase)
- **21:31:05** - Last GET request before gap (plan likely completed, status: apply_pending)
- **21:31:33** - Task started_at (apply phase began) ← **AUTO-EXECUTED WITHOUT CONFIRMATION**
- **13:32:05** - Task completed (note: timezone issue in display, actual time ~21:32:05)
- **21:36:00** - First request after gap (task already completed)

The 2-minute gap (21:31:05 to 21:31:33) indicates server restart triggered RecoverPendingTasks, which incorrectly executed the apply_pending task.

## Root Cause Analysis

### The Bug: RecoverPendingTasks Auto-Executes apply_pending Tasks

Located in `backend/services/task_queue_manager.go`:

```go
// RecoverPendingTasks 系统启动时恢复pending任务
func (m *TaskQueueManager) RecoverPendingTasks() error {
    // 1. 清理孤儿任务（running状态但后端已重启）
    log.Println("[TaskQueue] Cleaning up orphan tasks...")
    if err := m.CleanupOrphanTasks(); err != nil {
        log.Printf("[TaskQueue] Warning: Failed to cleanup orphan tasks: %v", err)
    }

    // 2. 获取所有有pending任务的workspace（排除apply_pending状态）
    // ❌ BUG: Comment says "排除apply_pending状态" but code doesn't exclude it!
    var workspaceIDs []string
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", models.TaskStatusPending).  //  Only gets "pending"
        Distinct("workspace_id").
        Pluck("workspace_id", &workspaceIDs)

    log.Printf("[TaskQueue] Recovering pending tasks for %d workspaces (excluding apply_pending tasks)", len(workspaceIDs))

    // 3. 为每个workspace尝试执行下一个任务
    for _, wsID := range workspaceIDs {
        log.Printf("[TaskQueue] Attempting to recover tasks for workspace %s", wsID)
        go m.TryExecuteNextTask(wsID)  // ❌ This calls GetNextExecutableTask
    }
    
    // ... rest of code
}
```

### The Problem Chain

1. **GetNextExecutableTask** returns `apply_pending` tasks:

```go
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // ...
    
    // 1. 检查plan_and_apply pending/apply_pending任务
    var planAndApplyTask models.WorkspaceTask
    err := m.db.Where("workspace_id = ? AND task_type = ? AND status IN (?)",
        workspaceID, models.TaskTypePlanAndApply, 
        []models.TaskStatus{models.TaskStatusPending, models.TaskStatusApplyPending}).  // ❌ Includes apply_pending!
        Order("created_at ASC").
        First(&planAndApplyTask).Error

    if err == nil {
        // ... no blocking tasks check ...
        return &planAndApplyTask, nil  // ❌ Returns apply_pending task!
    }
    // ...
}
```

2. **pushTaskToAgent** executes apply_pending tasks:

```go
func (m *TaskQueueManager) pushTaskToAgent(task *models.WorkspaceTask, workspace *models.Workspace) error {
    // ...
    
    // 5. Determine action based on task status
    action := "plan"
    if task.Status == models.TaskStatusApplyPending {
        action = "apply"  // ❌ Automatically sets action to "apply"!
    }

    // 6. Update task status to running and assign to agent
    task.Status = models.TaskStatusRunning
    task.StartedAt = timePtr(time.Now())
    task.AgentID = &selectedAgent.AgentID
    if action == "apply" {
        task.Stage = "applying"  // ❌ Automatically starts applying!
        task.PlanTaskID = &task.ID
    }
    
    // ... sends task to agent ...
}
```

## The Execution Flow That Caused the Bug

```
Server Restart (21:31:05)
    ↓
RecoverPendingTasks() called
    ↓
TryExecuteNextTask(workspace_id) for all workspaces with "pending" tasks
    ↓
GetNextExecutableTask() returns task 681 (status: apply_pending)
    ↓
pushTaskToAgent() automatically sets action="apply"
    ↓
Task 681 starts applying WITHOUT user confirmation! (21:36:00)
```

## Why This is Critical

1. **Security Risk**: Apply operations can make destructive changes to infrastructure
2. **Violates User Intent**: User explicitly needs to confirm apply via ConfirmApply API
3. **Bypasses Validation**: ConfirmApply includes resource version snapshot validation
4. **Data Integrity**: Apply should only happen after user reviews plan output

## Design Intent vs Actual Behavior

### Design Intent (from comments):
```go
// 注意：apply_pending 任务需要用户确认，不会被自动返回
// 只有通过 ConfirmApply 显式触发时才会执行
```

### Actual Behavior:
- `GetNextExecutableTask` DOES return `apply_pending` tasks
- `pushTaskToAgent` DOES automatically execute them
- Server restart triggers recovery which bypasses user confirmation

## Impact Assessment

**Severity**: CRITICAL

**Affected Scenarios**:
1. Server restart while task is in `apply_pending` state
2. Manual trigger via `TryExecuteNextTask` (e.g., monitoring scripts)
3. Any code path that calls `GetNextExecutableTask` without checking status

**User Impact**:
- Infrastructure changes applied without explicit user confirmation
- Potential for unintended resource modifications
- Loss of user control over apply timing

## Solution Required

### Option 1: Exclude apply_pending from GetNextExecutableTask (Recommended)

```go
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // ...
    
    // 1. 检查plan_and_apply pending任务（排除apply_pending）
    var planAndApplyTask models.WorkspaceTask
    err := m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
        workspaceID, models.TaskTypePlanAndApply, 
        models.TaskStatusPending).  //  Only pending, not apply_pending
        Order("created_at ASC").
        First(&planAndApplyTask).Error
    // ...
}
```

### Option 2: Add Explicit Check in pushTaskToAgent

```go
func (m *TaskQueueManager) pushTaskToAgent(task *models.WorkspaceTask, workspace *models.Workspace) error {
    // Reject apply_pending tasks that weren't explicitly confirmed
    if task.Status == models.TaskStatusApplyPending {
        return fmt.Errorf("apply_pending tasks require explicit user confirmation via ConfirmApply")
    }
    // ...
}
```

### Option 3: Separate Method for Apply Execution

Create a dedicated method that only ConfirmApply can call:

```go
func (m *TaskQueueManager) ExecuteConfirmedApply(taskID uint) error {
    // Only called from ConfirmApply endpoint
    // Explicitly handles apply_pending -> running transition
}
```

## Recommended Fix

**Use Option 1** as the primary fix because:
1. Aligns with design intent stated in comments
2. Prevents the issue at the source
3. Maintains clear separation: `GetNextExecutableTask` for auto-scheduling, `ConfirmApply` for user-triggered applies
4. Minimal code changes required

**Add Option 2** as a defensive measure:
- Provides additional safety layer
- Makes the intent explicit in code
- Catches any future code paths that might bypass the check

## Testing Requirements

1. **Server Restart Test**:
   - Create plan_and_apply task
   - Wait for plan to complete (status: apply_pending)
   - Restart server
   - Verify task remains in apply_pending (not auto-executed)

2. **Manual Trigger Test**:
   - Create plan_and_apply task in apply_pending state
   - Call TryExecuteNextTask manually
   - Verify task is NOT executed

3. **Normal Flow Test**:
   - Create plan_and_apply task
   - Wait for plan completion
   - Call ConfirmApply API
   - Verify apply executes correctly

4. **Concurrent Task Test**:
   - Create multiple plan_and_apply tasks
   - Verify only pending tasks are auto-scheduled
   - Verify apply_pending tasks wait for confirmation

## Related Code Locations

- `backend/services/task_queue_manager.go`:
  - `GetNextExecutableTask()` - Line ~100
  - `pushTaskToAgent()` - Line ~300
  - `RecoverPendingTasks()` - Line ~600

- `backend/controllers/workspace_task_controller.go`:
  - `ConfirmApply()` - Line ~400

## Prevention Measures

1. Add unit tests for GetNextExecutableTask with apply_pending tasks
2. Add integration tests for server restart scenarios
3. Add explicit status checks in all task execution paths
4. Document the apply_pending state machine clearly
5. Add monitoring/alerting for unexpected status transitions

## Conclusion

This is a critical security and data integrity bug that allows infrastructure changes to be applied without user confirmation. The fix is straightforward but requires careful testing to ensure no regression in normal task scheduling flows.

**Priority**: P0 - Fix immediately
**Risk**: High - Potential for unintended infrastructure changes
**Complexity**: Low - Clear fix with well-defined scope
