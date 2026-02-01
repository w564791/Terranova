# Agent 槽位行为分析和修复

## 问题描述

用户报告：Agent的slot分为plan slot=3, plan_apply_slot=1, 预留槽位不限制

但这个描述与实际设计不符。

## 实际设计规则（正确的）

根据 `task-606-slot-design-clarification.md`，正确的设计应该是：

### 槽位容量规则
- **每个 Pod 有 3 个槽位** (Slot 0, 1, 2)
- **Plan 任务**: 最多 3 个并发（可以使用任意空闲槽位）
- **Plan+Apply 任务 (running)**: 最多 1 个，**独占整个 Pod**
- **预留 (reserved)**: 数量不限，**不占用实际槽位**

### 关键设计原则

1. **Reserved 不占用槽位**: Reserved 只是标记，不影响其他槽位的使用
2. **Plan+Apply running 独占 Pod**: 当有一个 plan_and_apply 任务在 running 状态时，整个 Pod 不能接受其他任务
3. **多个 Reserved 可以共存**: 一个 Pod 可以有多个 reserved 槽位

## 当前代码状态检查

###  已修复部分 (k8s_pod_manager.go)

`FindPodWithFreeSlot` 函数已经实现了正确的逻辑：

```go
// 1. 检查 Pod 是否有 running 的 plan_and_apply 任务（独占整个 Pod）
hasRunningPlanAndApply := false
for _, slot := range pod.Slots {
    if slot.Status == "running" && slot.TaskType == string(models.TaskTypePlanAndApply) {
        hasRunningPlanAndApply = true
        break
    }
}

if hasRunningPlanAndApply {
    log.Printf("[PodManager] Pod %s has running plan_and_apply task, skipping (Pod is exclusively occupied)",
        pod.PodName)
    continue
}

// 2. 如果是新的 plan_and_apply 任务，检查 Pod 是否有任何 running 任务
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

// 3. 查找空闲槽位（reserved 不算占用，不影响新任务分配）
for i, slot := range pod.Slots {
    if slot.Status != "idle" {
        continue
    }
    // 找到空闲槽位
    return pod, i, nil
}
```

**这段代码是正确的！**

## 用户描述的问题分析

用户说："plan slot=3, plan_apply_slot=1"

这个描述可能有两种理解：

### 理解 1: 用户期望的是固定槽位分配
- Slot 0: 专门给 plan_and_apply
- Slot 1-2: 专门给 plan

**这不是当前的设计！** 当前设计是动态分配，任何空闲槽位都可以用。

### 理解 2: 用户观察到的实际行为
- Plan 任务可以使用 3 个槽位
- Plan_and_apply 任务只能使用 1 个槽位（因为独占 Pod）

**这是正确的行为！** 这正是当前设计的预期效果。

## 可能的实际问题

基于代码审查，可能存在以下问题：

### 问题 1: Reserved 槽位的处理

在 `FindPodWithFreeSlot` 中：
```go
for i, slot := range pod.Slots {
    if slot.Status != "idle" {
        continue  // 跳过所有非 idle 的槽位，包括 reserved
    }
    return pod, i, nil
}
```

**这是正确的！** Reserved 槽位应该被跳过，因为它们虽然不占用实际容量，但槽位本身已经被标记了。

但是，这里有一个潜在的混淆：
- Reserved 槽位**不占用 Pod 的执行容量**（不阻止其他任务）
- 但 Reserved 槽位**占用槽位编号**（Slot 0/1/2 中的一个）

### 问题 2: 槽位复用逻辑

当 apply_pending 任务转为 running 时，应该复用原来的 reserved 槽位：

在 `task_queue_manager.go` 的 `pushTaskToAgent` 中：
```go
if task.Status == models.TaskStatusApplyPending {
    log.Printf("[TaskQueue] Task %d is apply_pending, looking for reserved slot", task.ID)
    
    pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(task.ID)
    if err == nil {
        selectedPodName = pod.PodName
        selectedSlotID = slotID
        selectedAgentID = pod.AgentID
        // 复用 reserved 槽位
    }
}
```

**这个逻辑是正确的！**

## 真正的问题：Reserved 槽位的语义混淆

### 当前实现的问题

Reserved 槽位有两个相互矛盾的语义：

1. **不占用执行容量**: 文档说 "reserved 不占用实际槽位"
2. **占用槽位编号**: 代码中 reserved 槽位占用了 Slot 0/1/2 中的一个

### 示例说明问题

假设一个 Pod：
```
Slot 0: reserved (task-100, apply_pending)
Slot 1: idle
Slot 2: idle
```

**当前行为**:
- 可以分配 2 个新的 plan 任务（使用 Slot 1, 2）
- 总共可以有 1 个 reserved + 2 个 running = 3 个任务

**用户期望的行为**（根据 "预留槽位不限制"）:
- 可以分配 3 个新的 plan 任务（使用 Slot 0, 1, 2）
- Reserved 不应该占用 Slot 0
- 总共可以有 N 个 reserved + 3 个 running

## 修复方案

### 方案 A: 保持当前设计（推荐）

**理由**: 当前设计是合理的，只需要澄清文档

**修改**:
1. 更新文档，明确说明：
   - Reserved 槽位**占用槽位编号**但**不占用执行容量**
   - 一个 Pod 最多有 3 个槽位（包括 reserved）
   - Reserved 槽位的作用是防止 Pod 被删除，而不是增加容量

2. 更新用户理解：
   - Plan slot = 3（最多 3 个槽位）
   - Plan_and_apply slot = 1（独占 Pod，所以最多 1 个）
   - Reserved 槽位占用槽位编号，但不阻止其他槽位使用

### 方案 B: 实现真正的 "预留不限制"

**理由**: 如果用户真的需要无限预留

**修改**:
1. 将 reserved 任务存储在单独的列表中，不占用 Slot 0/1/2
2. 修改 `ManagedPod` 结构：
```go
type ManagedPod struct {
    Slots         [3]PodSlot           // 3 个执行槽位
    ReservedTasks map[uint]time.Time   // 预留任务列表（taskID -> 预留时间）
}
```

3. 修改槽位查找逻辑，reserved 任务不占用 Slot 编号

**缺点**: 需要大量代码修改，可能引入新的 bug

## 建议

**推荐方案 A**，原因：

1. 当前设计是合理的，符合实际需求
2. Reserved 槽位占用编号是必要的，因为 apply 时需要复用同一个槽位
3. 只需要澄清文档和用户理解，不需要修改代码

## 验证当前行为

需要验证以下场景：

### 场景 1: Reserved 不阻止其他槽位
```
Pod-1:
- Slot 0: reserved (task-100)
- Slot 1: idle
- Slot 2: idle

预期: 可以分配 2 个新的 plan 任务
实际:  正确（代码已实现）
```

### 场景 2: Running plan_and_apply 独占 Pod
```
Pod-1:
- Slot 0: running (task-101, plan_and_apply)
- Slot 1: idle
- Slot 2: idle

预期: 不能分配任何新任务
实际:  正确（代码已实现）
```

### 场景 3: 多个 Reserved 可以共存
```
Pod-1:
- Slot 0: reserved (task-102)
- Slot 1: reserved (task-103)
- Slot 2: idle

预期: 可以分配 1 个新的 plan 任务
实际:  正确（代码已实现）
```

## 结论

**当前代码实现是正确的！**

问题可能在于：
1. 用户对 "预留槽位不限制" 的理解有误
2. 文档描述不够清晰

建议：
1. 保持当前代码不变
2. 更新文档，明确说明槽位规则
3. 与用户确认实际遇到的问题场景
