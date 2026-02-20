# Workspace模块 - 任务工作流

> **文档版本**: v1.0  
> **创建日期**: 2025-10-09  
> **状态**: 完整设计

## 📘 概述

任务工作流是Workspace模块的核心功能，定义了Plan和Apply任务的完整执行流程，包括任务创建、执行、状态管理和结果处理。

## 🎯 任务类型

### 1. Plan任务

**目的**: 预览基础设施变更，不实际执行

**触发条件**:
- 用户手动触发
- 代码变更自动触发（GitOps模式）
- 定期检查（Drift检测）

**执行流程**:
```
1. 创建Plan任务
   ↓
2. Workspace状态: Created → Planning
   ↓
3. 选择执行器（Local/Agent/K8s）
   ↓
4. 执行terraform init
   ↓
5. 执行terraform plan
   ↓
6. 保存Plan输出
   ↓
7. 解析Plan结果
   ↓
8. Workspace状态: Planning → PlanDone
   ↓
9. 任务状态: pending → running → success
   ↓
10. 通知用户（Webhook）
```

### 2. Apply任务

**目的**: 实际执行基础设施变更

**前置条件**:
- Workspace状态必须是PlanDone或WaitingApply
- Workspace未被锁定
- 有有效的Plan结果

**执行流程**:
```
1. 创建Apply任务
   ↓
2. 检查前置条件
   ↓
3. Workspace状态: PlanDone/WaitingApply → Applying
   ↓
4. 锁定Workspace
   ↓
5. 选择执行器（Local/Agent/K8s）
   ↓
6. 执行terraform apply
   ↓
7. 保存Apply输出
   ↓
8. 保存新的State版本
   ↓
9. 解锁Workspace
   ↓
10. Workspace状态: Applying → Completed
   ↓
11. 任务状态: pending → running → success
   ↓
12. 通知用户（Webhook）
```

## 📊 任务状态机

### 任务状态

```go
type TaskStatus string

const (
    TaskStatusPending   TaskStatus = "pending"   // 等待执行
    TaskStatusRunning   TaskStatus = "running"   // 执行中
    TaskStatusSuccess   TaskStatus = "success"   // 成功
    TaskStatusFailed    TaskStatus = "failed"    // 失败
    TaskStatusCancelled TaskStatus = "cancelled" // 已取消
)
```

### 状态转换规则

```
pending → running → success
         ↓
         → failed
         ↓
         → cancelled
```

**转换条件**:
- `pending → running`: TaskWorker开始执行
- `running → success`: 执行成功完成
- `running → failed`: 执行失败
- `pending/running → cancelled`: 用户取消任务

## 🔄 完整工作流

### Plan工作流详解

#### 1. 创建Plan任务

**API**: `POST /api/v1/workspaces/:id/tasks/plan`

**请求体**:
```json
{
  "message": "Update security group rules",
  "variables": {
    "environment": "production"
  }
}
```

**处理逻辑**:
```go
func (c *WorkspaceTaskController) CreatePlanTask(ctx *gin.Context) {
    // 1. 验证Workspace状态
    if workspace.State != StateCreated && workspace.State != StatePlanDone {
        return errors.New("invalid workspace state")
    }
    
    // 2. 创建任务记录
    task := &WorkspaceTask{
        WorkspaceID: workspaceID,
        TaskType:    TaskTypePlan,
        Status:      TaskStatusPending,
        Message:     req.Message,
    }
    
    // 3. 更新Workspace状态
    lifecycleService.TransitionTo(workspace, StatePlanning)
    
    // 4. 保存任务
    db.Create(task)
    
    // 5. 返回任务ID
    return task.ID
}
```

#### 2. TaskWorker执行

**执行逻辑**:
```go
func (w *TaskWorker) ProcessPlanTask(task *WorkspaceTask) error {
    // 1. 更新任务状态为running
    task.Status = TaskStatusRunning
    task.StartedAt = time.Now()
    db.Save(task)
    
    // 2. 选择执行器
    executor := selectExecutor(workspace.ExecutionMode)
    
    // 3. 执行Plan
    result, err := executor.ExecutePlan(task)
    if err != nil {
        task.Status = TaskStatusFailed
        task.Error = err.Error()
        lifecycleService.TransitionTo(workspace, StateFailed)
        return err
    }
    
    // 4. 保存结果
    task.Status = TaskStatusSuccess
    task.Output = result.Output
    task.PlanJSON = result.PlanJSON
    task.CompletedAt = time.Now()
    
    // 5. 更新Workspace状态
    lifecycleService.TransitionTo(workspace, StatePlanDone)
    
    // 6. 发送通知
    notificationService.Send("plan_completed", task)
    
    return nil
}
```

