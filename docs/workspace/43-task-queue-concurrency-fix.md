# Workspace任务队列并发控制修复方案

## 🐛 Bug描述

### 当前问题
根据用户反馈和截图，发现了严重的并发控制bug：
- 同一个workspace中存在多个`plan_completed`（Apply Pending）状态的任务
- 违反了设计文档中的核心原则：**同一个workspace中，plan+apply同时只能存在一个**
- 其他任务应该进入Pending队列，强制顺序执行

### 设计文档要求（15-terraform-execution-detail.md）

根据设计文档，正确的行为应该是：

1. **串行执行**: 同一workspace的任务必须串行执行
2. **队列机制**: 新任务创建时如果有任务在执行，应该进入pending状态
3. **强制顺序**: 即使存在Apply Pending的任务，新任务也需要排队
4. **锁机制**: 执行中的任务应该锁定workspace

## 🔍 当前实现分析

### 问题1: 缺少任务创建时的并发检查

**当前代码** (`workspace_task_controller.go`):
```go
func (c *WorkspaceTaskController) CreatePlanTask(ctx *gin.Context) {
    // ... 省略前面的代码 ...
    
    // 创建任务（只创建一个任务）
    task := &models.WorkspaceTask{
        WorkspaceID:   uint(workspaceID),
        TaskType:      taskType,
        Status:        models.TaskStatusPending,  // 直接设置为Pending
        ExecutionMode: workspace.ExecutionMode,
        CreatedBy:     &uid,
        Stage:         "pending",
        Description:   req.Description,
    }

    if err := c.db.Create(task).Error; err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
        return
    }

    // 异步执行Plan任务 - 立即启动！
    go func() {
        // ... 执行逻辑 ...
    }()
}
```

**问题**:
- ❌ 没有检查是否有其他任务正在执行
- ❌ 没有检查是否有Apply Pending的任务
- ❌ 创建后立即异步执行，不管队列状态
- ❌ 多个任务可以同时进入running状态

### 问题2: 缺少任务队列处理器

**当前实现**:
- ❌ 没有任务队列管理器
- ❌ 没有任务调度器
- ❌ 每个任务创建后立即执行
- ❌ 无法保证串行执行

### 问题3: plan_completed状态处理不当

**当前实现**:
- ❌ plan_completed任务不会阻止新任务创建
- ❌ 新任务可以在plan_completed任务之前执行
- ❌ 没有强制执行顺序

## 💡 修复方案

### 方案概述

实现一个**任务队列管理器**，确保：
1. 同一workspace的任务串行执行
2. 新任务创建时检查队列状态
3. 只有队列头部的pending任务才能执行
4. plan_completed任务会阻塞队列

### 核心组件

#### 1. 任务队列管理器

