# PLAN_AND_APPLY Agent模式只执行Plan阶段问题分析

## 问题描述

在Agent模式（包括K8s Agent）下，PLAN_AND_APPLY类型的任务只执行了Plan阶段，没有自动触发Apply阶段。

任务链接：http://127.0.0.1:5173/workspaces/ws-mb7m9ii5ey/tasks/466?tab=plan

## 根本原因

通过代码分析，发现了完整的执行流程和问题所在：

### 1. PLAN_AND_APPLY任务的正常流程

在Local模式下，PLAN_AND_APPLY任务的完整流程是：

1. **Plan阶段**：
   - 执行terraform plan
   - 保存plan_data和plan_json
   - 如果有变更：设置状态为`plan_completed`，等待用户确认
   - 如果无变更：直接设置状态为`success`，任务完成

2. **Apply阶段**（需要用户确认后）：
   - 用户在前端点击"Apply"按钮
   - 后端将任务状态从`plan_completed`改为`apply_pending`
   - TaskQueueManager检测到`apply_pending`状态的任务
   - 执行terraform apply

### 2. Agent模式的问题

在Agent模式下，问题出在**任务状态转换**和**任务调度**两个环节：

#### 问题1：Plan完成后状态设置

在`terraform_executor.go`的`ExecutePlan`方法中（第1445-1465行）：

```go
// 根据任务类型决定最终状态
if task.TaskType == models.TaskTypePlanAndApply {
    // 检查是否有变更
    totalChanges := task.ChangesAdd + task.ChangesChange + task.ChangesDestroy

    if totalChanges == 0 {
        // 没有变更，直接完成任务，不需要Apply
        task.Status = models.TaskStatusSuccess
        task.Stage = "completed"
        log.Printf("Task %d (plan_and_apply) has no changes, completed without apply", task.ID)
        logger.Info("No changes detected, task completed without apply")
    } else {
        // 有变更，Plan完成后等待用户确认
        task.Status = models.TaskStatusPlanCompleted  // ← 设置为plan_completed
        task.Stage = "plan_completed"
        log.Printf("Task %d (plan_and_apply) plan phase completed, waiting for user confirmation", task.ID)

        // 自动锁定workspace，防止在Plan-Apply期间修改配置
        logger.Info("Locking workspace to prevent configuration changes during plan-apply gap...")
        lockReason := fmt.Sprintf("Locked for apply (task #%d). Do not modify resources/variables until apply completes.", task.ID)
        if err := s.lockWorkspace(workspace.WorkspaceID, "system", lockReason); err != nil {
            logger.Warn("Failed to lock workspace: %v", err)
        } else {
            logger.Info("✓ Workspace locked successfully")
        }
    }
}
```

**这里设置的是`plan_completed`状态，而不是`apply_pending`状态！**

#### 问题2：TaskQueueManager的任务调度逻辑

在`task_queue_manager.go`的`GetNextExecutableTask`方法中（第60-65行）：

```go
// 1. 优先检查是否有apply_pending任务（plan_and_apply的apply阶段）
var applyPendingTask models.WorkspaceTask
err := m.db.Where("workspace_id = ? AND status = ?",
    workspaceID, models.TaskStatusApplyPending).  // ← 只查找apply_pending状态
    Order("created_at ASC").
    First(&applyPendingTask).Error
```

**TaskQueueManager只会查找`apply_pending`状态的任务来执行Apply，而不会查找`plan_completed`状态的任务！**

#### 问题3：Agent模式下缺少状态转换机制

在Local模式下，用户点击"Apply"按钮后，前端会调用API将任务状态从`plan_completed`改为`apply_pending`，然后TaskQueueManager才会调度执行Apply。

但在Agent模式下：
- Agent执行完Plan后，任务状态变为`plan_completed`
- Agent不会自动将状态改为`apply_pending`
- TaskQueueManager不会调度`plan_completed`状态的任务
- **结果：Apply阶段永远不会被执行！**

### 3. K8s Agent模式的额外问题

在K8s Agent模式下，还有一个额外的问题：

在`task_queue_manager.go`的`pushTaskToAgent`方法中（第295-300行）：

```go
// 5. Determine action based on task status
action := "plan"
if task.Status == models.TaskStatusApplyPending {
    action = "apply"
}
```

**只有当任务状态是`apply_pending`时，才会发送"apply"动作给Agent。**

而在`cc_manager.go`的`executeTask`方法中（第234-241行）：

```go
// Execute the task based on action
var execErr error
if action == "apply" {
    log.Printf("[Agent] Executing apply for task %d", taskID)
    execErr = taskExecutor.ExecuteApply(ctx, task)
} else {
    log.Printf("[Agent] Executing plan for task %d", taskID)
    execErr = taskExecutor.ExecutePlan(ctx, task)
}
```

**Agent根据收到的action来决定执行plan还是apply。**

## 问题总结

PLAN_AND_APPLY任务在Agent模式下只执行Plan阶段的根本原因是：

