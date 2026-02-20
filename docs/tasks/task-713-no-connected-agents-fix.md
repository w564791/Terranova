# Task 713: No Connected Agents Fix

## 问题描述

从日志可以看到：
```
2025/12/04 14:02:40 [TaskQueue] K8s mode with slot management enabled for task 713
2025/12/04 14:02:40 [PodManager] Assigned task 713 (type: plan) to pod iac-agent-pool-z73eh8ihywlmgx0x-1764827465 slot 0
2025/12/04 14:02:40 [TaskQueue] Allocated slot 0 on pod iac-agent-pool-z73eh8ihywlmgx0x-1764827465 for task 713
2025/12/04 14:02:40 [PodManager] Released slot 0 on pod iac-agent-pool-z73eh8ihywlmgx0x-1764827465 (task 713 completed)
2025/12/04 14:02:40 [TaskQueue] Released slot 0 on pod iac-agent-pool-z73eh8ihywlmgx0x-1764827465 (no connected agents)
2025/12/04 14:02:40 [TaskQueue] No connected agents found, task 713 will retry
```

问题：
1. Pod存在（slot被分配成功）
2. 但是没有agent连接到C&C服务器
3. 任务进入重试循环

## 用户约定

1. **非冻结时间至少一个pod存活** - 需要确保min_replicas至少为1
2. **有任务待运行但是agent不足时需要启动新的agent** - 需要在发现没有agent时触发扩容

## 解决方案

### 1. 添加即时扩容触发 (task_queue_manager.go)

当发现没有connected agents时，立即触发K8s扩容检查：

```go
// 3. Get actually connected agents from AgentCCHandler
connectedAgentIDs := m.agentCCHandler.GetConnectedAgents()
if len(connectedAgentIDs) == 0 {
    // If we allocated a slot, release it before retrying
    if selectedPodName != "" {
        m.k8sDeploymentSvc.podManager.ReleaseSlot(selectedPodName, selectedSlotID)
        log.Printf("[TaskQueue] Released slot %d on pod %s (no connected agents)", selectedSlotID, selectedPodName)
    }

    log.Printf("[TaskQueue] No connected agents found, task %d will retry", task.ID)

    // For K8s mode: trigger immediate scale-up check if no agents are connected
    // This ensures pods are created when there are pending tasks but no agents
    if workspace.ExecutionMode == models.ExecutionModeK8s && m.k8sDeploymentSvc != nil {
        go m.triggerK8sScaleUpForPool(*workspace.CurrentPoolID)
    }

    m.scheduleRetry(task.WorkspaceID, 15*time.Second)
    return nil
}
```

### 2. 新增 triggerK8sScaleUpForPool 方法

```go
// triggerK8sScaleUpForPool triggers an immediate scale-up check for a K8s pool
// This is called when there are pending tasks but no connected agents
// It ensures that pods are created to handle the pending tasks
func (m *TaskQueueManager) triggerK8sScaleUpForPool(poolID string) {
    if m.k8sDeploymentSvc == nil {
        return
    }

    log.Printf("[TaskQueue] Triggering immediate scale-up check for pool %s (no connected agents)", poolID)

    // Get the pool
    var pool models.AgentPool
    if err := m.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
        log.Printf("[TaskQueue] Failed to get pool %s for scale-up: %v", poolID, err)
        return
    }

    // Check if pool is K8s type
    if pool.PoolType != models.AgentPoolTypeK8s {
        log.Printf("[TaskQueue] Pool %s is not K8s type, skipping scale-up", poolID)
        return
    }

    // Trigger auto-scale with a short timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // First, ensure pods exist for the pool (this will create min_replicas pods if none exist)
    if err := m.k8sDeploymentSvc.EnsurePodsForPool(ctx, &pool); err != nil {
        log.Printf("[TaskQueue] Failed to ensure pods for pool %s: %v", poolID, err)
    }

    // Then trigger auto-scale to potentially create more pods based on pending tasks
    newCount, scaled, err := m.k8sDeploymentSvc.AutoScalePods(ctx, &pool)
    if err != nil {
        log.Printf("[TaskQueue] Failed to auto-scale pool %s: %v", poolID, err)
        return
    }

    if scaled {
        log.Printf("[TaskQueue] Successfully triggered scale-up for pool %s, new pod count: %d", poolID, newCount)
    } else {
        log.Printf("[TaskQueue] Scale-up check completed for pool %s, no scaling needed (current pods: %d)", poolID, newCount)
    }
}
```

