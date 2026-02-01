# Plan任务与Plan+Apply任务非阻塞优化方案

## 问题分析

### 当前实现

根据 `backend/services/task_queue_manager.go` 中的 `GetNextExecutableTask` 方法分析:

**当前任务执行规则:**
```go
// 0. workspace被lock时,所有任务都要等待(最高优先级)
// 1. plan任务完全独立,可以并发执行,不受任何plan_and_apply任务阻塞
// 2. plan_and_apply任务之间必须串行执行
//    - running状态的plan_and_apply阻塞其他plan_and_apply任务
//    - pending/apply_pending状态的plan_and_apply阻塞其他plan_and_apply任务
```

### 问题描述

**用户反馈的问题:**
- Plan任务会阻塞Plan+Apply任务
- Plan+Apply任务不会阻塞Plan任务

**实际代码逻辑:**
查看 `GetNextExecutableTask` 方法的实现:

```go
// 1. 检查plan_and_apply pending/apply_pending任务
var planAndApplyTask models.WorkspaceTask
err := m.db.Where("workspace_id = ? AND task_type = ? AND status IN (?)",
    workspaceID, models.TaskTypePlanAndApply, 
    []models.TaskStatus{models.TaskStatusPending, models.TaskStatusApplyPending}).
    Order("created_at ASC").
    First(&planAndApplyTask).Error

if err == nil {
    // 找到plan_and_apply任务,检查是否有running/pending/apply_pending的plan_and_apply任务阻塞它
    var otherBlockingCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND task_type = ? AND id < ? AND status IN (?)",
            workspaceID,
            models.TaskTypePlanAndApply,
            planAndApplyTask.ID,
            []models.TaskStatus{models.TaskStatusPending, models.TaskStatusRunning, models.TaskStatusApplyPending}).
        Count(&otherBlockingCount)

    if otherBlockingCount > 0 {
        log.Printf("[TaskQueue] Plan_and_apply task %d is blocked by %d earlier plan_and_apply tasks", 
            planAndApplyTask.ID, otherBlockingCount)
        // plan_and_apply被阻塞,但plan任务可以执行
        // 继续检查plan任务
    } else {
        log.Printf("[TaskQueue] Found plan_and_apply task %d for workspace %s (no blocking tasks)", 
            planAndApplyTask.ID, workspaceID)
        return &planAndApplyTask, nil
    }
}

// 2. 获取plan任务（完全独立,可以并发执行,不受任何plan_and_apply任务阻塞）
var planTask models.WorkspaceTask
err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
    workspaceID, models.TaskTypePlan, models.TaskStatusPending).
    Order("created_at ASC").
    First(&planTask).Error
```

**代码逻辑分析:**
1. **优先级顺序**: 代码先检查 plan_and_apply 任务，再检查 plan 任务
2. **Plan任务不阻塞**: Plan任务完全独立，不会阻塞任何任务
3. **Plan+Apply任务优先**: 如果有可执行的 plan_and_apply 任务，会优先返回它
4. **Plan+Apply被阻塞时**: 如果 plan_and_apply 被其他 plan_and_apply 阻塞，会继续检查 plan 任务

### 问题根源

**用户感知到的"阻塞"可能来自:**

1. **优先级导致的延迟**
   - Plan+Apply任务优先级更高
   - 当有可执行的Plan+Apply任务时，Plan任务会被"跳过"
   - 这不是真正的阻塞，而是优先级调度

2. **Running状态的Plan+Apply任务**
   - 代码中检查阻塞条件时，包含了 `TaskStatusRunning`
   - 如果有一个Plan+Apply任务正在running，会阻塞后续的Plan+Apply任务
   - 但不会阻塞Plan任务

3. **资源竞争**
   - 在Agent/K8s模式下，如果Agent资源不足
   - Plan+Apply任务可能占用了所有可用的Agent
   - 导致Plan任务无法获得执行资源

## 优化方案

### 方案1: 调整任务优先级策略（推荐）

**核心思想**: 改变任务选择的优先级，让Plan任务和Plan+Apply任务有平等的机会被执行

**实现方式:**