```go
// services/task_queue_manager.go

type TaskQueueManager struct {
    db            *gorm.DB
    executor      *TerraformExecutor
    workspaceLocks sync.Map // workspace_id -> *sync.Mutex
}

func NewTaskQueueManager(db *gorm.DB, executor *TerraformExecutor) *TaskQueueManager {
    return &TaskQueueManager{
        db:       db,
        executor: executor,
    }
}

// 检查workspace是否可以执行新任务
func (m *TaskQueueManager) CanExecuteNewTask(workspaceID uint) (bool, string) {
    // 1. 检查是否有running任务
    var runningCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND status = ?", workspaceID, "running").
        Count(&runningCount)
    
    if runningCount > 0 {
        return false, "有任务正在执行中"
    }
    
    // 2. 检查是否有plan_and_apply任务处于plan_completed状态（真正的Apply Pending）
    // 注意：只有plan_and_apply类型的plan_completed才会阻塞队列
    var planAndApplyPendingCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND task_type = ? AND status = ?", 
            workspaceID, "plan_and_apply", "plan_completed").
        Count(&planAndApplyPendingCount)
    
    if planAndApplyPendingCount > 0 {
        return false, "有plan_and_apply任务等待Apply确认"
    }
    
    // 3. 检查是否有apply_pending任务
    var applyPendingCount int64
    m.db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND status = ?", workspaceID, "apply_pending").
        Count(&applyPendingCount)
    
    if applyPendingCount > 0 {
        return false, "有任务等待Apply执行"
    }
    
    return true, ""
}

// 获取下一个可执行的任务
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID uint) (*models.WorkspaceTask, error) {
    var task models.WorkspaceTask
    
    // 按创建时间排序，获取最早的pending任务
    err := m.db.Where("workspace_id = ? AND status = ?", workspaceID, "pending").
        Order("created_at ASC").
        First(&task).Error
    
    if err == gorm.ErrRecordNotFound {
        return nil, nil // 没有pending任务
    }
    
    if err != nil {
        return nil, err
    }
    
    return &task, nil
}

// 尝试执行下一个任务
func (m *TaskQueueManager) TryExecuteNextTask(workspaceID uint) error {
    // 1. 获取workspace锁
    lockKey := fmt.Sprintf("ws_%d", workspaceID)
    lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
    mutex := lock.(*sync.Mutex)
    
    mutex.Lock()
    defer mutex.Unlock()
    
    // 2. 检查是否可以执行
    canExecute, reason := m.CanExecuteNewTask(workspaceID)
    if !canExecute {
        log.Printf("Workspace %d cannot execute new task: %s", workspaceID, reason)
        return nil
    }
    
    // 3. 获取下一个任务
    task, err := m.GetNextExecutableTask(workspaceID)
    if err != nil {
        return err
    }
    
    if task == nil {
        log.Printf("No pending tasks for workspace %d", workspaceID)
        return nil
    }
    
    // 4. 执行任务
    log.Printf("Starting task %d for workspace %d", task.ID, workspaceID)
    go m.executeTask(task)
    
    return nil
}

// 执行任务
func (m *TaskQueueManager) executeTask(task *models.WorkspaceTask) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
    defer cancel()
    
    // 更新任务状态为running
    task.Status = models.TaskStatusRunning
    task.StartedAt = timePtr(time.Now())
    m.db.Save(task)
    
    // 根据任务类型执行
    var err error
    switch task.TaskType {
    case models.TaskTypePlan, models.TaskTypePlanAndApply:
        err = m.executor.ExecutePlan(ctx, task)
    case models.TaskTypeApply:
        err = m.executor.ExecuteApply(ctx, task)
    default:
        err = fmt.Errorf("unknown task type: %s", task.TaskType)
    }
    
    if err != nil {
        task.Status = models.TaskStatusFailed
        task.ErrorMessage = err.Error()
        task.CompletedAt = timePtr(time.Now())
        m.db.Save(task)
        log.Printf("Task %d failed: %v", task.ID, err)
    }
    
    // 任务完成后，尝试执行下一个任务
    m.TryExecuteNextTask(task.WorkspaceID)
}
```

#### 2. 修改任务创建逻辑

```go
// controllers/workspace_task_controller.go

func (c *WorkspaceTaskController) CreatePlanTask(ctx *gin.Context) {
    // ... 前面的验证代码保持不变 ...
    
    // 创建任务
    task := &models.WorkspaceTask{
        WorkspaceID:   uint(workspaceID),
        TaskType:      taskType,
        Status:        models.TaskStatusPending, // 始终创建为pending
        ExecutionMode: workspace.ExecutionMode,
        CreatedBy:     &uid,
        Stage:         "pending",
        Description:   req.Description,
    }

    if err := c.db.Create(task).Error; err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
        return
    }
    
    // ⭐ 关键修改：不再立即异步执行，而是通知队列管理器
    go func() {
        if err := c.queueManager.TryExecuteNextTask(uint(workspaceID)); err != nil {
            log.Printf("Failed to start task execution: %v", err)
        }
    }()
    
    // 返回创建的任务信息
    var message string
    if taskType == models.TaskTypePlanAndApply {
        message = "Plan+Apply task created and queued"
    } else {
        message = "Plan task created and queued"
    }

    ctx.JSON(http.StatusCreated, gin.H{
        "message": message,
        "task":    task,
    })
}
```

#### 3. 任务完成后触发下一个任务

