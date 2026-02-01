# Workspace生命周期与状态机

> **文档版本**: v2.0  
> **最后更新**: 2025-10-09  
> **前置阅读**: [00-overview.md](./00-overview.md)

## 📋 概述

Workspace的生命周期定义了从创建到完成的完整状态转换过程。本文档详细描述状态机设计、状态转换规则和异常处理机制。

## 🔄 生命周期状态机

### 状态图

```
                    ┌─────────┐
                    │ Created │ (初始状态)
                    └────┬────┘
                         │
                         ↓
                    ┌─────────┐
              ┌────→│Planning │ (执行Plan)
              │     └────┬────┘
              │          │
              │          ↓
              │     ┌─────────┐
              │     │PlanDone │ (Plan完成)
              │     └────┬────┘
              │          │
              │          ↓
              │   ┌──────────────┐
              │   │WaitingApply  │ (等待Apply)
              │   └──────┬───────┘
              │          │
              │          ↓
              │     ┌─────────┐
              └─────│Applying │ (执行Apply)
                    └────┬────┘
                         │
                    ┌────┴────┐
                    ↓         ↓
              ┌─────────┐ ┌────────┐
              │Completed│ │ Failed │ (终态)
              └─────────┘ └────────┘
```

### 状态定义

| 状态 | 说明 | 可转换到 | 是否终态 |
|------|------|----------|----------|
| `Created` | Workspace已创建，等待首次Plan | Planning | ❌ |
| `Planning` | 正在执行terraform plan | PlanDone, Failed | ❌ |
| `PlanDone` | Plan执行成功，等待决策 | WaitingApply, Planning | ❌ |
| `WaitingApply` | 等待Apply执行（手动模式） | Applying, Planning | ❌ |
| `Applying` | 正在执行terraform apply | Completed, Failed, Planning | ❌ |
| `Completed` | Apply执行成功 | Planning |  |
| `Failed` | 任意阶段失败 | Planning |  |

## 🎯 状态转换规则

### 1. Created → Planning

**触发条件**:
- 用户手动触发Plan任务
- 自动触发（如定时任务、Webhook）

**前置检查**:
- Workspace未被锁定
- 没有正在执行的任务

**操作**:
```go
func (s *WorkspaceService) StartPlan(workspaceID uint, userID uint) error {
    // 1. 检查workspace状态
    workspace, err := s.GetWorkspace(workspaceID)
    if err != nil {
        return err
    }
    
    // 2. 检查锁定状态
    if workspace.IsLocked {
        return errors.New("workspace is locked")
    }
    
    // 3. 检查是否有运行中的任务
    hasRunning, err := s.HasRunningTask(workspaceID)
    if err != nil {
        return err
    }
    if hasRunning {
        return errors.New("workspace has running task")
    }
    
    // 4. 创建Plan任务
    task := &models.WorkspaceTask{
        WorkspaceID:   workspaceID,
        TaskType:      models.TaskTypePlan,
        Status:        models.TaskStatusPending,
        ExecutionMode: workspace.ExecutionMode,
        AgentID:       workspace.AgentID,
        CreatedBy:     &userID,
    }
    
    if err := s.db.Create(task).Error; err != nil {
        return err
    }
    
    // 5. 更新workspace状态
    workspace.State = models.WorkspaceStatePlanning
    if err := s.db.Save(workspace).Error; err != nil {
        return err
    }
    
    // 6. 异步执行任务
    go s.ExecuteTask(task)
    
    return nil
}
```

### 2. Planning → PlanDone

**触发条件**:
- terraform plan执行成功

**操作**:
```go
func (s *WorkspaceService) HandlePlanSuccess(task *models.WorkspaceTask) error {
    // 1. 更新任务状态
    task.Status = models.TaskStatusSuccess
    task.CompletedAt = timePtr(time.Now())
    if err := s.db.Save(task).Error; err != nil {
        return err
    }
    
    // 2. 更新workspace状态
    workspace := task.Workspace
    workspace.State = models.WorkspaceStatePlanDone
    if err := s.db.Save(workspace).Error; err != nil {
        return err
    }
    
    // 3. 发送通知
    s.notifySystem.Notify(models.EventPlanDone, workspace, task)
    
    // 4. 检查是否自动Apply
    if workspace.AutoApply {
        return s.StartApply(workspace.ID, *task.CreatedBy)
    }
    
    return nil
}
```

### 3. PlanDone → WaitingApply

**触发条件**:
- Plan成功且配置为手动Apply模式