```go
// GetNextExecutableTask 获取下一个可执行的任务
// 优化后的任务执行规则:
// 0. workspace被lock时,所有任务都要等待(最高优先级)
// 1. plan任务和plan_and_apply任务按创建时间排序,先创建的先执行
// 2. plan任务完全独立,可以并发执行,不受任何plan_and_apply任务阻塞
// 3. plan_and_apply任务之间必须串行执行
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // 0. 检查workspace是否被lock
    var workspace models.Workspace
    if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
        return nil, fmt.Errorf("failed to get workspace: %w", err)
    }

    if workspace.IsLocked {
        log.Printf("[TaskQueue] Workspace %s is locked, all tasks must wait", workspaceID)
        return nil, nil
    }

    // 1. 获取最早的pending任务（不区分类型）
    var earliestTask models.WorkspaceTask
    err := m.db.Where("workspace_id = ? AND status = ?",
        workspaceID, models.TaskStatusPending).
        Order("created_at ASC").
        First(&earliestTask).Error

    if err == gorm.ErrRecordNotFound {
        // 没有pending任务，检查apply_pending任务
        err = m.db.Where("workspace_id = ? AND status = ?",
            workspaceID, models.TaskStatusApplyPending).
            Order("created_at ASC").
            First(&earliestTask).Error
        
        if err == gorm.ErrRecordNotFound {
            log.Printf("[TaskQueue] No pending tasks found for workspace %s", workspaceID)
            return nil, nil
        }
        if err != nil {
            return nil, err
        }
    } else if err != nil {
        return nil, err
    }

    // 2. 如果是plan任务，直接返回（plan任务不受阻塞）
    if earliestTask.TaskType == models.TaskTypePlan {
        log.Printf("[TaskQueue] Found plan task %d (created at %v), can execute immediately",
            earliestTask.ID, earliestTask.CreatedAt)
        return &earliestTask, nil
    }

    // 3. 如果是plan_and_apply任务，检查是否被其他plan_and_apply任务阻塞
    if earliestTask.TaskType == models.TaskTypePlanAndApply {
        var blockingCount int64
        m.db.Model(&models.WorkspaceTask{}).
            Where("workspace_id = ? AND task_type = ? AND id < ? AND status IN (?)",
                workspaceID,
                models.TaskTypePlanAndApply,
                earliestTask.ID,
                []models.TaskStatus{models.TaskStatusPending, models.TaskStatusRunning, models.TaskStatusApplyPending}).
            Count(&blockingCount)

        if blockingCount > 0 {
            log.Printf("[TaskQueue] Plan_and_apply task %d is blocked by %d earlier plan_and_apply tasks",
                earliestTask.ID, blockingCount)
            
            // 被阻塞，查找下一个plan任务
            var nextPlanTask models.WorkspaceTask
            err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
                workspaceID, models.TaskTypePlan, models.TaskStatusPending).
                Order("created_at ASC").
                First(&nextPlanTask).Error
            
            if err == gorm.ErrRecordNotFound {
                log.Printf("[TaskQueue] No plan tasks available, workspace %s must wait", workspaceID)
                return nil, nil
            }
            if err != nil {
                return nil, err
            }
            
            log.Printf("[TaskQueue] Found plan task %d as alternative (created at %v)",
                nextPlanTask.ID, nextPlanTask.CreatedAt)
            return &nextPlanTask, nil
        }

        log.Printf("[TaskQueue] Found plan_and_apply task %d (created at %v), no blocking tasks",
            earliestTask.ID, earliestTask.CreatedAt)
        return &earliestTask, nil
    }

    return &earliestTask, nil
}
```

**优点:**
- 按创建时间公平调度，先创建的任务先执行
- Plan任务不会被Plan+Apply任务"饿死"
- 保持Plan+Apply任务之间的串行执行保证
- 代码改动最小，风险最低

**缺点:**
- Plan+Apply任务可能需要等待Plan任务完成
- 但这是合理的，因为都是用户主动触发的任务

### 方案2: 完全独立的任务队列

**核心思想**: Plan任务和Plan+Apply任务使用完全独立的队列和资源

**实现方式:**

1. **数据库层面**: 添加任务队列类型字段
```sql
ALTER TABLE workspace_tasks ADD COLUMN queue_type VARCHAR(20) DEFAULT 'default';
-- 'plan_queue' for plan tasks
-- 'apply_queue' for plan_and_apply tasks
```

2. **调度层面**: 分别调度两个队列
```go
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // 同时检查两个队列
    planTask := m.getNextPlanTask(workspaceID)
    applyTask := m.getNextApplyTask(workspaceID)
    
    // 如果两个都有，按创建时间选择
    if planTask != nil && applyTask != nil {
        if planTask.CreatedAt.Before(applyTask.CreatedAt) {
            return planTask, nil
        }
        return applyTask, nil
    }
    
    if planTask != nil {
        return planTask, nil
    }
    return applyTask, nil
}
```

3. **资源分配**: 为两种任务类型预留不同的资源
```go
// Agent Pool配置
type AgentPoolConfig struct {
    PlanSlots  int // 专门用于Plan任务的槽位
    ApplySlots int // 专门用于Apply任务的槽位
}
```

