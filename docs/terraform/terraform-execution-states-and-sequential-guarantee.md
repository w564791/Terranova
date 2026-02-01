# Terraform核心执行流程状态与顺序执行保证机制

> **文档版本**: v1.0  
> **创建日期**: 2025-11-08  
> **状态**: 完整总结  
> **相关文档**: [15-terraform-execution-detail.md](workspace/15-terraform-execution-detail.md)

## 📋 概述

本文档详细说明我们的Terraform执行流程中的所有状态定义，以及如何通过任务队列管理器保证任务的顺序执行。

## 🎯 核心状态定义

### 1. 任务类型 (TaskType)

```go
type TaskType string

const (
    TaskTypePlan         TaskType = "plan"           // 单独的Plan任务
    TaskTypeApply        TaskType = "apply"          // 单独的Apply任务（已废弃）
    TaskTypePlanAndApply TaskType = "plan_and_apply" // Plan+Apply组合任务
)
```

**说明**:
- `plan`: 独立的Plan任务，可以并发执行
- `plan_and_apply`: Plan和Apply的组合任务，必须串行执行
- `apply`: 已废弃，不再使用

### 2. 任务状态 (TaskStatus)

```go
type TaskStatus string

const (
    TaskStatusPending      TaskStatus = "pending"       // 等待执行
    TaskStatusWaiting      TaskStatus = "waiting"       // 等待前置任务完成
    TaskStatusRunning      TaskStatus = "running"       // 正在执行
    TaskStatusApplyPending TaskStatus = "apply_pending" // Plan完成，等待用户确认Apply
    TaskStatusSuccess      TaskStatus = "success"       // Plan任务成功完成
    TaskStatusApplied      TaskStatus = "applied"       // Apply任务成功完成
    TaskStatusFailed       TaskStatus = "failed"        // 任务失败
    TaskStatusCancelled    TaskStatus = "cancelled"     // 任务被取消
)
```

**重要说明**: 
-  **没有单独的 `applying` 状态**
- 当任务执行Apply时，`status` = `running`，`stage` = `applying`
- 状态（Status）和阶段（Stage）是分开管理的

**状态说明**:

| 状态 | 含义 | 是否最终状态 | 备注 |
|------|------|-------------|------|
| `pending` | 任务在队列中等待执行 | ❌ | 可以被调度执行 |
| `waiting` | 等待前置任务完成 | ❌ | 暂不使用 |
| `running` | 任务正在执行中 | ❌ | 可能在执行Plan或Apply，通过`stage`字段区分 |
| `apply_pending` | Plan完成，等待用户确认 | ❌ | **特殊状态**：需要用户手动确认 |
| `success` | Plan任务成功完成 |  | Plan任务的最终成功状态 |
| `applied` | Apply任务成功完成 |  | Apply任务的最终成功状态 |
| `failed` | 任务执行失败 |  | 最终失败状态 |
| `cancelled` | 任务被用户取消 |  | 最终取消状态 |

### 3. 执行阶段 (Stage)

```go
// Stage字段记录任务当前所处的执行阶段
type RunStage string

const (
    StagePending         RunStage = "pending"          // 等待执行
    StageFetching        RunStage = "fetching"         // 获取配置
    StagePrePlan         RunStage = "pre_plan"         // Plan前置
    StagePlanning        RunStage = "planning"         // 执行Plan
    StagePostPlan        RunStage = "post_plan"        // Plan后置
    StageCostEstimation  RunStage = "cost_estimation"  // 成本估算
    StagePolicyCheck     RunStage = "policy_check"     // 策略检查
    StagePreApply        RunStage = "pre_apply"        // Apply前置

    StageApplying        RunStage = "applying"         // 执行Apply
    StagePostApply       RunStage = "post_apply"       // Apply后置
    StageCompletion      RunStage = "completion"       // 完成阶段
)
```

**说明**: 
- Stage字段用于跟踪任务在11个执行阶段中的位置（参考TFE标准）
-  **注意**: `applying` 是一个 **Stage（阶段）**，不是Status（状态）
- 当任务执行Apply时：`status` = `running` + `stage` = `applying`

### 4. 执行模式 (ExecutionMode)

```go
type ExecutionMode string

const (
    ExecutionModeLocal ExecutionMode = "local" // 本地执行
    ExecutionModeAgent ExecutionMode = "agent" // Agent执行
    ExecutionModeK8s   ExecutionMode = "k8s"   // K8s执行
)
```

## 🔄 完整状态转换流程

### Plan任务状态转换

```
pending → running (planning) → success
   ↓         ↓                    ↓
cancelled  failed              (最终状态)
```

### Plan+Apply任务状态转换

```
pending → running (planning) → apply_pending → running (applying) → applied
   ↓         ↓                      ↓               ↓                  ↓
cancelled  failed                cancelled        failed          (最终状态)
```