```go
// services/terraform_executor.go

func (s *TerraformExecutor) ExecutePlan(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // ... 执行Plan的代码 ...
    
    // Plan执行完成后
    if task.TaskType == models.TaskTypePlanAndApply {
        // plan_and_apply任务完成plan阶段后，状态变为plan_completed
        task.Status = models.TaskStatusPlanCompleted
        task.Stage = "plan_completed"
    } else {
        // 普通plan任务直接完成
        task.Status = models.TaskStatusSuccess
        task.Stage = "completed"
    }
    
    task.CompletedAt = timePtr(time.Now())
    s.db.Save(task)
    
    // ⭐ 关键：如果是普通plan任务完成，尝试执行下一个任务
    if task.TaskType == models.TaskTypePlan {
        go s.queueManager.TryExecuteNextTask(task.WorkspaceID)
    }
    
    return nil
}

func (s *TerraformExecutor) ExecuteApply(
    ctx context.Context,
    task *models.WorkspaceTask,
) error {
    // ... 执行Apply的代码 ...
    
    // Apply完成后
    task.Status = models.TaskStatusApplied
    task.Stage = "completed"
    task.CompletedAt = timePtr(time.Now())
    s.db.Save(task)
    
    // ⭐ 关键：Apply完成后，尝试执行下一个任务
    go s.queueManager.TryExecuteNextTask(task.WorkspaceID)
    
    return nil
}
```

#### 4. 用户确认Apply后的处理

```go
// controllers/workspace_task_controller.go

func (c *WorkspaceTaskController) ConfirmApply(ctx *gin.Context) {
    // ... 前面的验证代码 ...
    
    // 更新任务状态
    task.ApplyDescription = req.ApplyDescription
    task.Status = models.TaskStatusApplyPending
    task.Stage = "apply_pending"

    if err := c.db.Save(&task).Error; err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
        return
    }

    // ⭐ 关键修改：不再立即异步执行，而是通知队列管理器
    go func() {
        if err := c.queueManager.TryExecuteNextTask(task.WorkspaceID); err != nil {
            log.Printf("Failed to start apply execution: %v", err)
        }
    }()

    ctx.JSON(http.StatusOK, gin.H{
        "message": "Apply queued for execution",
        "task":    task,
    })
}
```

#### 5. 任务取消后的处理

```go
func (c *WorkspaceTaskController) CancelTask(ctx *gin.Context) {
    // ... 取消任务的代码 ...
    
    // 更新任务状态
    task.Status = models.TaskStatusCancelled
    task.CompletedAt = timePtr(time.Now())
    task.ErrorMessage = "Task cancelled by user"

    if err := c.db.Save(&task).Error; err != nil {
        ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel task"})
        return
    }

    // ⭐ 关键：任务取消后，尝试执行下一个任务
    go c.queueManager.TryExecuteNextTask(task.WorkspaceID)

    ctx.JSON(http.StatusOK, gin.H{
        "message": "Task cancelled successfully",
        "task":    task,
    })
}
```

## 📋 修复步骤

### Step 1: 创建TaskQueueManager服务

**文件**: `backend/services/task_queue_manager.go`

**功能**:
- 检查workspace是否可以执行新任务
- 获取下一个可执行的任务
- 管理workspace级别的执行锁
- 任务完成后自动触发下一个任务

### Step 2: 修改任务创建逻辑

**文件**: `backend/controllers/workspace_task_controller.go`

**修改点**:
- `CreatePlanTask`: 创建任务后不立即执行，通知队列管理器
- `ConfirmApply`: 确认后不立即执行，通知队列管理器
- `CancelTask`: 取消后触发下一个任务

### Step 3: 修改任务执行逻辑

**文件**: `backend/services/terraform_executor.go`

**修改点**:
- `ExecutePlan`: 完成后触发下一个任务（仅普通plan）
- `ExecuteApply`: 完成后触发下一个任务

### Step 4: 初始化队列管理器

**文件**: `backend/main.go` 或 `backend/internal/router/router.go`

**修改点**:
- 创建TaskQueueManager实例
- 注入到Controller中

### Step 5: 启动时恢复队列

**功能**: 系统启动时，检查所有workspace的pending任务并尝试执行