**操作**:
```go
func (s *WorkspaceService) WaitForApply(workspaceID uint) error {
    workspace, err := s.GetWorkspace(workspaceID)
    if err != nil {
        return err
    }
    
    // 更新状态为等待Apply
    workspace.State = models.WorkspaceStateWaitingApply
    if err := s.db.Save(workspace).Error; err != nil {
        return err
    }
    
    // 发送通知
    s.notifySystem.Notify(models.EventWaitingApply, workspace, nil)
    
    return nil
}
```

### 4. WaitingApply → Applying

**触发条件**:
- 用户手动触发Apply

**前置检查**:
- Workspace状态为WaitingApply
- Workspace未被锁定
- 存在成功的Plan任务

**操作**:
```go
func (s *WorkspaceService) StartApply(workspaceID uint, userID uint) error {
    // 1. 检查workspace状态
    workspace, err := s.GetWorkspace(workspaceID)
    if err != nil {
        return err
    }
    
    if workspace.State != models.WorkspaceStateWaitingApply && 
       workspace.State != models.WorkspaceStatePlanDone {
        return errors.New("invalid workspace state for apply")
    }
    
    // 2. 检查锁定状态
    if workspace.IsLocked {
        return errors.New("workspace is locked")
    }
    
    // 3. 获取最近的成功Plan任务
    var lastPlanTask models.WorkspaceTask
    err = s.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
        workspaceID, models.TaskTypePlan, models.TaskStatusSuccess).
        Order("created_at DESC").
        First(&lastPlanTask).Error
    if err != nil {
        return errors.New("no successful plan task found")
    }
    
    // 4. 创建Apply任务
    task := &models.WorkspaceTask{
        WorkspaceID:   workspaceID,
        TaskType:      models.TaskTypeApply,
        Status:        models.TaskStatusPending,
        ExecutionMode: workspace.ExecutionMode,
        AgentID:       workspace.AgentID,
        CreatedBy:     &userID,
    }
    
    if err := s.db.Create(task).Error; err != nil {
        return err
    }
    
    // 5. 更新workspace状态
    workspace.State = models.WorkspaceStateApplying
    if err := s.db.Save(workspace).Error; err != nil {
        return err
    }
    
    // 6. 异步执行任务
    go s.ExecuteTask(task)
    
    return nil
}
```

### 5. Applying → Completed

**触发条件**:
- terraform apply执行成功

**操作**:
```go
func (s *WorkspaceService) HandleApplySuccess(task *models.WorkspaceTask) error {
    // 1. 更新任务状态
    task.Status = models.TaskStatusSuccess
    task.CompletedAt = timePtr(time.Now())
    if err := s.db.Save(task).Error; err != nil {
        return err
    }
    
    // 2. 保存State文件（带重试）
    if err := s.SaveStateWithRetry(task.WorkspaceID, task.StateContent, task.ID); err != nil {
        log.Printf("Failed to save state: %v", err)
        // 不影响任务成功状态，但需要记录错误
    }
    
    // 3. 更新workspace状态
    workspace := task.Workspace
    workspace.State = models.WorkspaceStateCompleted
    if err := s.db.Save(workspace).Error; err != nil {
        return err
    }
    
    // 4. 发送通知
    s.notifySystem.Notify(models.EventCompleted, workspace, task)
    
    return nil
}
```

### 6. 任意状态 → Failed

**触发条件**:
- terraform命令执行失败
- 系统错误
- 超时

**操作**:
```go
func (s *WorkspaceService) HandleTaskFailure(task *models.WorkspaceTask, err error) error {
    // 1. 更新任务状态
    task.Status = models.TaskStatusFailed
    task.ErrorMessage = err.Error()
    task.CompletedAt = timePtr(time.Now())
    if err := s.db.Save(task).Error; err != nil {
        return err
    }
    
    // 2. 更新workspace状态
    workspace := task.Workspace
    workspace.State = models.WorkspaceStateFailed
    if err := s.db.Save(workspace).Error; err != nil {
        return err
    }
    
    // 3. 发送通知
    s.notifySystem.Notify(models.EventFailed, workspace, task)
    
    // 4. 检查是否需要重试
    if task.RetryCount < task.MaxRetries {
        return s.RetryTask(task)
    }
    
    return nil
}
```

## 🔒 并发控制

### 同一Workspace的并发规则

1. **Plan任务**: 可以并行执行多个Plan（不同配置）
2. **Apply任务**: 必须串行执行，同时只能有一个Apply
3. **锁定状态**: 锁定时所有任务进入pending队列