**优点:**
- 完全隔离，互不影响
- 可以为不同类型任务配置不同的资源
- 更灵活的调度策略

**缺点:**
- 需要修改数据库schema
- 代码改动较大
- 资源利用率可能降低（资源隔离导致）
- 复杂度增加

### 方案3: 基于权重的调度策略

**核心思想**: 给不同类型的任务分配权重，动态调整优先级

**实现方式:**

```go
type TaskScheduler struct {
    planWeight      int // Plan任务权重
    applyWeight     int // Apply任务权重
    planCounter     int // Plan任务计数器
    applyCounter    int // Apply任务计数器
}

func (s *TaskScheduler) GetNextTask(workspaceID string) (*models.WorkspaceTask, error) {
    // 更新计数器
    s.planCounter += s.planWeight
    s.applyCounter += s.applyWeight
    
    // 选择计数器较大的任务类型
    if s.planCounter > s.applyCounter {
        task := s.getNextPlanTask(workspaceID)
        if task != nil {
            s.planCounter = 0
            return task, nil
        }
    }
    
    task := s.getNextApplyTask(workspaceID)
    if task != nil {
        s.applyCounter = 0
        return task, nil
    }
    
    return s.getNextPlanTask(workspaceID)
}
```

**优点:**
- 灵活的优先级控制
- 可以动态调整权重
- 避免任务饿死

**缺点:**
- 实现复杂
- 需要维护状态
- 可能导致任务执行顺序不符合用户预期

## 推荐方案

**推荐使用方案1: 调整任务优先级策略**

**理由:**
1. **最小改动**: 只需修改 `GetNextExecutableTask` 方法
2. **符合直觉**: 按创建时间排序，先创建的先执行
3. **低风险**: 不涉及数据库schema变更
4. **易于理解**: 逻辑清晰，便于维护
5. **解决问题**: 完全解决Plan任务被"饿死"的问题

## 实施步骤

### 步骤1: 修改 GetNextExecutableTask 方法

文件: `backend/services/task_queue_manager.go`

```go
// GetNextExecutableTask 获取下一个可执行的任务
// 优化后的任务执行规则:
// 0. workspace被lock时,所有任务都要等待(最高优先级)
// 1. 按创建时间排序,先创建的任务先执行(公平调度)
// 2. plan任务完全独立,可以并发执行,不受任何plan_and_apply任务阻塞
// 3. plan_and_apply任务之间必须串行执行
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    log.Printf("[TaskQueue] GetNextExecutableTask for workspace %s", workspaceID)

    // 0. 首先检查workspace是否被lock
    var workspace models.Workspace
    if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
        return nil, fmt.Errorf("failed to get workspace: %w", err)
    }

    if workspace.IsLocked {
        lockedBy := "unknown"
        if workspace.LockedBy != nil {
            lockedBy = *workspace.LockedBy
        }
        log.Printf("[TaskQueue] Workspace %s is locked by %s, all tasks must wait", workspaceID, lockedBy)
        return nil, nil
    }

    // 1. 获取最早的pending任务（不区分类型，按创建时间排序）
    var earliestTask models.WorkspaceTask
    err := m.db.Where("workspace_id = ? AND status = ?",
        workspaceID, models.TaskStatusPending).
        Order("created_at ASC").
        First(&earliestTask).Error

    if err == gorm.ErrRecordNotFound {
        // 没有pending任务，检查apply_pending任务
        err = m.db.Where("workspace_id = ? AND status = ?",
            workspaceID, models.TaskStatusApplyPending).
            Order("created_at ASC").
            First(&earliestTask).Error
        
        if err == gorm.ErrRecordNotFound {
            log.Printf("[TaskQueue] No pending tasks found for workspace %s", workspaceID)
            return nil, nil
        }
        if err != nil {
            log.Printf("[TaskQueue] Error checking apply_pending tasks: %v", err)
            return nil, err
        }
    } else if err != nil {
        log.Printf("[TaskQueue] Error checking pending tasks: %v", err)
        return nil, err
    }

    // 2. 如果是plan任务，直接返回（plan任务完全独立，不受阻塞）
    if earliestTask.TaskType == models.TaskTypePlan {
        log.Printf("[TaskQueue] Found plan task %d (created at %v), can execute immediately (completely independent)",
            earliestTask.ID, earliestTask.CreatedAt)
        return &earliestTask, nil
    }

    // 3. 如果是plan_and_apply任务，检查是否被其他plan_and_apply任务阻塞
    if earliestTask.TaskType == models.TaskTypePlanAndApply {
        var blockingCount int64
        m.db.Model(&models.WorkspaceTask{}).
            Where("workspace_id = ? AND task_type = ? AND id < ? AND status IN (?)",
                workspaceID,
                models.TaskTypePlanAndApply,
                earliestTask.ID,
                []models.TaskStatus{models.TaskStatusPending, models.TaskStatusRunning, models.TaskStatusApplyPending}).
            Count(&blockingCount)

        if blockingCount > 0 {
            log.Printf("[TaskQueue] Plan_and_apply task %d (created at %v) is blocked by %d earlier plan_and_apply tasks",
                earliestTask.ID, earliestTask.CreatedAt, blockingCount)
            
            // 被阻塞，查找下一个plan任务（plan任务不受阻塞）
            var nextPlanTask models.WorkspaceTask
            err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
                workspaceID, models.TaskTypePlan, models.TaskStatusPending).
                Order("created_at ASC").
                First(&nextPlanTask).Error
            
            if err == gorm.ErrRecordNotFound {
                log.Printf("[TaskQueue] No plan tasks available, workspace %s must wait for plan_and_apply task %d",
                    workspaceID, earliestTask.ID)
                return nil, nil
            }
            if err != nil {
                log.Printf("[TaskQueue] Error checking plan tasks: %v", err)
                return nil, err
            }
            
            log.Printf("[TaskQueue] Found plan task %d as alternative (created at %v, can execute while plan_and_apply is blocked)",
                nextPlanTask.ID, nextPlanTask.CreatedAt)
            return &nextPlanTask, nil
        }

        log.Printf("[TaskQueue] Found plan_and_apply task %d (created at %v), no blocking tasks",
            earliestTask.ID, earliestTask.CreatedAt)
        return &earliestTask, nil
    }

    return &earliestTask, nil
}
```

