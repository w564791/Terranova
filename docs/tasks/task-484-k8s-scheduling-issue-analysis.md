# Task 484 K8s任务调度问题分析

## 问题现象

任务484在执行confirm apply后：
- 任务状态变为 `running`，stage变为 `applying`
- 但是 `agent_id` 和 `k8s_pod_name` 字段为空
- 任务在后端服务本地执行，而不是在Agent中执行
- Agent日志中没有任务调度记录
- 后端日志中**完全没有**TaskQueue相关的日志

## 数据库状态

```sql
id  | workspace_id  |   task_type    | status  |  stage   | execution_mode | agent_id | k8s_pod_name
484 | ws-mb7m9ii5ey | plan_and_apply | running | applying | k8s            |          |
```

## 根本原因分析

### 1. ConfirmApply方法的异步调用

在 `backend/controllers/workspace_task_controller.go` 的 `ConfirmApply` 方法中（第1088-1093行）：

```go
// 通知队列管理器尝试执行Apply
go func() {
    if err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID); err != nil {
        log.Printf("Failed to start apply execution: %v", err)
    }
}()
```

**问题**：
1. 这个goroutine是异步执行的，如果发生panic会被静默吞掉
2. 没有panic recovery机制
3. 错误日志只在goroutine内部打印，可能因为panic而没有执行到

### 2. 缺少日志的原因

后端日志中完全没有以下关键字：
- `TryExecuteNextTask`
- `pushTaskToAgent`
- `Agent C&C handler`

这说明：
1. goroutine可能在调用 `TryExecuteNextTask` 之前就panic了
2. 或者 `TryExecuteNextTask` 方法本身在早期就panic了
3. 没有任何错误被记录到日志

### 3. 可能的Panic原因

查看 `TaskQueueManager.TryExecuteNextTask` 方法（task_queue_manager.go 第107-157行）：

```go
func (m *TaskQueueManager) TryExecuteNextTask(workspaceID string) error {
    // 1. 获取workspace锁
    lockKey := fmt.Sprintf("ws_%s", workspaceID)
    lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
    mutex := lock.(*sync.Mutex)  // ← 可能的panic点

    mutex.Lock()
    defer mutex.Unlock()
    
    // ...
}
```

**可能的panic原因**：
1. `m.workspaceLocks` 可能是nil（虽然在NewTaskQueueManager中初始化了）
2. 类型断言 `lock.(*sync.Mutex)` 可能失败
3. `m.agentCCHandler` 可能是nil（虽然在main.go中设置了）

### 4. 任务为什么会变成running状态

即使 `TryExecuteNextTask` 失败了，任务状态已经在 `ConfirmApply` 方法中被设置为 `apply_pending`：

```go
// 更新任务
task.ApplyDescription = req.ApplyDescription
task.Status = models.TaskStatusApplyPending  // ← 这里设置
task.Stage = "apply_pending"
task.PlanTaskID = &task.ID

if err := c.db.Save(&task).Error; err != nil {
    ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
    return
}
```

然后某个地方（可能是本地执行的fallback逻辑）把状态改成了 `running`。

## 解决方案

### 方案1：添加Panic Recovery

在 `ConfirmApply` 方法的goroutine中添加panic recovery：

```go
// 通知队列管理器尝试执行Apply
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Printf("[PANIC] TryExecuteNextTask panicked: %v", r)
            // 可选：更新任务状态为failed
        }
    }()
    
    if err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID); err != nil {
        log.Printf("Failed to start apply execution: %v", err)
    }
}()
```

### 方案2：同步调用（推荐）

将异步调用改为同步调用，这样错误可以直接返回给用户：

```go
// 通知队列管理器尝试执行Apply
if err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID); err != nil {
    log.Printf("[ERROR] Failed to start apply execution: %v", err)
    ctx.JSON(http.StatusInternalServerError, gin.H{
        "error": fmt.Sprintf("Failed to start apply execution: %v", err),
    })
    return
}
```

### 方案3：增强日志

在 `TryExecuteNextTask` 方法开始处添加日志：

```go
func (m *TaskQueueManager) TryExecuteNextTask(workspaceID string) error {
    log.Printf("[TaskQueue] ===== TryExecuteNextTask START for workspace %s =====", workspaceID)
    defer log.Printf("[TaskQueue] ===== TryExecuteNextTask END for workspace %s =====", workspaceID)
    
    // 添加panic recovery
    defer func() {
        if r := recover(); r != nil {
            log.Printf("[TaskQueue] PANIC in TryExecuteNextTask: %v", r)
        }
    }()
    
    // 原有代码...
}
```

## 下一步行动

1. 添加panic recovery到所有关键的goroutine
2. 将 `ConfirmApply` 中的异步调用改为同步调用
3. 增强 `TryExecuteNextTask` 的日志输出
4. 测试修复后的行为