1. **状态不匹配**：Plan完成后设置为`plan_completed`，但TaskQueueManager只调度`apply_pending`状态的任务
2. **缺少状态转换**：没有机制将`plan_completed`自动转换为`apply_pending`
3. **调度逻辑缺陷**：TaskQueueManager的`GetNextExecutableTask`方法不会返回`plan_completed`状态的任务

## 解决方案

### 方案1：自动Apply（推荐用于Agent模式）

在Agent模式下，PLAN_AND_APPLY任务应该自动执行Apply，不需要用户确认。

**修改位置**：`terraform_executor.go`的`ExecutePlan`方法

```go
// 根据任务类型决定最终状态
if task.TaskType == models.TaskTypePlanAndApply {
    // 检查是否有变更
    totalChanges := task.ChangesAdd + task.ChangesChange + task.ChangesDestroy

    if totalChanges == 0 {
        // 没有变更，直接完成任务
        task.Status = models.TaskStatusSuccess
        task.Stage = "completed"
    } else {
        // 有变更
        // 检查执行模式
        if workspace.ExecutionMode == models.ExecutionModeAgent || 
           workspace.ExecutionMode == models.ExecutionModeK8s {
            // Agent/K8s模式：自动设置为apply_pending，让TaskQueueManager调度Apply
            task.Status = models.TaskStatusApplyPending
            task.Stage = "apply_pending"
            log.Printf("Task %d (plan_and_apply) plan completed, auto-triggering apply in agent mode", task.ID)
        } else {
            // Local模式：等待用户确认
            task.Status = models.TaskStatusPlanCompleted
            task.Stage = "plan_completed"
            log.Printf("Task %d (plan_and_apply) plan completed, waiting for user confirmation", task.ID)
        }
        
        // 锁定workspace
        lockReason := fmt.Sprintf("Locked for apply (task #%d)", task.ID)
        s.lockWorkspace(workspace.WorkspaceID, "system", lockReason)
    }
}
```

### 方案2：增强TaskQueueManager调度逻辑

让TaskQueueManager也能调度`plan_completed`状态的任务。

**修改位置**：`task_queue_manager.go`的`GetNextExecutableTask`方法

```go
// 1. 优先检查是否有apply_pending任务
var applyPendingTask models.WorkspaceTask
err := m.db.Where("workspace_id = ? AND status IN (?)",
    workspaceID, 
    []models.TaskStatus{
        models.TaskStatusApplyPending,
        models.TaskStatusPlanCompleted,  // ← 新增：也查找plan_completed
    }).
    Order("created_at ASC").
    First(&applyPendingTask).Error

if err == nil {
    // 如果是plan_completed状态，先转换为apply_pending
    if applyPendingTask.Status == models.TaskStatusPlanCompleted {
        // 检查执行模式，只在Agent/K8s模式下自动转换
        var workspace models.Workspace
        if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err == nil {
            if workspace.ExecutionMode == models.ExecutionModeAgent || 
               workspace.ExecutionMode == models.ExecutionModeK8s {
                applyPendingTask.Status = models.TaskStatusApplyPending
                m.db.Save(&applyPendingTask)
                log.Printf("[TaskQueue] Auto-converted task %d from plan_completed to apply_pending", applyPendingTask.ID)
            }
        }
    }
    return &applyPendingTask, nil
}
```

### 方案3：混合方案（最佳）

结合方案1和方案2，确保在各种情况下都能正常工作：

1. 在`ExecutePlan`中，根据执行模式设置不同的状态
2. 在`TaskQueueManager`中，增加对`plan_completed`状态的处理作为后备机制

## 推荐实施方案

**推荐使用方案1（自动Apply）**，原因：

1. **简单直接**：只需修改一处代码
2. **符合Agent模式的设计理念**：Agent模式本来就是为了自动化执行
3. **避免状态混乱**：不需要在多个地方处理状态转换
4. **用户体验一致**：Agent模式下用户期望的就是自动执行

## 验证步骤

修改后需要验证：

1. **Agent模式**：
   - 创建PLAN_AND_APPLY任务
   - 验证Plan完成后自动触发Apply
   - 验证Apply成功完成

2. **K8s Agent模式**：
   - 创建PLAN_AND_APPLY任务
   - 验证Plan完成后自动触发Apply
   - 验证Apply成功完成

3. **Local模式**：
   - 创建PLAN_AND_APPLY任务
   - 验证Plan完成后等待用户确认
   - 点击Apply按钮后验证Apply执行

## 相关代码位置

- `backend/services/terraform_executor.go`: 第1445-1465行（ExecutePlan方法）
- `backend/services/task_queue_manager.go`: 第60-65行（GetNextExecutableTask方法）
- `backend/services/task_queue_manager.go`: 第295-300行（pushTaskToAgent方法）
- `backend/agent/control/cc_manager.go`: 第234-241行（executeTask方法）
- `backend/internal/models/workspace.go`: TaskStatus定义

## 时间线

- 2025-01-03 21:07: 问题分析完成
- 待实施：代码修改和测试
