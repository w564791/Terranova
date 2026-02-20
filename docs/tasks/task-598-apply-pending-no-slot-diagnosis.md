# Task 598 Apply_Pending无法执行问题诊断

## 问题描述

**时间**: 2025-11-08 15:53:08

**任务**: Task 598 (workspace ws-mb7m9ii5ey)  
**状态**: apply_pending  
**Pool**: pool-z73eh8ihywlmgx0x

**错误日志**:
```
[TaskQueue] K8s mode with slot management enabled for task 598
[TaskQueue] No free slot available for task 598: no free slot available in pool pool-z73eh8ihywlmgx0x, will retry
```

## 问题分析

### 根本原因

apply_pending任务需要Slot 0（只有Slot 0可以执行plan_and_apply任务），但当前pool中所有Pod的Slot 0都被占用。

### 可能的情况

1. **Slot 0被其他任务占用**
   - 其他plan_and_apply任务正在执行
   - 其他plan任务占用了Slot 0

2. **Slot 0已经被预留**
   - 该任务的plan阶段已完成
   - Slot 0已经被预留为reserved状态
   - 但FindPodWithFreeSlot没有识别到这是同一个任务

3. **没有Pod或所有Pod离线**
   - Pool中没有Pod
   - 所有Pod的Agent都离线（超过2分钟无心跳）

## 诊断步骤

### 1. 检查Pool中的Pod状态

```sql
-- 查看pool中的agents
SELECT agent_id, agent_name, pool_id, last_ping_at, 
       CASE 
         WHEN last_ping_at > datetime('now', '-2 minutes') THEN 'online'
         ELSE 'offline'
       END as status
FROM agents
WHERE pool_id = 'pool-z73eh8ihywlmgx0x';
```

### 2. 检查任务状态

```sql
-- 查看task 598的详细信息
SELECT id, workspace_id, task_type, status, stage, agent_id, 
       created_at, started_at, completed_at
FROM workspace_tasks
WHERE id = 598;

-- 查看该workspace的所有任务
SELECT id, task_type, status, stage, agent_id
FROM workspace_tasks
WHERE workspace_id = 'ws-mb7m9ii5ey'
ORDER BY id;
```

### 3. 检查槽位状态（需要查看日志）

查找PodManager的日志：
```
grep "Pool pool-z73eh8ihywlmgx0x" backend/logs/*.log | grep -E "slot|Pod"
```

## 可能的解决方案

### 方案1: 修复FindPodWithFreeSlot逻辑

**问题**: apply_pending任务应该能够找到自己已经预留的Slot 0

**修复**: 在FindPodWithFreeSlot中添加特殊处理
```go
// 对于apply_pending任务，查找已经预留给该任务的槽位
if taskType == string(models.TaskTypePlanAndApply) {
    // 先查找是否有预留给该任务的槽位
    for _, pod := range m.pods {
        if pod.PoolID != poolID {
            continue
        }
        
        pod.mu.RLock()
        for i, slot := range pod.Slots {
            if slot.Status == "reserved" && 
               slot.TaskID != nil && 
               *slot.TaskID == taskID {
                pod.mu.RUnlock()
                return pod, i, nil
            }
        }
        pod.mu.RUnlock()
    }
}
```

### 方案2: 修改槽位预留逻辑

**问题**: 槽位预留后状态变为reserved，但FindPodWithFreeSlot只查找idle状态的槽位

**修复**: 
1. 在pushTaskToAgent中，对于apply_pending任务，不调用FindPodWithFreeSlot
2. 直接使用FindPodByTaskID找到已预留的槽位
3. 将槽位状态从reserved改回running

### 方案3: 扩容创建新Pod

**问题**: 如果确实没有空闲的Slot 0，应该触发扩容

**修复**: 
- 检查是否所有Pod的Slot 0都被占用
- 如果是，触发扩容创建新Pod
- 等待新Pod上线后重试

## 推荐方案

**方案2最合适**，因为：
1. apply_pending任务应该在同一个Pod上执行apply
2. 槽位已经被预留，只需要重新激活
3. 不需要查找新的槽位

## 实施步骤

### 修改pushTaskToAgent方法

在`backend/services/task_queue_manager.go`的pushTaskToAgent方法中：

```go
// 2.5. For K8s mode with slot management
if workspace.ExecutionMode == models.ExecutionModeK8s && 
   m.k8sDeploymentSvc != nil && 
   m.k8sDeploymentSvc.podManager != nil {
    
    // 特殊处理：apply_pending任务应该使用已预留的槽位
    if task.Status == models.TaskStatusApplyPending {
        // 查找已预留给该任务的槽位
        pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(task.ID)
        if err == nil {
            // 找到了预留的槽位，直接使用
            selectedPodName = pod.PodName
            selectedSlotID = slotID
            selectedAgentID = pod.AgentID
            
            // 将槽位状态从reserved改为running
            // (实际上不需要改，因为任务执行时会自动更新)
            
            log.Printf("[TaskQueue] Found reserved slot %d on pod %s for apply_pending task %d", 
                slotID, pod.PodName, task.ID)
        } else {
            // 没有找到预留槽位，这不正常，记录警告
            log.Printf("[TaskQueue] Warning: apply_pending task %d has no reserved slot, will try to find free slot", 
                task.ID)
            // 继续执行正常的槽位查找逻辑
        }
    }
    
    // 如果还没有选择槽位（非apply_pending或没有预留槽位）
    if selectedPodName == "" {
        // 正常的槽位查找逻辑
        pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodWithFreeSlot(...)
        // ...
    }
}
```

## 临时解决方案

如果需要立即解决task 598的问题：

1. **手动释放槽位**:
```sql
-- 查看当前任务分配
SELECT id, task_type, status, agent_id 
FROM workspace_tasks 
WHERE workspace_id = 'ws-mb7m9ii5ey' 
AND status IN ('running', 'apply_pending');
```

2. **重启服务器**: 让ReconcilePods重新同步槽位状态

3. **手动触发扩容**: 创建新的Pod

## 下一步行动

1. 实施方案2的修复
2. 测试apply_pending任务的执行
3. 验证槽位预留和重用逻辑