## 工作流程

1. 任务被调度执行
2. TaskQueue尝试分配slot并发送任务给agent
3. 如果没有connected agents：
   - 释放已分配的slot
   - **新增**: 触发即时扩容检查 (`triggerK8sScaleUpForPool`)
   - 调度15秒后重试
4. `triggerK8sScaleUpForPool` 会：
   - 调用 `EnsurePodsForPool` 确保至少有 `min_replicas` 个pod
   - 调用 `AutoScalePods` 根据pending任务数量可能创建更多pod
5. 新创建的pod启动后，agent会连接到C&C服务器
6. 15秒后重试时，应该能找到connected agents

## 关于 min_replicas 的说明

当前实现中，`AutoScalePods` 已经有以下逻辑：

1. **冷启动场景** (totalSlots == 0 || currentPodCount == 0):
   - 如果有pending任务，至少创建1个pod（即使min_replicas=0）
   - 如果没有pending任务，创建min_replicas个pod

2. **正常运行场景**:
   - 缩容时不会低于min_replicas
   - 有pending任务时不会缩容

如果用户需要"非冻结时间至少一个pod存活"，建议将pool的 `min_replicas` 配置为1。

## 修改的文件

1. `backend/services/task_queue_manager.go`
   - 在 `pushTaskToAgent` 中添加即时扩容触发
   - 新增 `triggerK8sScaleUpForPool` 方法

## 测试建议

1. 创建一个K8s pool，设置 `min_replicas = 1`
2. 确保没有pod运行
3. 创建一个pending任务
4. 观察日志，应该看到：
   - `[TaskQueue] No connected agents found, task X will retry`
   - `[TaskQueue] Triggering immediate scale-up check for pool Y`
   - `[K8sPodService] Pool Y cold start: N pending tasks, scaling to 1 pods`
5. 等待pod启动并agent连接
6. 任务应该被成功执行

## 额外修复：Failed状态Pod的自动清理

### 问题

发现有agent的状态是failed，但是没有被自动清理和重建。

### 解决方案

修改 `k8s_pod_manager.go` 中的 `SyncPodsFromK8s` 方法，增加对Failed/Succeeded状态Pod的检测和清理：

```go
// SyncPodsFromK8s 从K8s同步Pod状态
// 同时检测并清理Failed状态的Pod
func (m *K8sPodManager) SyncPodsFromK8s(ctx context.Context, poolID string) error {
    // ... 列出所有Pod ...

    // 收集需要清理的Failed Pod
    var failedPods []string

    for _, k8sPod := range podList.Items {
        podName := k8sPod.Name

        // 检查Pod状态，如果是Failed或Succeeded，需要清理
        if k8sPod.Status.Phase == corev1.PodFailed || k8sPod.Status.Phase == corev1.PodSucceeded {
            log.Printf("[PodManager] Pod %s is in %s state, will be cleaned up", podName, k8sPod.Status.Phase)
            failedPods = append(failedPods, podName)
            continue
        }
        // ... 正常处理Running Pod ...
    }

    // 清理Failed/Succeeded状态的Pod
    for _, podName := range failedPods {
        log.Printf("[PodManager] Cleaning up failed/succeeded pod %s", podName)
        
        // 从管理列表中移除
        m.mu.Lock()
        delete(m.pods, podName)
        m.mu.Unlock()

        // 删除K8s中的Pod
        propagationPolicy := metav1.DeletePropagationBackground
        if err := m.clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
            PropagationPolicy: &propagationPolicy,
        }); err != nil && !errors.IsNotFound(err) {
            log.Printf("[PodManager] Warning: failed to delete failed pod %s: %v", podName, err)
        } else {
            log.Printf("[PodManager] Deleted failed/succeeded pod %s from K8s", podName)
        }
    }
    // ...
}
```

### 工作流程

1. AutoScaler定期运行（默认10秒间隔）
2. 调用 `ReconcilePods` -> `SyncPodsFromK8s`
3. `SyncPodsFromK8s` 检测到Failed/Succeeded状态的Pod
4. 自动删除这些Pod
5. AutoScaler发现Pod数量不足（低于min_replicas或有pending任务）
6. 创建新的Pod来替代

### 修改的文件

1. `backend/services/k8s_pod_manager.go`
   - 修改 `SyncPodsFromK8s` 方法，增加Failed/Succeeded Pod的检测和清理

## 完成日期

2025-12-04
