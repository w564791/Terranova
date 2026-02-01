# 任务调度规则全面修复方案

## 正确的任务调度规则

根据用户需求,任务调度应该遵循以下规则:

### 1. Workspace Lock机制
- 当workspace被lock后,**任何任务都要pending**
- Lock优先级最高,阻塞所有任务

### 2. Plan任务规则
- **Plan任务可以并发执行**(多个plan任务可以同时运行)
- Plan任务**不会**被其他plan任务阻塞
- Plan任务**只会**被以下情况阻塞:
  - Workspace被lock
  - 有running状态的plan+apply任务
  - 有apply_pending状态的plan+apply任务

### 3. Plan+Apply任务规则
- **Plan+Apply任务必须顺序执行**(串行)
- 只要有plan+apply任务没有标记为结束,其他plan+apply任务都要阻塞
- Plan+Apply任务的非最终状态包括:
  - `pending` - 等待执行
  - `running` - 正在执行plan或apply
  - `apply_pending` - plan完成,等待用户确认apply
- Plan+Apply任务的最终状态包括:
  - `success` - Plan任务成功完成(仅plan模式)
  - `applied` - Apply成功完成
  - `failed` - 失败
  - `cancelled` - 取消

### 4. 任务优先级
1. **Workspace Lock** (最高优先级)
2. **Running/Apply_Pending的Plan+Apply任务** (阻塞所有新任务)
3. **Pending的Plan+Apply任务** (优先于Plan任务)
4. **Plan任务** (可并发)

## 当前实现的问题

### 问题1: GetNextExecutableTask逻辑不完整

当前代码:
```go
// 1. 只检查running或apply_pending状态的plan_and_apply任务
var blockingPlanAndApplyCount int64
m.db.Model(&models.WorkspaceTask{}).
    Where("workspace_id = ? AND task_type = ? AND status IN (?)",
        workspaceID, 
        models.TaskTypePlanAndApply, 
        []models.TaskStatus{models.TaskStatusRunning, models.TaskStatusApplyPending}).
    Count(&blockingPlanAndApplyCount)
```

**问题**: 这个逻辑是正确的,但是缺少workspace lock检查!

### 问题2: 缺少Workspace Lock检查

当前代码完全没有检查workspace的lock状态。如果workspace被lock,所有任务都应该被阻塞。

### 问题3: CanExecuteNewTask方法未被使用

`CanExecuteNewTask` 方法存在但从未被调用,导致其检查逻辑无效。

## 修复方案

### 1. 在GetNextExecutableTask中添加Workspace Lock检查

```go
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    log.Printf("[TaskQueue] GetNextExecutableTask for workspace %s", workspaceID)

    // 0. 首先检查workspace是否被lock
    var workspace models.Workspace
    if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
        return nil, fmt.Errorf("failed to get workspace: %w", err)
    }

    if workspace.IsLocked {
        log.Printf("[TaskQueue] Workspace %s is locked by %s, all tasks must wait", 
            workspaceID, *workspace.LockedBy)
        return nil, nil
    }

    // 1. 检查是否有plan_and_apply任务处于running或apply_pending状态
    // 这些状态都会阻塞所有新任务
    var blockingPlanAndApplyCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND task_type = ? AND status IN (?)",
            workspaceID, 
            models.TaskTypePlanAndApply, 
            []models.TaskStatus{models.TaskStatusRunning, models.TaskStatusApplyPending}).
        Count(&blockingPlanAndApplyCount)

    if blockingPlanAndApplyCount > 0 {
        log.Printf("[TaskQueue] Found %d blocking plan_and_apply tasks (running or apply_pending), all tasks must wait", 
            blockingPlanAndApplyCount)
        return nil, nil
    }

    // 2. 检查plan_and_apply pending任务
    var planAndApplyTask models.WorkspaceTask
    err := m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
        workspaceID, models.TaskTypePlanAndApply, models.TaskStatusPending).
        Order("created_at ASC").
        First(&planAndApplyTask).Error

    if err == nil {
        // 找到plan_and_apply pending任务,可以执行
        log.Printf("[TaskQueue] Found plan_and_apply pending task %d for workspace %s (no blocking tasks)", 
            planAndApplyTask.ID, workspaceID)
        return &planAndApplyTask, nil
    } else if err != gorm.ErrRecordNotFound {
        log.Printf("[TaskQueue] Error checking plan_and_apply tasks: %v", err)
        return nil, err
    }

    // 3. 获取plan任务（可以并发执行）
    var planTask models.WorkspaceTask
    err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
        workspaceID, models.TaskTypePlan, models.TaskStatusPending).
        Order("created_at ASC").
        First(&planTask).Error

    if err == gorm.ErrRecordNotFound {
        log.Printf("[TaskQueue] No pending tasks found for workspace %s", workspaceID)
        return nil, nil
    }

    if err != nil {
        log.Printf("[TaskQueue] Error checking plan tasks: %v", err)
        return nil, err
    }

    log.Printf("[TaskQueue] Found plan pending task %d for workspace %s (can execute concurrently)", 
        planTask.ID, workspaceID)
    return &planTask, nil
}
```

