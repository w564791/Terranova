# Plan任务与Plan+Apply任务非阻塞优化方案 V2

## 问题根源分析

### 真正的问题所在

通过深入分析代码，发现问题在 `TryExecuteNextTask` 方法的 **workspace级别互斥锁**：

```go
// backend/services/task_queue_manager.go 第193-197行
// 1. 获取workspace锁
lockKey := fmt.Sprintf("ws_%s", workspaceID)
lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
mutex := lock.(*sync.Mutex)

mutex.Lock()
defer mutex.Unlock()
```

**问题流程：**
1. Plan任务开始调度 → 获取workspace锁
2. Plan任务调度完成（推送到Agent）→ 释放workspace锁
3. 在Plan任务调度期间，任何其他任务（包括Plan+Apply）都被阻塞
4. **结果：Plan任务阻塞了Plan+Apply任务的调度**

### 为什么需要这个锁？

这个锁的原始目的是：
- 防止并发调度冲突
- 确保 `GetNextExecutableTask` 和任务状态更新的原子性
- 避免同一个任务被重复调度

### 问题的本质

**当前设计：** 所有任务类型共享同一个workspace锁
**导致问题：** Plan任务（不需要串行）也会持有锁，阻塞Plan+Apply任务

## 优化方案

### 核心思想

**Plan任务不持有锁，只有Plan+Apply任务才持有锁**

- Plan任务：完全独立，可以并发执行，不需要锁
- Plan+Apply任务：需要串行执行，必须持有锁

### 实现方式

修改 `TryExecuteNextTask` 方法，根据任务类型决定是否加锁：

```go
func (m *TaskQueueManager) TryExecuteNextTask(workspaceID string) error {
    log.Printf("[TaskQueue] ===== TryExecuteNextTask START for workspace %s =====", workspaceID)
    defer log.Printf("[TaskQueue] ===== TryExecuteNextTask END for workspace %s =====", workspaceID)

    // 添加panic recovery
    defer func() {
        if r := recover(); r != nil {
            log.Printf("[TaskQueue] ❌ PANIC in TryExecuteNextTask for workspace %s: %v", workspaceID, r)
            log.Printf("[TaskQueue] Stack trace: %s", debug.Stack())
        }
    }()

    log.Printf("[TaskQueue] TryExecuteNextTask called for workspace %s", workspaceID)

    // 1. 先获取下一个任务（不加锁）
    task, err := m.GetNextExecutableTask(workspaceID)
    if err != nil {
        log.Printf("[TaskQueue] Error getting next task for workspace %s: %v", workspaceID, err)
        return err
    }

    if task == nil {
        log.Printf("[TaskQueue] No executable tasks for workspace %s", workspaceID)
        return nil
    }

    // 2. 根据任务类型决定是否加锁
    // Plan任务：不加锁，可以并发执行
    // Plan+Apply任务：加锁，必须串行执行
    if task.TaskType == models.TaskTypePlanAndApply {
        log.Printf("[TaskQueue] Plan+Apply task %d requires workspace lock", task.ID)
        
        // 获取workspace锁
        lockKey := fmt.Sprintf("ws_%s", workspaceID)
        lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
        mutex := lock.(*sync.Mutex)

        mutex.Lock()
        defer mutex.Unlock()
        
        log.Printf("[TaskQueue] Acquired workspace lock for plan+apply task %d", task.ID)
        
        // 重新检查任务状态（可能在等待锁期间被其他goroutine处理了）
        var currentTask models.WorkspaceTask
        if err := m.db.First(&currentTask, task.ID).Error; err != nil {
            log.Printf("[TaskQueue] Task %d not found after acquiring lock: %v", task.ID, err)
            return nil
        }
        
        if currentTask.Status != models.TaskStatusPending && currentTask.Status != models.TaskStatusApplyPending {
            log.Printf("[TaskQueue] Task %d status changed to %s after acquiring lock, skipping", task.ID, currentTask.Status)
            return nil
        }
        
        // 更新task为最新状态
        task = &currentTask
    } else {
        log.Printf("[TaskQueue] Plan task %d does not require workspace lock (can execute concurrently)", task.ID)
    }

    // 3. 获取workspace信息以确定执行模式
    var workspace models.Workspace
    if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
        log.Printf("[TaskQueue] Error getting workspace %s: %v", workspaceID, err)
        return err
    }

    // 4. 检查是否为K8s执行模式
    if workspace.ExecutionMode == models.ExecutionModeK8s {
        log.Printf("[TaskQueue] Workspace %s is in K8s mode, pushing task to K8s deployment agent", workspaceID)
        return m.pushTaskToAgent(task, &workspace)
    }

    // 5. 检查是否为Agent执行模式
    if workspace.ExecutionMode == models.ExecutionModeAgent {
        log.Printf("[TaskQueue] Workspace %s is in Agent mode, pushing task to agent", workspaceID)
        return m.pushTaskToAgent(task, &workspace)
    }

    // 6. 本地模式 - 直接执行任务
    log.Printf("[TaskQueue] Starting task %d (type: %s, status: %s) for workspace %s in Local mode",
        task.ID, task.TaskType, task.Status, workspaceID)
    go m.executeTask(task)

    return nil
}
```