### Apply工作流详解

#### 1. 创建Apply任务

**API**: `POST /api/v1/workspaces/:id/tasks/apply`

**请求体**:
```json
{
  "message": "Apply infrastructure changes",
  "auto_approve": false
}
```

**处理逻辑**:
```go
func (c *WorkspaceTaskController) CreateApplyTask(ctx *gin.Context) {
    // 1. 验证Workspace状态
    if workspace.State != StatePlanDone && workspace.State != StateWaitingApply {
        return errors.New("must run plan first")
    }
    
    // 2. 检查锁定状态
    if workspace.IsLocked {
        return errors.New("workspace is locked")
    }
    
    // 3. 创建任务记录
    task := &WorkspaceTask{
        WorkspaceID: workspaceID,
        TaskType:    TaskTypeApply,
        Status:      TaskStatusPending,
        Message:     req.Message,
    }
    
    // 4. 更新Workspace状态
    lifecycleService.TransitionTo(workspace, StateApplying)
    
    // 5. 锁定Workspace
    workspaceService.LockWorkspace(workspaceID, "applying")
    
    // 6. 保存任务
    db.Create(task)
    
    return task.ID
}
```

#### 2. TaskWorker执行

**执行逻辑**:
```go
func (w *TaskWorker) ProcessApplyTask(task *WorkspaceTask) error {
    // 1. 更新任务状态
    task.Status = TaskStatusRunning
    task.StartedAt = time.Now()
    db.Save(task)
    
    // 2. 选择执行器
    executor := selectExecutor(workspace.ExecutionMode)
    
    // 3. 执行Apply
    result, err := executor.ExecuteApply(task)
    if err != nil {
        task.Status = TaskStatusFailed
        task.Error = err.Error()
        lifecycleService.TransitionTo(workspace, StateFailed)
        workspaceService.UnlockWorkspace(workspace.ID)
        return err
    }
    
    // 4. 保存新的State版本
    stateVersion := &WorkspaceStateVersion{
        WorkspaceID: workspace.ID,
        Version:     workspace.CurrentVersion + 1,
        Content:     result.State,
        Checksum:    calculateChecksum(result.State),
        TaskID:      &task.ID,
    }
    db.Create(stateVersion)
    
    // 5. 更新Workspace
    workspace.CurrentVersion++
    workspace.CurrentStateID = &stateVersion.ID
    
    // 6. 更新任务状态
    task.Status = TaskStatusSuccess
    task.Output = result.Output
    task.CompletedAt = time.Now()
    
    // 7. 解锁Workspace
    workspaceService.UnlockWorkspace(workspace.ID)
    
    // 8. 更新Workspace状态
    lifecycleService.TransitionTo(workspace, StateCompleted)
    
    // 9. 发送通知
    notificationService.Send("apply_completed", task)
    
    return nil
}
```

## 🔒 并发控制

### Workspace锁定

**目的**: 防止并发Apply导致State冲突

**锁定时机**:
- Apply任务开始前
- 手动锁定

**解锁时机**:
- Apply任务完成（成功或失败）
- 手动解锁

**锁定检查**:
```go
func (s *WorkspaceService) CanExecuteApply(workspaceID uint) error {
    var workspace Workspace
    db.First(&workspace, workspaceID)
    
    if workspace.IsLocked {
        return fmt.Errorf("workspace is locked by user %d at %s: %s",
            workspace.LockedBy, workspace.LockedAt, workspace.LockReason)
    }
    
    return nil
}
```

### 任务队列

**实现方式**: 数据库队列 + TaskWorker轮询

**队列逻辑**:
```go
func (w *TaskWorker) GetNextTask() (*WorkspaceTask, error) {
    var task WorkspaceTask
    
    // 按创建时间排序，获取最早的pending任务
    err := db.Where("status = ?", TaskStatusPending).
        Order("created_at ASC").
        First(&task).Error
    
    if err == gorm.ErrRecordNotFound {
        return nil, nil // 没有待处理任务
    }
    
    return &task, err
}
```

