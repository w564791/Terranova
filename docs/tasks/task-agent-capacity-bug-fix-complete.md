# Agent容量计算Bug修复完成报告

## 问题总结

同一个workspace,一个plan任务一个plan+apply任务,自动启动了2个pod,但按照agent容量设计(3个plan槽位 + 1个plan+apply槽位),应该只需要1个pod。

## 根本原因

在 `backend/services/k8s_deployment_service.go` 中的 `CountPendingTasksForPool` 方法使用了错误的计算公式:

```go
// 错误的逻辑: 副本数 = 活跃任务总数
desiredReplicas = int32(activeTaskCount)
```

这个公式完全忽略了agent的容量设计,导致每个任务都创建一个pod。

## 修复方案

### 1. 修改 CountPendingTasksForPool 方法

**修改前**:
- 简单统计所有活跃任务总数
- 返回任务总数作为所需agent数

**修改后**:
- 分别统计plan任务和plan+apply任务
- 使用正确的容量计算公式:
  ```
  所需agent数 = max(
      ceil(plan任务数 / 3),
      plan_and_apply任务数
  )
  ```

### 2. 实现的代码变更

```go
func (s *K8sDeploymentService) CountPendingTasksForPool(ctx context.Context, poolID string) (int64, error) {
    // Count plan tasks (pending + running)
    var planTaskCount int64
    err := s.db.WithContext(ctx).
        Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
        Where("workspace_tasks.task_type = ?", models.TaskTypePlan).
        Where("workspace_tasks.status IN (?)", []models.TaskStatus{
            models.TaskStatusPending,
            models.TaskStatusRunning,
        }).
        Count(&planTaskCount).Error

    if err != nil {
        return 0, fmt.Errorf("failed to count plan tasks: %w", err)
    }

    // Count plan_and_apply tasks (pending + running + apply_pending)
    var planAndApplyTaskCount int64
    err = s.db.WithContext(ctx).
        Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspaces.execution_mode = ?", models.ExecutionModeK8s).
        Where("workspace_tasks.task_type = ?", models.TaskTypePlanAndApply).
        Where("workspace_tasks.status IN (?)", []models.TaskStatus{
            models.TaskStatusPending,
            models.TaskStatusRunning,
            models.TaskStatusApplyPending,
        }).
        Count(&planAndApplyTaskCount).Error

    if err != nil {
        return 0, fmt.Errorf("failed to count plan_and_apply tasks: %w", err)
    }

    // Calculate required agents based on capacity
    agentsForPlanTasks := (planTaskCount + 2) / 3 // Ceiling division
    agentsForPlanAndApplyTasks := planAndApplyTaskCount

    requiredAgents := agentsForPlanTasks
    if agentsForPlanAndApplyTasks > requiredAgents {
        requiredAgents = agentsForPlanAndApplyTasks
    }

    log.Printf("[K8sDeployment] Pool %s capacity calculation: plan_tasks=%d, plan_and_apply_tasks=%d, agents_for_plan=%d, agents_for_plan_and_apply=%d, required_agents=%d",
        poolID, planTaskCount, planAndApplyTaskCount, agentsForPlanTasks, agentsForPlanAndApplyTasks, requiredAgents)

    return requiredAgents, nil
}
```

## 修复效果验证

### 场景1: 1个plan + 1个plan+apply (原问题场景)
- **修复前**: 2个pod
- **修复后**: 1个pod ✓
- **计算过程**:
  - plan任务需要的agent数: ceil(1/3) = 1
  - plan+apply任务需要的agent数: 1
  - 所需agent数: max(1, 1) = 1

### 场景2: 3个plan + 1个plan+apply
- **修复前**: 4个pod
- **修复后**: 1个pod ✓
- **计算过程**:
  - plan任务需要的agent数: ceil(3/3) = 1
  - plan+apply任务需要的agent数: 1
  - 所需agent数: max(1, 1) = 1

### 场景3: 4个plan + 0个plan+apply
- **修复前**: 4个pod
- **修复后**: 2个pod ✓
- **计算过程**:
  - plan任务需要的agent数: ceil(4/3) = 2
  - plan+apply任务需要的agent数: 0
  - 所需agent数: max(2, 0) = 2

### 场景4: 6个plan + 2个plan+apply
- **修复前**: 8个pod
- **修复后**: 2个pod ✓
- **计算过程**:
  - plan任务需要的agent数: ceil(6/3) = 2
  - plan+apply任务需要的agent数: 2
  - 所需agent数: max(2, 2) = 2

### 场景5: 10个plan + 1个plan+apply
- **修复前**: 11个pod
- **修复后**: 4个pod ✓
- **计算过程**:
  - plan任务需要的agent数: ceil(10/3) = 4
  - plan+apply任务需要的agent数: 1
  - 所需agent数: max(4, 1) = 4

## 优化效果

### 资源节省
- **场景1**: 节省50% pod资源 (2→1)
- **场景2**: 节省75% pod资源 (4→1)
- **场景3**: 节省50% pod资源 (4→2)
- **场景4**: 节省75% pod资源 (8→2)
- **场景5**: 节省64% pod资源 (11→4)

### 成本优化
- 显著降低K8s集群资源消耗
- 减少不必要的pod创建和销毁
- 提高资源利用率

## 测试建议

### 1. 单元测试
创建测试用例验证 `CountPendingTasksForPool` 方法的计算逻辑:
- 测试各种任务组合场景
- 验证ceiling division计算正确性
- 验证max函数逻辑

### 2. 集成测试
在实际K8s环境中测试:
- 创建不同数量的plan和plan+apply任务
- 观察pod自动扩缩容行为
- 验证任务能正常分配到agent执行

### 3. 监控验证
- 观察日志中的容量计算信息
- 监控pod数量变化
- 确认资源利用率提升

## 日志示例

修复后,日志会显示详细的容量计算过程:

```
[K8sDeployment] Pool pool-xxx capacity calculation: plan_tasks=1, plan_and_apply_tasks=1, agents_for_plan=1, agents_for_plan_and_apply=1, required_agents=1
[K8sDeployment] Auto-scaled pool pool-xxx from 2 to 1 replicas (active tasks: 1, includes pending+running)
```

## 相关文档

- [Agent容量计算Bug分析](./task-agent-capacity-bug-analysis.md)
- [K8s Deployment实现总结](./k8s-deployment-implementation-summary.md)

## 修改文件

- `backend/services/k8s_deployment_service.go`
  - 修改 `CountPendingTasksForPool` 方法
  - 添加详细的容量计算日志

## 后续优化建议

### 1. 配置化容量参数
将agent容量(3个plan槽位 + 1个plan+apply槽位)作为可配置参数:
```go
type AgentCapacityConfig struct {
    PlanTasksPerAgent        int // 默认3
    PlanAndApplyTasksPerAgent int // 默认1
}
```

### 2. 动态容量调整
根据实际任务执行情况,动态调整容量参数:
- 监控agent实际负载
- 根据任务执行时间调整容量
- 优化资源利用率

### 3. 更精细的调度策略
考虑任务优先级和资源需求:
- 高优先级任务优先分配
- 根据任务资源需求(CPU/内存)调度
- 实现更智能的负载均衡

## 总结

此次修复解决了agent容量计算的根本性bug,显著提高了资源利用率,降低了运营成本。修复后的逻辑正确实现了agent容量设计,确保系统按照预期的容量模型运行。
