# Task 606: Slot 设计规则澄清和修复

## 正确的设计规则

根据用户需求，Pod 槽位的正确设计应该是：

### 槽位容量规则
- **Plan 任务**: 最多 3 个并发（可以使用任意槽位）
- **Plan+Apply 任务 (running)**: 最多 1 个，独占整个 Pod
- **预留 (reserved)**: 数量不限 (N)，**不占用实际槽位**

### 关键点
1. **Reserved 不占用槽位**: Reserved 只是标记，表示这个槽位预留给某个 apply_pending 任务，但不影响其他槽位的使用
2. **Plan+Apply running 独占 Pod**: 当有一个 plan_and_apply 任务在 running 状态时，整个 Pod 不能接受其他任务
3. **多个 Reserved 可以共存**: 一个 Pod 可以有多个 reserved 槽位（例如多个 apply_pending 任务）

## 当前实现的问题

### 问题 1: Reserved 槽位被错误地当作占用

在 `k8s_pod_manager.go` 的 `FindPodWithFreeSlot` 函数中：

```go
for i, slot := range pod.Slots {
    if slot.Status != "idle" {
        continue  // 跳过所有非 idle 的槽位，包括 reserved
    }
    // ...
}
```

**问题**: Reserved 槽位被跳过，导致其他槽位无法使用

### 问题 2: 没有检查 Plan+Apply Running 独占规则

当前实现没有检查：如果 Pod 上有一个 plan_and_apply 任务在 running 状态，整个 Pod 应该被标记为不可用。

### 问题 3: 自动扩容没有考虑槽位类型匹配

自动扩容只看槽位利用率，没有考虑：
- 有 pending 的 plan_and_apply 任务
- 但所有 Pod 都有 running 的 plan_and_apply 任务
- 需要创建新 Pod

## 修复方案

### 修复 1: 正确处理 Reserved 槽位

```go
func (m *K8sPodManager) FindPodWithFreeSlot(poolID string, taskType string) (*ManagedPod, int, error) {
    for _, pod := range m.pods {
        // 1. 检查 Pod 是否有 running 的 plan_and_apply 任务
        hasRunningPlanAndApply := false
        for _, slot := range pod.Slots {
            if slot.Status == "running" && slot.TaskType == string(models.TaskTypePlanAndApply) {
                hasRunningPlanAndApply = true
                break
            }
        }
        
        // 2. 如果有 running 的 plan_and_apply，整个 Pod 不可用
        if hasRunningPlanAndApply {
            log.Printf("[PodManager] Pod %s has running plan_and_apply task, skipping (Pod is exclusively occupied)",
                pod.PodName)
            continue
        }
        
        // 3. 如果是新的 plan_and_apply 任务，检查是否有任何 running 任务
        if taskType == string(models.TaskTypePlanAndApply) {
            hasAnyRunningTask := false
            for _, slot := range pod.Slots {
                if slot.Status == "running" {
                    hasAnyRunningTask = true
                    break
                }
            }
            
            if hasAnyRunningTask {
                log.Printf("[PodManager] Pod %s has running tasks, cannot accept plan_and_apply (requires exclusive Pod)",
                    pod.PodName)
                continue
            }
        }
        
        // 4. 查找空闲槽位（reserved 不算占用）
        for i, slot := range pod.Slots {
            if slot.Status == "idle" {
                // 找到空闲槽位
                return pod, i, nil
            }
        }
    }
    
    return nil, -1, fmt.Errorf("no free slot available in pool %s", poolID)
}
```

### 修复 2: 自动扩容考虑任务类型

在 `k8s_deployment_service.go` 的 `AutoScalePods` 函数中，需要检查：

```go
// 检查是否有 pending 的 plan_and_apply 任务但没有可用的 Pod
var pendingPlanAndApplyCount int64
db.Model(&models.WorkspaceTask{}).
    Where("status = ? AND task_type = ?", models.TaskStatusPending, models.TaskTypePlanAndApply).
    Count(&pendingPlanAndApplyCount)

if pendingPlanAndApplyCount > 0 {
    // 检查是否有完全空闲的 Pod（没有任何 running 任务）
    hasIdlePod := false
    for _, pod := range pods {
        hasRunning := false
        for _, slot := range pod.Slots {
            if slot.Status == "running" {
                hasRunning = true
                break
            }
        }
        if !hasRunning {
            hasIdlePod = true
            break
        }
    }
    
    if !hasIdlePod {
        // 需要创建新 Pod
        desiredPodCount = currentPodCount + 1
    }
}
```

## 正确的槽位状态示例

### 示例 1: 一个 Pod 可以同时有多个 Reserved

```
Pod-1:
- Slot 0: reserved (task-600, apply_pending)
- Slot 1: reserved (task-601, apply_pending)
- Slot 2: idle

状态: 可以接受新的 plan 任务（使用 Slot 2）
状态: 可以接受新的 plan_and_apply 任务（使用 Slot 2）
```

### 示例 2: Running Plan+Apply 独占 Pod

```
Pod-1:
- Slot 0: running (task-602, plan_and_apply)
- Slot 1: idle
- Slot 2: idle

状态: 不能接受任何新任务（Pod 被独占）
```

### 示例 3: Running Plan 不独占

```
Pod-1:
- Slot 0: running (task-603, plan)
- Slot 1: running (task-604, plan)
- Slot 2: idle

状态: 可以接受新的 plan 任务（使用 Slot 2）
状态: 不能接受 plan_and_apply 任务（有 running 任务）
```

## 实施计划

1.  修复 `FindPodWithFreeSlot` - 正确处理 reserved 和 running 状态
2.  添加 Plan+Apply 独占检查
3. ⏳ 修复自动扩容逻辑 - 考虑任务类型匹配
4. ⏳ 测试验证

## 预期效果

修复后：
-  Reserved 槽位不会阻塞其他槽位
-  Running plan_and_apply 任务会独占整个 Pod
-  多个 reserved 可以共存
-  当没有可用 Pod 时，自动创建新 Pod