### 步骤2: 更新注释和文档

更新相关注释，说明新的调度策略:

```go
// 任务执行规则:
// 0. workspace被lock时,所有任务都要等待(最高优先级)
// 1. 按创建时间排序,先创建的任务先执行(公平调度)
//    - 这确保了plan任务不会被plan_and_apply任务"饿死"
//    - 用户触发的任务按照时间顺序得到执行
// 2. plan任务完全独立,可以并发执行,不受任何plan_and_apply任务阻塞
//    - plan任务不会修改实际资源,因此可以安全并发
// 3. plan_and_apply任务之间必须串行执行
//    - running状态的plan_and_apply阻塞其他plan_and_apply任务
//    - pending/apply_pending状态的plan_and_apply阻塞其他plan_and_apply任务
//    - 这确保了资源变更的顺序性和一致性
```

### 步骤3: 测试验证

**测试场景1: Plan任务不被阻塞**
```
1. 创建一个plan_and_apply任务 (Task A)
2. 立即创建一个plan任务 (Task B)
3. 验证: Task A先执行（因为先创建）
4. 创建另一个plan_and_apply任务 (Task C)
5. 验证: Task B可以执行（不被Task C阻塞）
```

**测试场景2: 公平调度**
```
1. 创建plan任务 (Task A)
2. 创建plan_and_apply任务 (Task B)
3. 创建plan任务 (Task C)
4. 验证执行顺序: A -> B -> C (按创建时间)
```

**测试场景3: Plan+Apply串行保证**
```
1. 创建plan_and_apply任务 (Task A)
2. 创建plan_and_apply任务 (Task B)
3. 验证: Task B必须等待Task A完成
4. 创建plan任务 (Task C)
5. 验证: Task C可以在Task A执行期间执行
```

## 预期效果

### 优化前
- Plan+Apply任务优先级高，可能导致Plan任务长时间等待
- 用户感知: "Plan任务被Plan+Apply任务阻塞"

### 优化后
- 按创建时间公平调度
- Plan任务和Plan+Apply任务都能及时执行
- 用户感知: "任务按照创建顺序执行，很公平"

## 风险评估

### 低风险
- 只修改任务选择逻辑
- 不涉及数据库schema变更
- 不影响任务执行逻辑
- 保持Plan+Apply任务的串行保证

### 需要注意
- 确保测试覆盖所有场景
- 监控生产环境的任务执行情况
- 准备回滚方案（保留原代码）

## 回滚方案

如果优化后出现问题，可以快速回滚到原逻辑:

```go
// 回滚到原逻辑: 优先执行plan_and_apply任务
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // ... 原代码 ...
}
```

## 总结

通过调整任务选择的优先级策略，我们可以实现:
1. Plan任务不会被Plan+Apply任务"饿死"
2. 按创建时间公平调度
3. 保持Plan+Apply任务之间的串行执行保证
4. 最小化代码改动和风险

这个方案既解决了用户反馈的问题，又保持了系统的稳定性和可维护性。