### 实现示例

```go
// 获取workspace锁
func (s *WorkspaceService) AcquireWorkspaceLock(workspaceID uint, taskType models.TaskType) error {
    // Apply任务需要独占锁
    if taskType == models.TaskTypeApply {
        // 使用数据库行锁
        var workspace models.Workspace
        err := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).
            Where("id = ?", workspaceID).
            First(&workspace).Error
        if err != nil {
            return err
        }
        
        // 检查是否有运行中的Apply任务
        var count int64
        s.db.Model(&models.WorkspaceTask{}).
            Where("workspace_id = ? AND task_type = ? AND status = ?",
                workspaceID, models.TaskTypeApply, models.TaskStatusRunning).
            Count(&count)
        
        if count > 0 {
            return errors.New("another apply task is running")
        }
    }
    
    return nil
}
```

## 🔄 状态回滚

### 支持的回滚场景

1. **Plan失败**: 自动回滚到Created或Completed状态
2. **Apply失败**: 保持Failed状态，需要手动干预
3. **State回滚**: 可以回滚到历史State版本

### 回滚实现

```go
func (s *WorkspaceService) RollbackToState(workspaceID uint, version int) error {
    // 1. 获取指定版本的State
    stateVersion, err := s.GetStateVersion(workspaceID, version)
    if err != nil {
        return err
    }
    
    // 2. 创建回滚任务
    task := &models.WorkspaceTask{
        WorkspaceID: workspaceID,
        TaskType:    models.TaskTypeRollback,
        Status:      models.TaskStatusPending,
    }
    
    // 3. 执行回滚
    // ... 实现回滚逻辑
    
    return nil
}
```

## 📊 状态统计

### 监控指标

```go
// Prometheus指标定义
var (
    workspaceStateGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "workspace_state",
            Help: "Current state of workspaces",
        },
        []string{"workspace_id", "state"},
    )
    
    taskDurationHistogram = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "workspace_task_duration_seconds",
            Help:    "Duration of workspace tasks",
            Buckets: prometheus.DefBuckets,
        },
        []string{"workspace_id", "task_type", "status"},
    )
)
```

## 🧪 测试用例

### 状态转换测试

```go
func TestWorkspaceLifecycle(t *testing.T) {
    // 1. 创建workspace
    workspace := createTestWorkspace(t)
    assert.Equal(t, models.WorkspaceStateCreated, workspace.State)
    
    // 2. 启动Plan
    err := service.StartPlan(workspace.ID, testUserID)
    assert.NoError(t, err)
    
    workspace = getWorkspace(t, workspace.ID)
    assert.Equal(t, models.WorkspaceStatePlanning, workspace.State)
    
    // 3. Plan成功
    task := getLastTask(t, workspace.ID)
    err = service.HandlePlanSuccess(task)
    assert.NoError(t, err)
    
    workspace = getWorkspace(t, workspace.ID)
    assert.Equal(t, models.WorkspaceStatePlanDone, workspace.State)
    
    // 4. 启动Apply
    err = service.StartApply(workspace.ID, testUserID)
    assert.NoError(t, err)
    
    workspace = getWorkspace(t, workspace.ID)
    assert.Equal(t, models.WorkspaceStateApplying, workspace.State)
    
    // 5. Apply成功
    task = getLastTask(t, workspace.ID)
    err = service.HandleApplySuccess(task)
    assert.NoError(t, err)
    
    workspace = getWorkspace(t, workspace.ID)
    assert.Equal(t, models.WorkspaceStateCompleted, workspace.State)
}
```

## 📝 最佳实践

### 1. 状态检查
- 每次操作前检查当前状态
- 使用状态机模式确保状态转换合法

### 2. 错误处理
- 详细记录错误信息
- 提供重试机制
- 发送失败通知

### 3. 并发控制
- Apply任务使用数据库锁
- Plan任务可以并行
- 锁定状态优先级最高

### 4. 监控告警
- 记录状态转换时间
- 监控失败率
- 设置超时告警

## 🔗 相关文档

- **上一篇**: [00-overview.md](./00-overview.md) - 总览与架构
- **下一篇**: [02-execution-modes.md](./02-execution-modes.md) - 执行模式详解
- **相关**: [04-task-workflow.md](./04-task-workflow.md) - 任务流程

---

**下一步**: 阅读 [02-execution-modes.md](./02-execution-modes.md) 了解执行模式设计