```go
func (m *TaskQueueManager) RecoverPendingTasks() error {
    // 获取所有有pending任务的workspace
    var workspaceIDs []uint
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", "pending").
        Distinct("workspace_id").
        Pluck("workspace_id", &workspaceIDs)
    
    // 为每个workspace尝试执行下一个任务
    for _, wsID := range workspaceIDs {
        go m.TryExecuteNextTask(wsID)
    }
    
    return nil
}
```

## 🎯 预期行为

### 场景1: 创建新任务时有任务在执行

```
当前状态: Task #100 (running)
操作: 创建 Task #101
结果: Task #101 状态为 pending，等待Task #100完成
```

### 场景2: 创建新任务时有Apply Pending任务

```
当前状态: Task #100 (plan_completed/Apply Pending)
操作: 创建 Task #101
结果: Task #101 状态为 pending，等待Task #100被确认并完成Apply
```

### 场景3: 多个任务排队

```
当前状态: 
  - Task #100 (running)
  - Task #101 (pending)
  - Task #102 (pending)

Task #100完成后:
  → Task #101 自动开始执行
  → Task #102 继续等待

Task #101完成后:
  → Task #102 自动开始执行
```

### 场景4: plan_and_apply任务流程

```
1. 创建Task #100 (plan_and_apply)
   → 状态: pending
   
2. 队列管理器启动Task #100
   → 状态: running (执行plan)
   
3. Plan完成
   → 状态: plan_completed (等待用户确认)
   → 此时新任务会被阻塞
   
4. 用户确认Apply
   → 状态: apply_pending
   → 队列管理器检测到可以执行
   → 状态: running (执行apply)
   
5. Apply完成
   → 状态: applied
   → 触发下一个pending任务
```

##  注意事项

### 1. 向后兼容性

- 现有的pending任务需要在系统启动时恢复
- 现有的running任务需要检查是否真的在运行（可能是系统崩溃导致的）

### 2. 死锁预防

- 使用workspace级别的锁，避免全局锁
- 锁的粒度要细，只在检查和更新状态时持有
- 避免在持有锁时执行长时间操作

### 3. 错误处理

- 任务执行失败后，应该触发下一个任务
- 任务取消后，应该触发下一个任务
- 系统崩溃恢复后，应该恢复队列

### 4. 性能考虑

- 队列检查应该高效（使用索引）
- 避免频繁的数据库查询
- 考虑使用Redis作为队列存储（未来优化）

## 📊 数据库查询优化

### 需要的索引

```sql
-- 任务队列查询优化
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_queue 
ON workspace_tasks(workspace_id, status, created_at ASC)
WHERE status IN ('pending', 'running', 'plan_completed', 'apply_pending');

-- 任务状态查询优化
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_status_check
ON workspace_tasks(workspace_id, status)
WHERE status IN ('running', 'plan_completed', 'apply_pending');
```

## 🧪 测试场景

### 测试1: 并发创建任务

```go
func TestConcurrentTaskCreation(t *testing.T) {
    workspaceID := uint(1)
    
    // 并发创建10个任务
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            CreatePlanTask(workspaceID, fmt.Sprintf("Task %d", idx))
        }(i)
    }
    wg.Wait()
    
    // 验证：只有1个任务在running，其他都在pending
    var runningCount, pendingCount int64
    db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND status = ?", workspaceID, "running").
        Count(&runningCount)
    db.Model(&models.WorkspaceTask{}).
        Where("workspace_id = ? AND status = ?", workspaceID, "pending").
        Count(&pendingCount)
    
    assert.Equal(t, int64(1), runningCount)
    assert.Equal(t, int64(9), pendingCount)
}
```

### 测试2: Apply Pending阻塞