## 📝 任务输出

### Plan输出

**包含内容**:
- 资源变更列表（新增/修改/删除）
- 输出变更
- 依赖关系
- 执行日志

**格式**:
```json
{
  "changes": {
    "add": 3,
    "change": 2,
    "destroy": 1
  },
  "resources": [
    {
      "address": "aws_instance.web",
      "mode": "managed",
      "type": "aws_instance",
      "name": "web",
      "change": {
        "actions": ["create"],
        "before": null,
        "after": {
          "ami": "ami-12345678",
          "instance_type": "t2.micro"
        }
      }
    }
  ],
  "output_changes": {
    "instance_ip": {
      "actions": ["create"],
      "after": "10.0.1.100"
    }
  }
}
```

### Apply输出

**包含内容**:
- 执行结果
- 资源变更详情
- 新的State
- 执行日志

**格式**:
```json
{
  "success": true,
  "resources_created": 3,
  "resources_updated": 2,
  "resources_destroyed": 1,
  "outputs": {
    "instance_ip": "10.0.1.100",
    "instance_id": "i-1234567890abcdef0"
  },
  "duration": 45.2
}
```

## 🚨 错误处理

### 常见错误

1. **Terraform错误**
   - 配置语法错误
   - Provider认证失败
   - 资源冲突

2. **State错误**
   - State锁定冲突
   - State损坏
   - 版本不匹配

3. **系统错误**
   - 网络超时
   - 磁盘空间不足
   - 执行器不可用

### 错误处理策略

```go
func (w *TaskWorker) HandleError(task *WorkspaceTask, err error) {
    // 1. 记录错误
    task.Status = TaskStatusFailed
    task.Error = err.Error()
    task.CompletedAt = time.Now()
    
    // 2. 更新Workspace状态
    lifecycleService.TransitionTo(workspace, StateFailed)
    
    // 3. 解锁Workspace（如果已锁定）
    if workspace.IsLocked {
        workspaceService.UnlockWorkspace(workspace.ID)
    }
    
    // 4. 发送通知
    notificationService.Send("task_failed", task)
    
    // 5. 保存任务
    db.Save(task)
}
```

## 🔄 任务重试

### 重试策略

**可重试的错误**:
- 网络超时
- 临时性Provider错误
- 执行器不可用

**不可重试的错误**:
- 配置语法错误
- 权限错误
- State冲突

**重试逻辑**:
```go
func (w *TaskWorker) RetryTask(task *WorkspaceTask) error {
    if task.RetryCount >= MaxRetries {
        return errors.New("max retries exceeded")
    }
    
    // 增加重试计数
    task.RetryCount++
    task.Status = TaskStatusPending
    task.Error = ""
    
    db.Save(task)
    
    return nil
}
```

## 📊 任务监控

### 监控指标

- 任务执行时间
- 任务成功率
- 任务队列长度
- 执行器使用率

### 监控API

```http
GET /api/v1/workspaces/:id/tasks/stats
```

**响应**:
```json
{
  "total_tasks": 100,
  "success_tasks": 85,
  "failed_tasks": 10,
  "cancelled_tasks": 5,
  "avg_duration": 45.2,
  "pending_tasks": 3
}
```

## 🚀 未来扩展：插入任务流

### 概念

在Plan和Apply之间插入额外的任务步骤，如审批、安全扫描等。

### 任务流配置

```json
{
  "workflow": {
    "plan": {
      "next": "security_scan"
    },
    "security_scan": {
      "type": "scan",
      "provider": "checkov",
      "next": "approval"
    },
    "approval": {
      "type": "manual",
      "approvers": ["admin@example.com"],
      "next": "apply"
    },
    "apply": {
      "next": null
    }
  }
}
```

### 执行流程

```
Plan → Security Scan → Approval → Apply
```

---

**相关文档**:
- [00-overview.md](./00-overview.md) - 总览和架构
- [01-lifecycle.md](./01-lifecycle.md) - 生命周期状态机
- [02-execution-modes.md](./02-execution-modes.md) - 执行模式
- [03-state-management.md](./03-state-management.md) - State管理
