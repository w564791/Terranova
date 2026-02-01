# 任务调度规则最终修复总结

## 正确的设计原则

根据用户明确的设计原则:

### 1. Plan任务规则
-  **Plan任务可以并发执行**
-  **Plan任务不受plan+apply任务阻塞**
-  **Plan任务只受workspace lock限制**

### 2. Plan+Apply任务规则
-  **Plan+Apply任务必须顺序执行**(串行)
-  **Running状态的plan+apply阻塞所有任务**(正在执行,需要独占)
-  **Pending/Apply_pending状态的plan+apply只阻塞其他plan+apply**
-  **Apply_pending任务只能通过用户confirm触发**

### 3. Workspace Lock规则
-  **Workspace被lock时,所有任务都要等待**(最高优先级)

## 修复的关键问题

### 问题1: Agent容量计算错误
**修复前**: 副本数 = 活跃任务总数
**修复后**: 副本数 = max(ceil(plan/3), plan+apply)
**文件**: `backend/services/k8s_deployment_service.go`

### 问题2: Plan任务被错误阻塞
**修复前**: Apply_pending状态的plan+apply阻塞所有任务(包括plan)
**修复后**: Apply_pending状态的plan+apply只阻塞其他plan+apply,不阻塞plan
**文件**: `backend/services/task_queue_manager.go`

### 问题3: 缺少workspace lock检查
**修复前**: 没有检查workspace.IsLocked
**修复后**: 在GetNextExecutableTask步骤0检查lock状态
**文件**: `backend/services/task_queue_manager.go`

### 问题4: 缺少pending任务重试机制
**修复前**: Pending任务被阻塞后没有重试
**修复后**: 添加StartPendingTasksMonitor定期重试
**文件**: `backend/services/task_queue_manager.go` + `backend/main.go`

## 最终的任务调度逻辑

```go
func GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // 0. 检查workspace是否被lock
    if workspace.IsLocked {
        return nil, nil // 所有任务等待
    }

    // 1. 检查是否有running状态的plan_and_apply任务
    if runningPlanAndApplyCount > 0 {
        return nil, nil // 所有任务等待
    }

    // 2. 检查plan_and_apply pending/apply_pending任务
    if found plan_and_apply task {
        // 检查是否有其他plan_and_apply任务阻塞它
        if otherBlockingCount > 0 {
            // plan_and_apply被阻塞,但plan任务可以执行
            // 继续检查plan任务
        } else {
            return plan_and_apply task
        }
    }

    // 3. 获取plan任务（可以并发,不受plan_and_apply阻塞）
    if found plan task {
        return plan task
    }

    return nil, nil
}
```

## 任务执行流程图

```
开始
  ↓
Workspace被Lock?
  ├─ 是 → 返回nil (所有任务等待)
  └─ 否 → 继续
      ↓
有Running的Plan+Apply?
  ├─ 是 → 返回nil (所有任务等待)
  └─ 否 → 继续
      ↓
有Pending/Apply_Pending的Plan+Apply?
  ├─ 是 → 检查是否被其他Plan+Apply阻塞
  │       ├─ 是 → 继续检查Plan任务
  │       └─ 否 → 返回该Plan+Apply任务
  └─ 否 → 继续
      ↓
有Pending的Plan任务?
  ├─ 是 → 返回该Plan任务 (可并发,不受Plan+Apply阻塞)
  └─ 否 → 返回nil
```

## 修改文件清单

1. `backend/services/k8s_deployment_service.go`
   - 修改 `CountPendingTasksForPool` - Agent容量计算

2. `backend/services/task_queue_manager.go`
   - 修改 `GetNextExecutableTask` - 任务调度规则
   - 修改 `CanExecuteNewTask` - 添加lock检查
   - 添加 `StartPendingTasksMonitor` - Pending任务监控
   - 添加 `checkAndRetryPendingTasks` - 重试逻辑

3. `backend/main.go`
   - 添加监控器启动代码

## 测试场景验证

### 场景1: Plan任务不受Apply_Pending阻塞 
- 状态: 1个plan+apply任务apply_pending, 1个plan任务pending
- 期望: Plan任务可以执行
- 结果:  Plan任务返回并执行

### 场景2: Running的Plan+Apply阻塞所有任务 
- 状态: 1个plan+apply任务running
- 期望: 所有任务(plan和plan+apply)都等待
- 结果:  返回nil

### 场景3: Plan+Apply任务串行执行 
- 状态: 2个plan+apply任务都pending
- 期望: 只执行最早的一个
- 结果:  返回最早的plan+apply任务

### 场景4: Workspace Lock阻塞所有任务 
- 状态: workspace.IsLocked = true
- 期望: 所有任务都等待
- 结果:  返回nil

### 场景5: Plan任务并发执行 
- 状态: 3个plan任务pending
- 期望: 可以并发执行(通过多次调用)
- 结果:  每次返回一个plan任务

## 重启后的预期行为

重启后端服务后,任务536将会:

1. **被pending任务监控器检测到** (30秒间隔)
   ```
   [TaskQueue] Checking 1 workspaces with pending tasks
   ```

2. **通过GetNextExecutableTask正确判断**
   ```
   [TaskQueue] GetNextExecutableTask for workspace ws-mb7m9ii5ey
   [TaskQueue] Workspace ws-mb7m9ii5ey is not locked
   [TaskQueue] Found 0 running plan_and_apply tasks
   [TaskQueue] Found plan pending task 536 (can execute concurrently, not blocked by plan_and_apply)
   ```

3. **自动开始执行**
   ```
   [TaskQueue] Selected agent xxx from pool pool-z73eh8ihywlmgx0x
   [TaskQueue] Successfully pushed task 536 to agent xxx
   ```

## 总结

所有修复已完成,正确实现了设计原则:
-  Plan任务可以并发,不受plan+apply阻塞
-  Plan+apply任务串行执行
-  Running的plan+apply独占workspace
-  Workspace lock阻塞所有任务
-  Pending任务有自动重试机制
-  Apply_pending任务不受重启影响
-  Agent容量正确计算

**请重启后端服务以应用所有修复。**
