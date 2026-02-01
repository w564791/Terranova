# Pending Task Execution and Filter Classification Fix

## 问题描述

用户报告了两个问题：

1. **任务pending但没有自动执行**: 只有一个任务#163，状态是pending，但没有自动开始执行
2. **pending任务的过滤器分类错误**: pending任务显示在"Needs Attention"中，但应该在"On Hold"中

## 根本原因分析

### 问题1: Pending任务不自动执行

**原因**: `TaskQueueManager.RecoverPendingTasks()` 从未在系统启动时被调用

- `main.go` 中只初始化了 `OutputStreamManager`
- 没有初始化 `TaskQueueManager`
- 没有调用 `RecoverPendingTasks()` 来恢复系统重启前的pending任务

### 问题2: Filter分类错误

**原因**: `GetTasks` 方法中的filter逻辑将 `pending` 和 `apply_pending` 状态错误地归类到 "Needs Attention"

- "Needs Attention" 应该只包含需要用户操作的状态：`requires_approval`, `plan_completed`
- "On Hold" 应该包含等待执行的状态：`on_hold`, `pending`, `apply_pending`

## 实施的修复

### 1. 初始化TaskQueueManager并恢复Pending任务

**文件**: `backend/main.go`

```go
// 初始化任务队列管理器
executor := services.NewTerraformExecutor(db, streamManager)
queueManager := services.NewTaskQueueManager(db, executor)
log.Println("Task queue manager initialized")

// 恢复pending任务
if err := queueManager.RecoverPendingTasks(); err != nil {
    log.Printf("Warning: Failed to recover pending tasks: %v", err)
}
```

**效果**:
- 系统启动时自动初始化TaskQueueManager
- 自动恢复所有pending和apply_pending任务
- 为每个有pending任务的workspace触发任务执行

### 2. 修复Filter分类逻辑

**文件**: `backend/controllers/workspace_task_controller.go`

**修改前**:
```go
case "needs_attention":
    query = query.Where("status IN ?", []string{"pending", "requires_approval", "plan_completed"})
case "on_hold":
    query = query.Where("status = ?", "on_hold")
```

**修改后**:
```go
case "needs_attention":
    query = query.Where("status IN ?", []string{"requires_approval", "plan_completed"})
case "on_hold":
    query = query.Where("status IN ?", []string{"on_hold", "pending", "apply_pending"})
```

**同时更新filter counts计算**:
```go
// Needs Attention - 只包含需要用户操作的状态
c.db.Model(&models.WorkspaceTask{}).Where("workspace_id = ?", workspaceID).
    Scopes(applySearchAndTimeFilters(search, startDate, endDate)).
    Where("status IN ?", []string{"requires_approval", "plan_completed"}).Count(&count)
filterCounts["needs_attention"] = count

// On Hold - 包含等待执行的状态
c.db.Model(&models.WorkspaceTask{}).Where("workspace_id = ?", workspaceID).
    Scopes(applySearchAndTimeFilters(search, startDate, endDate)).
    Where("status IN ?", []string{"on_hold", "pending", "apply_pending"}).Count(&count)
filterCounts["on_hold"] = count
```

### 3. 增强日志记录

**文件**: `backend/services/task_queue_manager.go`

添加了详细的日志记录以便调试：

```go
// RecoverPendingTasks
log.Printf("[TaskQueue] Recovering pending tasks for %d workspaces", len(workspaceIDs))
log.Printf("[TaskQueue] Attempting to recover tasks for workspace %d", wsID)

// TryExecuteNextTask
log.Printf("[TaskQueue] TryExecuteNextTask called for workspace %d", workspaceID)
log.Printf("[TaskQueue] Starting task %d (type: %s, status: %s) for workspace %d", ...)

// GetNextExecutableTask
log.Printf("[TaskQueue] GetNextExecutableTask for workspace %d", workspaceID)
log.Printf("[TaskQueue] Found apply_pending task %d for workspace %d", ...)
log.Printf("[TaskQueue] Found plan_and_apply pending task %d for workspace %d", ...)
log.Printf("[TaskQueue] Can execute plan_and_apply task %d", ...)
log.Printf("[TaskQueue] Cannot execute plan_and_apply task %d: %s", ...)

// executeTask
log.Printf("[TaskQueue] Executing plan for task %d (workspace %d)", ...)
log.Printf("[TaskQueue] Task %d plan completed with status: %s", ...)
log.Printf("[TaskQueue] Task %d reached final status %s, triggering next task", ...)
```