**关键点**:
1. `apply_pending`是一个**特殊的中间状态**，需要用户手动确认
2. 服务器重启时，`apply_pending`状态的任务**不会**被自动执行
3. 只有用户点击"Confirm Apply"后，任务才会从`apply_pending`转换到`running (applying)`

## 🔒 顺序执行保证机制

### 1. 核心执行规则

我们的任务队列管理器(`TaskQueueManager`)通过以下规则保证任务的正确执行顺序：

```go
// 任务执行规则:
// 0. workspace被lock时,所有任务都要等待(最高优先级)
// 1. plan任务完全独立,可以并发执行,不受任何plan_and_apply任务阻塞
// 2. plan_and_apply任务之间必须串行执行
//    - running状态的plan_and_apply阻塞其他plan_and_apply任务
//    - pending/apply_pending状态的plan_and_apply阻塞其他plan_and_apply任务
```

### 2. Workspace锁机制

```go
// 检查workspace是否被lock
if workspace.IsLocked {
    log.Printf("[TaskQueue] Workspace %s is locked, all tasks must wait", workspaceID)
    return nil, nil
}
```

**说明**:
- Workspace被锁定时，**所有任务**（包括plan和plan_and_apply）都必须等待
- 这是最高优先级的阻塞条件

### 3. Plan任务并发执行

```go
// Plan任务完全独立,可以并发执行
var planTask models.WorkspaceTask
err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
    workspaceID, models.TaskTypePlan, models.TaskStatusPending).
    Order("created_at ASC").
    First(&planTask).Error
```

**特点**:
-  Plan任务可以并发执行
-  不受plan_and_apply任务阻塞
-  多个Plan任务可以同时运行

### 4. Plan+Apply任务串行执行

```go
// 检查是否有其他plan_and_apply任务阻塞
var otherBlockingCount int64
m.db.Model(&models.WorkspaceTask{}).
    Where("workspace_id = ? AND task_type = ? AND id < ? AND status IN (?)",
        workspaceID,
        models.TaskTypePlanAndApply,
        planAndApplyTask.ID,
        []models.TaskStatus{
            models.TaskStatusPending, 
            models.TaskStatusRunning, 
            models.TaskStatusApplyPending,
        }).
    Count(&otherBlockingCount)

if otherBlockingCount > 0 {
    log.Printf("[TaskQueue] Plan_and_apply task blocked by earlier tasks")
    return nil, nil
}
```

**保证机制**:
1. **按创建时间排序**: 使用`created_at ASC`确保先创建的任务先执行
2. **检查前序任务**: 只有当所有更早创建的plan_and_apply任务都完成时，才能执行当前任务
3. **阻塞状态**: `pending`、`running`、`apply_pending`状态都会阻塞后续任务

### 5. 任务锁机制

```go
// 获取workspace锁
lockKey := fmt.Sprintf("ws_%s", workspaceID)
lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
mutex := lock.(*sync.Mutex)

mutex.Lock()
defer mutex.Unlock()
```

**说明**:
- 使用`sync.Map`为每个workspace维护一个互斥锁
- 确保同一时刻只有一个goroutine在调度该workspace的任务
- 防止并发调度导致的竞态条件

## 📊 状态转换示例

### 示例1: 单个Plan+Apply任务

```
时间线:
T1: 创建任务 → status=pending, stage=pending
T2: 调度执行 → status=running, stage=planning
T3: Plan完成 → status=apply_pending, stage=apply_pending
T4: 用户确认 → status=running, stage=applying
T5: Apply完成 → status=applied, stage=completion
```

### 示例2: 多个Plan+Apply任务串行

```
任务A: pending → running (planning) → apply_pending
任务B: pending (等待A完成)
任务C: pending (等待A和B完成)

用户确认A:
任务A: apply_pending → running (applying) → applied
任务B: pending → running (planning) → apply_pending (A完成后自动开始)
任务C: pending (继续等待B完成)

用户确认B:
任务B: apply_pending → running (applying) → applied
任务C: pending → running (planning) → apply_pending (B完成后自动开始)
```

### 示例3: Plan任务与Plan+Apply任务并发

```
任务A (plan_and_apply): pending → running (planning)
任务B (plan): pending → running (planning)  可以并发执行
任务C (plan): pending → running (planning)  可以并发执行
任务D (plan_and_apply): pending (等待A完成) ❌ 必须等待
```

## 🚨 特殊状态处理

### 1. apply_pending状态

**特点**:
- 这是一个**需要用户交互**的状态
- 任务已经完成Plan，但还未开始Apply
- 服务器重启时**不会**自动执行

**处理逻辑**:
```go
// RecoverPendingTasks中排除apply_pending
m.db.Model(&models.WorkspaceTask{}).
    Where("status = ?", models.TaskStatusPending). // 只恢复pending
    Distinct("workspace_id").
    Pluck("workspace_id", &workspaceIDs)

// 记录但不恢复apply_pending任务
var applyPendingCount int64
m.db.Model(&models.WorkspaceTask{}).
    Where("status = ?", models.TaskStatusApplyPending).
    Count(&applyPendingCount)

log.Printf("Found %d apply_pending tasks waiting for user confirmation", 
    applyPendingCount)
```

