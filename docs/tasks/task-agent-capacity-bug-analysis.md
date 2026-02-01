# Agent容量计算Bug分析

## 问题描述

按照设定,agent可以承载3个plan任务和1个plan+apply任务,但实际上同一个workspace,一个plan任务一个plan+apply任务,自动启动了2个pod,这不合适。

## 根本原因分析

### 1. Agent容量设计

根据代码注释和设计文档,agent的容量设计应该是:
- **3个plan任务槽位**: 可以同时执行3个plan任务
- **1个plan+apply任务槽位**: 可以执行1个plan+apply任务

这意味着一个agent理论上可以同时处理:
- 3个plan任务 + 1个plan+apply任务 = 4个任务

### 2. 当前的Pod调度逻辑问题

在 `backend/services/k8s_deployment_service.go` 的 `CountPendingTasksForPool` 方法中:

```go
// Count tasks that need agents:
// 1. pending - waiting to be picked up by an agent
// 2. running - currently being executed by an agent
// 3. apply_pending - plan completed, waiting for user to confirm apply (agent must stay alive)
err := s.db.WithContext(ctx).
    Model(&models.WorkspaceTask{}).
    Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
    Where("workspaces.current_pool_id = ?", poolID).
    Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
    Where("workspace_tasks.status IN (?)", []models.TaskStatus{
        models.TaskStatusPending,
        models.TaskStatusRunning,
        models.TaskStatusApplyPending,
    }).
    Count(&count).Error
```

**问题**: 这个计算方法是 `副本数 = 活跃任务数`,完全没有考虑agent的容量设计!

### 3. 自动扩缩容逻辑问题

在 `AutoScaleDeployment` 方法中:

```go
if activeTaskCount == 0 {
    // No active tasks, scale down to min_replicas (can be 0)
    desiredReplicas = int32(k8sConfig.MinReplicas)
} else {
    // Has active tasks, set replicas = active task count
    desiredReplicas = int32(activeTaskCount)
    
    // Respect max_replicas constraint
    if desiredReplicas > int32(k8sConfig.MaxReplicas) {
        desiredReplicas = int32(k8sConfig.MaxReplicas)
    }
    
    // Ensure at least min_replicas
    if desiredReplicas < int32(k8sConfig.MinReplicas) {
        desiredReplicas = int32(k8sConfig.MinReplicas)
    }
}
```

**问题**: `desiredReplicas = activeTaskCount` 这个公式完全错误!

### 4. 实际场景分析

当前场景:
- 1个plan任务 (pending/running)
- 1个plan+apply任务 (pending/running/apply_pending)
- activeTaskCount = 2
- 因此创建了2个pod

**正确的逻辑应该是**:
- 1个agent可以处理: 3个plan + 1个plan+apply = 4个任务
- 因此1个plan + 1个plan+apply = 只需要1个pod!

## 正确的容量计算公式

需要根据任务类型分别计算:

```
plan任务数 = count(task_type='plan' AND status IN ('pending','running'))
plan_and_apply任务数 = count(task_type='plan_and_apply' AND status IN ('pending','running','apply_pending'))

所需agent数 = max(
    ceil(plan任务数 / 3),           # plan任务需要的agent数
    plan_and_apply任务数             # plan+apply任务需要的agent数(每个占用1个agent)
)
```

### 示例计算

**场景1**: 1个plan + 1个plan+apply
- plan任务需要的agent数: ceil(1/3) = 1
- plan+apply任务需要的agent数: 1
- 所需agent数: max(1, 1) = **1个pod** ✓

**场景2**: 3个plan + 1个plan+apply
- plan任务需要的agent数: ceil(3/3) = 1
- plan+apply任务需要的agent数: 1
- 所需agent数: max(1, 1) = **1个pod** ✓

**场景3**: 4个plan + 0个plan+apply
- plan任务需要的agent数: ceil(4/3) = 2
- plan+apply任务需要的agent数: 0
- 所需agent数: max(2, 0) = **2个pod** ✓

**场景4**: 6个plan + 2个plan+apply
- plan任务需要的agent数: ceil(6/3) = 2
- plan+apply任务需要的agent数: 2
- 所需agent数: max(2, 2) = **2个pod** ✓

**场景5**: 10个plan + 1个plan+apply
- plan任务需要的agent数: ceil(10/3) = 4
- plan+apply任务需要的agent数: 1
- 所需agent数: max(4, 1) = **4个pod** ✓

## 修复方案

### 1. 修改 CountPendingTasksForPool 方法

需要分别统计plan和plan+apply任务数,而不是简单的总数。

### 2. 修改 AutoScaleDeployment 方法

使用正确的容量计算公式来确定所需的副本数。

### 3. 考虑配置化

将agent容量配置(3个plan槽位 + 1个plan+apply槽位)作为可配置参数,而不是硬编码。

## 影响范围

- K8s模式的agent pool自动扩缩容
- 资源利用率优化
- 成本优化(避免创建不必要的pod)

## 优先级

**高优先级** - 这个bug导致资源浪费,应该尽快修复。
