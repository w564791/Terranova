# Task 511 Pending Bug Fix

## Bug Description

任务创建后一直处于 pending 状态，无法被调度执行。即使 agent 在线且配置正确，任务也不会被分配。

## Root Cause

在 `CreatePlanTask` 和 `ConfirmApply` 中，任务创建后会启动一个 goroutine 来调用 `TryExecuteNextTask`：

```go
go func() {
    if err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID); err != nil {
        log.Printf("[ERROR] Failed to start task execution...")
    }
}()
```

**问题**：
1. 如果 goroutine 执行失败（返回 error），**没有任何重试机制**
2. 如果 `TryExecuteNextTask` 在某些临界时刻失败（比如 Agent C&C Handler 还未完全初始化），任务会永远卡在 pending 状态
3. 错误只会被记录到日志，但任务不会被重新调度

## Fix Implementation

### 1. 添加重试机制

在 `CreatePlanTask` 和 `ConfirmApply` 中添加了带指数退避的重试逻辑：

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("[PANIC] TryExecuteNextTask panicked...")
        }
    }()
    
    // 添加重试机制：最多重试3次，每次间隔递增
    maxRetries := 3
    for attempt := 0; attempt <= maxRetries; attempt++ {
        if attempt > 0 {
            // 指数退避：1s, 2s, 4s
            waitTime := time.Duration(1<<uint(attempt-1)) * time.Second
            log.Printf("[TaskQueue] Retry attempt %d/%d after %v", attempt, maxRetries, waitTime)
            time.Sleep(waitTime)
        }
        
        err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID)
        if err == nil {
            // 成功，退出重试循环
            log.Printf("[TaskQueue] Successfully triggered task execution (attempt %d)", attempt+1)
            return
        }
        
        log.Printf("[ERROR] Failed to start task execution (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
        
        // 如果是最后一次尝试，记录严重错误
        if attempt == maxRetries {
            log.Printf("[CRITICAL] All %d attempts failed. Task %d may be stuck in pending state.", maxRetries+1, task.ID)
        }
    }
}()
```

### 2. 修复编译错误

修复了 `task_queue_manager.go` 中 `GetNextExecutableTask` 函数的 `err` 变量未声明问题：

```go
// 修复前
err = m.db.Where(...).First(&planAndApplyTask).Error

// 修复后
err := m.db.Where(...).First(&planAndApplyTask).Error
```

## Safety Guarantees for Apply Pending Tasks

### 重启 Server 或 Agent 的安全性

** 完全安全！apply_pending 任务不会被自动执行**

#### 证据 1: GetNextExecutableTask 明确排除 apply_pending

```go
// GetNextExecutableTask 获取下一个可执行的任务
// 注意：apply_pending 任务需要用户确认，不会被自动返回
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // 注意：不检查 apply_pending 任务！
    // apply_pending 任务需要用户通过 ConfirmApply 显式确认后才能执行
    // 这样可以防止服务器重启时自动执行未经用户确认的 apply 操作
    
    // 只查询 status = 'pending' 的任务
    err := m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
        workspaceID, models.TaskTypePlanAndApply, models.TaskStatusPending).
        // ...
}
```

#### 证据 2: RecoverPendingTasks 只恢复 pending 任务

```go
// RecoverPendingTasks 系统启动时恢复pending任务
// 注意：只恢复真正pending的任务，不包括apply_pending状态的任务
func (m *TaskQueueManager) RecoverPendingTasks() error {
    // 2. 获取所有有pending任务的workspace（排除apply_pending状态）
    var workspaceIDs []string
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", models.TaskStatusPending).  // 只查询 pending，不包括 apply_pending
        Distinct("workspace_id").
        Pluck("workspace_id", &workspaceIDs)
    
    // 4. 记录apply_pending任务的数量（这些任务不会自动恢复）
    var applyPendingCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", models.TaskStatusApplyPending).
        Count(&applyPendingCount)
    
    if applyPendingCount > 0 {
        log.Printf("[TaskQueue] Found %d apply_pending tasks waiting for user confirmation (will not auto-execute)", applyPendingCount)
    }
}
```

#### 证据 3: CleanupOrphanTasks 保护 apply_pending 任务

```go
// CleanupOrphanTasks 清理孤儿任务
func (m *TaskQueueManager) CleanupOrphanTasks() error {
    // 查找所有running状态的任务
    // 注意：不包括apply_pending状态
    var orphanTasks []models.WorkspaceTask
    err := m.db.Where("status = ?", models.TaskStatusRunning).Find(&orphanTasks).Error
    
    for _, task := range orphanTasks {
        // 如果任务的stage是apply_pending，不标记为失败
        if task.Stage == "apply_pending" {
            log.Printf("[TaskQueue] Skipping task %d - waiting for user confirmation")
            
            // 将状态重置为apply_pending
            task.Status = models.TaskStatusApplyPending
            m.db.Save(&task)
            continue
        }
        
        // 其他任务标记为failed
        task.Status = models.TaskStatusFailed
        // ...
    }
}
```

### 结论

**重启 Server 或 Agent 完全安全**：

1.  `apply_pending` 状态的任务**永远不会**被 `GetNextExecutableTask` 返回
2.  `RecoverPendingTasks` **只恢复** `status = 'pending'` 的任务
3.  `CleanupOrphanTasks` 会**保护** `apply_pending` 任务，不会标记为失败
4.  `apply_pending` 任务**只能**通过用户在前端点击 "Confirm Apply" 按钮来触发执行

## Testing

### 测试步骤

1. 编译并重启后端服务：
   ```bash
   cd backend
   make build
   # 重启服务
   ```

2. 观察日志，应该看到：
   ```
   [TaskQueue] Recovering pending tasks for X workspaces (excluding apply_pending tasks)
   [TaskQueue] Attempting to recover tasks for workspace ws-mb7m9ii5ey
   [TaskQueue] Successfully triggered task execution for workspace ws-mb7m9ii5ey (attempt 1)
   ```

3. 任务 511 应该会被调度并开始执行

### 验证 apply_pending 安全性

1. 创建一个 plan_and_apply 任务
2. 等待 plan 完成，任务进入 `apply_pending` 状态
3. 重启后端服务
4. 确认任务仍然是 `apply_pending` 状态，没有自动执行 apply

## Files Changed

1. `backend/controllers/workspace_task_controller.go`
   - `CreatePlanTask`: 添加重试机制（最多3次，指数退避）
   - `ConfirmApply`: 添加重试机制（最多3次，指数退避）

2. `backend/services/task_queue_manager.go`
   - `GetNextExecutableTask`: 修复 err 变量声明
   - `pushTaskToAgent`: 添加 `task.AgentID` 赋值（修复 agent_id 为空的问题）

3. `backend/internal/handlers/agent_cc_handler_raw.go`
   - `handleConnection`: 添加 agent 断开连接时的任务清理
   - `cleanupAgentTasks`: 新增函数，清理断开连接的 agent 正在执行的任务

## Related Issues

- Task 497: Server restart apply pending fix
- Task 500: Apply pending auto-execute bug fix
- Task 510: Apply pending restart fix

这些之前的修复都确保了 apply_pending 任务不会在服务器重启时自动执行。