## 状态分类说明

### Needs Attention (需要用户操作)
- `requires_approval`: 需要用户批准
- `plan_completed`: Plan完成，等待用户确认Apply

### On Hold (等待执行)
- `on_hold`: 手动暂停
- `pending`: 等待执行
- `apply_pending`: 等待执行Apply阶段

### Running (运行中)
- `running`: 正在执行

### Success (成功)
- `success`: Plan任务成功
- `applied`: Apply任务成功

### Errored (失败)
- `failed`: 任务失败

### Cancelled (已取消)
- `cancelled`: 用户取消

## 测试验证

### 测试场景1: 系统重启后恢复pending任务

1. 创建一个pending任务
2. 重启后端服务
3. 验证：
   - 日志显示 `[TaskQueue] Recovering pending tasks for X workspaces`
   - 日志显示 `[TaskQueue] Attempting to recover tasks for workspace X`
   - 任务自动开始执行

### 测试场景2: Filter分类正确性

1. 创建不同状态的任务
2. 在前端查看Runs列表
3. 验证：
   - Pending任务显示在"On Hold"过滤器中
   - Apply_pending任务显示在"On Hold"过滤器中
   - Plan_completed任务显示在"Needs Attention"过滤器中
   - Filter counts正确

### 测试场景3: 任务队列执行

1. 创建多个pending任务
2. 验证：
   - 任务按创建顺序执行
   - Plan任务可以并发
   - Plan_and_apply任务串行执行
   - 日志清晰显示执行流程

## 相关文件

- `backend/main.go` - 系统启动初始化
- `backend/services/task_queue_manager.go` - 任务队列管理器
- `backend/controllers/workspace_task_controller.go` - 任务控制器和filter逻辑

## 后续改进建议

1. **监控和告警**
   - 添加pending任务超时监控
   - 添加任务执行失败告警

2. **性能优化**
   - 考虑使用消息队列（如Redis）替代数据库轮询
   - 添加任务优先级支持

3. **用户体验**
   - 在前端显示任务在队列中的位置
   - 显示预计等待时间

## 发现的额外问题和修复

### 问题3: GetNextExecutableTask的逻辑bug

**发现**: 重启后端后，日志显示：
```
[TaskQueue] Found plan_and_apply pending task 163 for workspace 12
[TaskQueue] Cannot execute plan_and_apply task 163: 有plan_and_apply任务正在进行中
```

**根本原因**: 
- `GetNextExecutableTask` 找到pending的plan_and_apply任务后，调用 `CanExecuteNewTask` 检查
- `CanExecuteNewTask` 查询所有非最终状态的plan_and_apply任务
- 但这个查询包含了当前pending任务本身！
- 结果：任务#163阻止了自己的执行

**修复方案**:
在 `GetNextExecutableTask` 中检查plan_and_apply任务时，排除当前任务本身：

```go
// 检查是否有其他plan_and_apply任务正在运行（排除当前任务）
var blockingTaskCount int64
m.db.Model(&models.WorkspaceTask{}).
    Where("workspace_id = ? AND task_type = ? AND id != ? AND status NOT IN (?)",
        workspaceID,
        models.TaskTypePlanAndApply,
        planAndApplyTask.ID,  // 排除当前任务
        []string{"success", "applied", "failed", "cancelled"}).
    Count(&blockingTaskCount)

if blockingTaskCount > 0 {
    log.Printf("[TaskQueue] Cannot execute plan_and_apply task %d: other plan_and_apply tasks are in progress", planAndApplyTask.ID)
    return nil, nil
}
```

这样确保只有真正的其他任务才会阻止当前任务执行。

## 总结

本次修复解决了三个关键问题：

1.  系统启动时自动恢复pending任务
2.  修正了filter分类逻辑
3.  修复了GetNextExecutableTask的自我阻塞bug
4.  增强了日志记录以便调试

这些修复确保了任务队列系统的可靠性和用户体验的一致性。