### 2. 更新CanExecuteNewTask方法(保持向后兼容)

虽然这个方法目前未被使用,但应该保持其逻辑正确性:

```go
func (m *TaskQueueManager) CanExecuteNewTask(workspaceID string) (bool, string) {
    // 1. 检查workspace是否被lock
    var workspace models.Workspace
    if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
        return false, fmt.Sprintf("无法获取workspace信息: %v", err)
    }

    if workspace.IsLocked {
        lockedBy := "unknown"
        if workspace.LockedBy != nil {
            lockedBy = *workspace.LockedBy
        }
        return false, fmt.Sprintf("Workspace被%s锁定", lockedBy)
    }

    // 2. 检查是否有plan_and_apply任务处于非最终状态
    var blockingTaskCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND task_type = ? AND status NOT IN (?)",
            workspaceID,
            models.TaskTypePlanAndApply,
            []string{"success", "applied", "failed", "cancelled"}).
        Count(&blockingTaskCount)

    if blockingTaskCount > 0 {
        return false, "有plan_and_apply任务正在进行中"
    }

    return true, ""
}
```

## 任务调度流程图

```
开始
  ↓
检查Workspace是否被Lock?
  ├─ 是 → 返回nil (所有任务等待)
  └─ 否 → 继续
      ↓
检查是否有Running/Apply_Pending的Plan+Apply任务?
  ├─ 是 → 返回nil (所有任务等待)
  └─ 否 → 继续
      ↓
检查是否有Pending的Plan+Apply任务?
  ├─ 是 → 返回该Plan+Apply任务
  └─ 否 → 继续
      ↓
检查是否有Pending的Plan任务?
  ├─ 是 → 返回该Plan任务
  └─ 否 → 返回nil (无待执行任务)
```

## 测试场景

### 场景1: Workspace被Lock
- **状态**: Workspace.IsLocked = true
- **期望**: 所有任务(plan和plan+apply)都被阻塞
- **验证**: GetNextExecutableTask返回nil

### 场景2: 有Running的Plan+Apply任务
- **状态**: 1个plan+apply任务status=running
- **期望**: 所有新任务(plan和plan+apply)都被阻塞
- **验证**: GetNextExecutableTask返回nil

### 场景3: 有Apply_Pending的Plan+Apply任务
- **状态**: 1个plan+apply任务status=apply_pending
- **期望**: 所有新任务(plan和plan+apply)都被阻塞
- **验证**: GetNextExecutableTask返回nil

### 场景4: 有Pending的Plan+Apply任务,无阻塞任务
- **状态**: 1个plan+apply任务status=pending, 无running/apply_pending任务
- **期望**: 返回该plan+apply任务
- **验证**: GetNextExecutableTask返回该plan+apply任务

### 场景5: 只有Pending的Plan任务,无Plan+Apply任务
- **状态**: 多个plan任务status=pending, 无plan+apply任务
- **期望**: 返回最早的plan任务(可并发)
- **验证**: GetNextExecutableTask返回最早的plan任务

### 场景6: 有Pending的Plan+Apply和Plan任务
- **状态**: 1个plan+apply任务pending, 1个plan任务pending
- **期望**: 优先返回plan+apply任务
- **验证**: GetNextExecutableTask返回plan+apply任务

## 修改文件清单

- `backend/services/task_queue_manager.go`
  - 修改 `GetNextExecutableTask` 方法,添加workspace lock检查
  - 更新 `CanExecuteNewTask` 方法,添加workspace lock检查

## 影响范围

- 任务调度逻辑
- Workspace lock功能
- Plan和Plan+Apply任务的并发控制

## 优先级

**高优先级** - 这是核心的任务调度逻辑,必须确保正确性。