### 关键改进点

1. **先获取任务，再决定是否加锁**
   - 避免所有任务都竞争锁
   - Plan任务完全不需要等待锁

2. **只有Plan+Apply任务加锁**
   - Plan任务：不加锁，可以并发调度
   - Plan+Apply任务：加锁，保证串行执行

3. **加锁后重新检查任务状态**
   - 防止在等待锁期间任务被其他goroutine处理
   - 确保任务状态的一致性

4. **详细的日志记录**
   - 记录是否加锁
   - 便于调试和问题追踪

## 优化效果

### 优化前

```
时间线：
T1: Plan任务A开始调度 → 获取workspace锁
T2: Plan+Apply任务B尝试调度 → 等待workspace锁（被阻塞）
T3: Plan任务A调度完成 → 释放workspace锁
T4: Plan+Apply任务B获取锁 → 开始调度
```

**问题：** Plan任务A阻塞了Plan+Apply任务B

### 优化后

```
时间线：
T1: Plan任务A开始调度 → 不加锁，直接执行
T2: Plan+Apply任务B开始调度 → 获取workspace锁
T3: Plan任务A调度完成（与T2并发）
T4: Plan+Apply任务B调度完成 → 释放workspace锁
```

**效果：** Plan任务A和Plan+Apply任务B可以并发调度

### 并发场景

**场景1：多个Plan任务**
```
Plan任务A、B、C可以同时调度（不加锁）
```

**场景2：Plan任务 + Plan+Apply任务**
```
Plan任务A：不加锁，直接调度
Plan+Apply任务B：加锁，串行执行
两者互不影响
```

**场景3：多个Plan+Apply任务**
```
Plan+Apply任务A：获取锁，执行
Plan+Apply任务B：等待锁，串行执行
保持原有的串行保证
```

## 风险评估

### 低风险

1. **Plan任务的并发安全性**
   - Plan任务本身就是设计为可以并发的
   - 不会修改实际资源，只是读取和计算
   - 移除锁不会影响其正确性

2. **Plan+Apply任务的串行保证**
   - 仍然使用workspace锁
   - 保持原有的串行执行逻辑
   - 不影响资源变更的顺序性

### 需要注意

1. **任务状态的一致性**
   - 加锁后重新检查任务状态
   - 防止重复调度

2. **数据库并发**
   - 依赖数据库的事务隔离级别
   - 确保任务状态更新的原子性

## 实施步骤

### 步骤1：修改 TryExecuteNextTask 方法

文件：`backend/services/task_queue_manager.go`

替换整个 `TryExecuteNextTask` 方法为上面提供的新实现。

### 步骤2：测试验证

**测试场景1：Plan任务不阻塞Plan+Apply**
```
1. 创建Plan任务A
2. 立即创建Plan+Apply任务B
3. 验证：两个任务可以同时开始调度
4. 验证：Plan任务A不会阻塞Plan+Apply任务B
```

**测试场景2：多个Plan任务并发**
```
1. 创建3个Plan任务（A、B、C）
2. 验证：3个任务可以同时调度
3. 验证：没有任务被阻塞
```

**测试场景3：Plan+Apply任务串行**
```
1. 创建Plan+Apply任务A
2. 创建Plan+Apply任务B
3. 验证：任务B必须等待任务A完成
4. 验证：串行执行保证仍然有效
```

**测试场景4：混合场景**
```
1. 创建Plan+Apply任务A（开始执行）
2. 创建Plan任务B
3. 创建Plan+Apply任务C
4. 验证：Plan任务B可以立即执行（不等待A）
5. 验证：Plan+Apply任务C必须等待A完成
```

### 步骤3：监控和回滚

1. **监控指标**
   - 任务调度延迟
   - 任务执行成功率
   - 并发任务数量

2. **回滚方案**
   - 保留原代码作为备份
   - 如果出现问题，快速回滚到原实现

## 总结

通过让Plan任务不持有workspace锁，我们实现了：

1.  **Plan任务不阻塞Plan+Apply任务** - 核心目标
2.  **Plan任务可以并发执行** - 提高执行效率
3.  **Plan+Apply任务保持串行** - 保证资源变更的顺序性
4.  **最小化代码改动** - 只修改一个方法
5.  **低风险** - 不影响现有的串行保证

这个方案完美解决了用户反馈的问题，同时保持了系统的稳定性和一致性。