### 2. 孤儿任务清理

**场景**: 服务器重启时，running状态的任务需要清理

```go
func (m *TaskQueueManager) CleanupOrphanTasks() error {
    var orphanTasks []models.WorkspaceTask
    m.db.Where("status = ?", models.TaskStatusRunning).Find(&orphanTasks)
    
    for _, task := range orphanTasks {
        // 特殊处理: apply_pending阶段的任务不应标记为失败
        if task.Stage == "apply_pending" {
            task.Status = models.TaskStatusApplyPending
            m.db.Save(&task)
            continue
        }
        
        // 其他running任务标记为失败
        task.Status = models.TaskStatusFailed
        task.ErrorMessage = "Task interrupted by server restart"
        m.db.Save(&task)
    }
}
```

## 🔄 任务调度流程

### 完整调度流程图

```
TryExecuteNextTask(workspaceID)
    ↓
获取workspace锁 (sync.Mutex)
    ↓
检查workspace是否被lock
    ↓ (未锁定)
GetNextExecutableTask()
    ↓
检查plan_and_apply任务
    ↓
    ├─ 有pending/apply_pending的plan_and_apply
    │   ↓
    │   检查是否有更早的plan_and_apply任务阻塞
    │   ↓
    │   ├─ 有阻塞 → 检查plan任务
    │   └─ 无阻塞 → 返回该plan_and_apply任务
    │
    └─ 无plan_and_apply任务
        ↓
        检查plan任务
        ↓
        ├─ 有pending的plan → 返回该plan任务
        └─ 无plan任务 → 返回nil
    ↓
根据执行模式执行任务
    ↓
    ├─ Local模式 → 直接执行
    ├─ Agent模式 → 推送到Agent
    └─ K8s模式 → 推送到K8s Agent
```

### 关键代码片段

```go
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
    // 0. 检查workspace锁
    if workspace.IsLocked {
        return nil, nil
    }
    
    // 1. 检查plan_and_apply任务
    var planAndApplyTask models.WorkspaceTask
    err := m.db.Where("workspace_id = ? AND task_type = ? AND status IN (?)",
        workspaceID, 
        models.TaskTypePlanAndApply,
        []models.TaskStatus{models.TaskStatusPending, models.TaskStatusApplyPending}).
        Order("created_at ASC").
        First(&planAndApplyTask).Error
    
    if err == nil {
        // 检查是否有更早的任务阻塞
        var blockingCount int64
        m.db.Model(&models.WorkspaceTask{}).
            Where("workspace_id = ? AND task_type = ? AND id < ? AND status IN (?)",
                workspaceID,
                models.TaskTypePlanAndApply,
                planAndApplyTask.ID,
                []models.TaskStatus{
                    models.TaskStatusPending,
                    models.TaskStatusRunning,
                    models.TaskStatusApplyPending,
                }).
            Count(&blockingCount)
        
        if blockingCount == 0 {
            return &planAndApplyTask, nil
        }
    }
    
    // 2. 检查plan任务（完全独立，可并发）
    var planTask models.WorkspaceTask
    err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
        workspaceID, 
        models.TaskTypePlan, 
        models.TaskStatusPending).
        Order("created_at ASC").
        First(&planTask).Error
    
    if err == nil {
        return &planTask, nil
    }
    
    return nil, nil
}
```

## 📝 总结

### 核心状态

1. **任务类型**: `plan`（可并发）、`plan_and_apply`（必须串行）
2. **任务状态（Status）**: 8个状态，其中4个是最终状态
   -  **没有 `applying` 状态**，Apply执行时使用 `running` 状态
3. **执行阶段（Stage）**: 11个阶段（参考TFE标准）
   -  **有 `applying` 阶段**，用于标识正在执行Apply
4. **Status vs Stage**: 状态和阶段是分开的，`running` 状态可以对应多个阶段（`planning`、`applying`等）

### 顺序执行保证

1. **Workspace锁**: 最高优先级，锁定时所有任务等待
2. **任务锁**: 使用`sync.Mutex`防止并发调度
3. **Plan任务**: 完全独立，可以并发执行
4. **Plan+Apply任务**: 严格串行，按创建时间顺序执行
5. **状态检查**: 通过检查前序任务状态确保顺序

### 特殊处理

1. **apply_pending**: 需要用户确认，不会自动执行
2. **服务器重启**: 清理孤儿任务，恢复pending任务
3. **重试机制**: 失败任务使用指数退避重试

---

**相关文档**:
- [15-terraform-execution-detail.md](workspace/15-terraform-execution-detail.md) - 完整执行流程设计
- [04-task-workflow.md](workspace/04-task-workflow.md) - 任务工作流
- [task_queue_manager.go](../backend/services/task_queue_manager.go) - 任务队列管理器实现
- [workspace.go](../backend/internal/models/workspace.go) - 数据模型定义