```go
func TestApplyPendingBlocking(t *testing.T) {
    workspaceID := uint(1)
    
    // 1. 创建plan_and_apply任务并执行到plan_completed
    task1 := CreatePlanAndApplyTask(workspaceID)
    WaitForStatus(task1.ID, "plan_completed")
    
    // 2. 尝试创建新任务
    task2 := CreatePlanTask(workspaceID)
    
    // 3. 验证：task2应该在pending状态
    time.Sleep(2 * time.Second)
    db.First(&task2, task2.ID)
    assert.Equal(t, "pending", task2.Status)
    
    // 4. 确认task1的Apply
    ConfirmApply(task1.ID)
    WaitForStatus(task1.ID, "applied")
    
    // 5. 验证：task2应该自动开始执行
    time.Sleep(2 * time.Second)
    db.First(&task2, task2.ID)
    assert.Equal(t, "running", task2.Status)
}
```

### 测试3: 任务取消后队列恢复

```go
func TestQueueRecoveryAfterCancel(t *testing.T) {
    workspaceID := uint(1)
    
    // 1. 创建并启动task1
    task1 := CreatePlanTask(workspaceID)
    WaitForStatus(task1.ID, "running")
    
    // 2. 创建task2（应该pending）
    task2 := CreatePlanTask(workspaceID)
    time.Sleep(1 * time.Second)
    db.First(&task2, task2.ID)
    assert.Equal(t, "pending", task2.Status)
    
    // 3. 取消task1
    CancelTask(task1.ID)
    
    // 4. 验证：task2应该自动开始执行
    time.Sleep(2 * time.Second)
    db.First(&task2, task2.ID)
    assert.Equal(t, "running", task2.Status)
}
```

## 📝 实施检查清单

### 开发阶段
- [ ] 创建TaskQueueManager服务
- [ ] 实现CanExecuteNewTask检查
- [ ] 实现GetNextExecutableTask查询
- [ ] 实现TryExecuteNextTask调度
- [ ] 修改CreatePlanTask逻辑
- [ ] 修改ConfirmApply逻辑
- [ ] 修改CancelTask逻辑
- [ ] 修改ExecutePlan完成处理
- [ ] 修改ExecuteApply完成处理
- [ ] 实现RecoverPendingTasks恢复逻辑
- [ ] 添加数据库索引

### 测试阶段
- [ ] 单元测试：CanExecuteNewTask
- [ ] 单元测试：GetNextExecutableTask
- [ ] 集成测试：并发创建任务
- [ ] 集成测试：Apply Pending阻塞
- [ ] 集成测试：任务取消后恢复
- [ ] 集成测试：系统重启后恢复
- [ ] 压力测试：大量并发任务

### 验证阶段
- [ ] 验证同一workspace只有一个任务running
- [ ] 验证plan_completed会阻塞新任务
- [ ] 验证任务按创建顺序执行
- [ ] 验证任务完成后自动执行下一个
- [ ] 验证系统重启后队列恢复

## 🚀 部署建议

### 1. 灰度发布

- 先在测试环境验证
- 选择1-2个workspace进行灰度
- 监控任务执行情况
- 确认无问题后全量发布

### 2. 监控指标

```go
// 添加队列相关指标
var (
    queueLength = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "iac_task_queue_length",
            Help: "Number of pending tasks per workspace",
        },
        []string{"workspace_id"},
    )
    
    queueWaitTime = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "iac_task_queue_wait_seconds",
            Help:    "Time tasks spend in queue",
            Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800},
        },
        []string{"workspace_id"},
    )
)
```

### 3. 告警规则

- 队列长度超过10个任务
- 任务等待时间超过30分钟
- 任务执行失败率超过10%

## 📖 总结

### 核心修复

1. **引入TaskQueueManager** - 集中管理任务队列
2. **任务创建不立即执行** - 通知队列管理器调度
3. **串行执行保证** - 同一workspace只有一个任务running
4. **plan_completed阻塞** - Apply Pending任务会阻塞队列
5. **自动触发下一个** - 任务完成/取消后自动执行下一个

### 关键改进

-  符合设计文档要求
-  保证任务串行执行
-  支持任务队列
-  自动恢复机制
-  向后兼容

### 风险评估

**低风险**:
- 不改变数据库schema
- 不改变API接口
- 只改变内部执行逻辑

**需要注意**:
- 系统重启时的队列恢复
- 长时间pending的任务处理
- 死锁预防

---

**等待用户确认后开始实施修复** ✋
