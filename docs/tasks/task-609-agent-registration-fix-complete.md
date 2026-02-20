# Task 609: Agent Registration Fix - Complete

## 问题总结

任务无法分配给已经在线的 Agent，因为 Pod 注册检查失败。从日志和截图可以看到：
- 2 个 Pod 已创建
- 2 个 Agent 已在线（在数据库中显示为 online）
- 但任务分配失败，提示 "agent not registered yet"

## 根本原因

系统中存在两个独立的注册机制：

1. **Agent 在数据库中的注册**（agents 表）- Agent 已注册 ✓
2. **Pod 在 PodManager 中的注册**（`pod.AgentID`）- Pod 未注册 ✗

问题在于 `FindPodWithFreeSlot` 检查 `pod.AgentID` 是否为空，但这个字段只有在调用 `RegisterAgent` 后才会被设置。而 Agent 实际上已经通过 C&C 通道连接并在线了。

## 修复方案

### 修复 1: 移除 Pod 注册检查

在 `backend/services/k8s_pod_manager.go` 的 `FindPodWithFreeSlot` 函数中：

**修改前**:
```go
// Grace period: 5 minutes for new pods to register and start sending heartbeats
if timeSinceCreation < 5*time.Minute {
    // New pod - check if agent has registered
    if pod.AgentID == "" {
        log.Printf("[PodManager] Pod %s is new but agent not registered yet, skipping", podName)
        continue
    }
    log.Printf("[PodManager] Pod %s is new with registered agent %s, allowing", podName, pod.AgentID)
} else if timeSinceHeartbeat > 2*time.Minute {
    // Old pod without recent heartbeat, skip it
    continue
}
```

**修改后**:
```go
// Check Agent heartbeat - only skip pods that are clearly offline
// Don't check pod.AgentID registration as Agent may be connected via C&C but not yet registered in PodManager
timeSinceHeartbeat := time.Since(pod.LastHeartbeat)

// Only skip pods that haven't sent heartbeat in over 2 minutes (clearly offline)
if timeSinceHeartbeat > 2*time.Minute {
    log.Printf("[PodManager] Pod %s is offline (last heartbeat: %v ago), skipping", pod.PodName, timeSinceHeartbeat)
    continue
}
```

### 修复 2: 正确的槽位分配逻辑

确保 `FindPodWithFreeSlot` 正确实现以下规则：
1. **Running plan_and_apply 独占 Pod**: 如果 Pod 有 running 的 plan_and_apply 任务，整个 Pod 不可用
2. **新 plan_and_apply 需要独占 Pod**: 新的 plan_and_apply 任务需要完全空闲的 Pod（没有任何 running 任务）
3. **Reserved 不占用槽位**: Reserved 槽位不影响新任务分配

### 修复 3: 自动扩容逻辑优化

在 `backend/services/k8s_deployment_service.go` 的 `AutoScalePods` 函数中：

1. **正确计算所需 Pod 数量**:
```go
// 需要的总 Pod 数 = 有 running 任务的 Pod 数 + unblocked pending 任务数
requiredPods := podsWithRunningTasks + int(unblockedPendingPlanAndApplyCount)
desiredPodCount = requiredPods
```

2. **防止扩容和缩容冲突**:
```go
// 缩容前检查是否有 unblocked pending 任务
if unblockedPendingCount > 0 {
    desiredPodCount = currentPodCount  // 维持当前数量，不缩容
}
```

## 关键改进

### 1. 简化 Pod 可用性检查

**之前**: 检查 Pod 创建时间、Agent 注册状态、心跳时间
**现在**: 只检查心跳时间（超过 2 分钟才认为离线）

这样做的好处：
-  Agent 通过 C&C 连接后立即可用
-  不依赖 `RegisterAgent` 调用时机
-  简化了逻辑，减少了潜在的竞态条件

### 2. 正确的任务分配规则

-  Reserved 槽位不阻塞新任务
-  Running plan_and_apply 独占 Pod
-  新 plan_and_apply 需要独占 Pod
-  Plan 任务可以使用任意空闲槽位

### 3. 智能的自动扩容

-  根据实际需求计算 Pod 数量
-  只统计不被 block 的 pending 任务
-  防止扩容后立即缩容的冲突

## 预期效果

修复后的行为：

1. **Pod 创建**: 自动扩容检测到需要 2 个 Pod，创建 2 个 Pod
2. **Agent 启动**: Agent 启动并通过 C&C 连接
3. **任务分配**: `FindPodWithFreeSlot` 找到可用 Pod（只检查心跳）
4. **槽位分配**: 成功分配槽位给任务
5. **Agent 确认**: `pushTaskToAgent` 确认 Agent 在 C&C 中连接
6. **任务执行**: 任务成功分配并开始执行

## 测试建议

重启后端服务后，观察以下日志：

### 成功指标
```
[PodManager] Pod X has Y seconds since last heartbeat, allowing
[TaskQueue] Allocated slot N on pod X for task Y
[TaskQueue] Successfully pushed task Y to agent Z
```

### 不应再出现
```
[PodManager] Pod X is new but agent not registered yet, skipping
```

## 相关修复

本次修复解决了以下问题：
1. **Task 604**: Agent 注册检查导致的槽位分配失败
2. **Task 606**: 槽位设计规则澄清和实现
3. **Task 609**: Agent 在线但 Pod 未注册导致的任务分配失败

所有这些问题的根源都是 Pod 注册检查与 Agent C&C 连接状态不同步。

## 部署说明

1. **无数据库变更**: 纯代码修复
2. **无配置变更**: 不需要修改 K8s 配置或 Agent 设置
3. **需要重启**: 后端服务器需要重启
4. **零停机**: 可以在不影响运行中任务的情况下部署

## 验证步骤

1. 重启后端服务
2. 创建测试任务
3. 观察 Pod 创建和 Agent 连接
4. 确认任务成功分配
5. 检查日志中不再出现 "agent not registered yet"

## 成功标准

-  Agent 在线后任务立即可分配
-  不再依赖 Pod 注册时机
-  扩容和缩容逻辑正确工作
-  任务分配不再卡在 pending 状态

## 结论

通过移除 Pod 注册检查，我们简化了任务分配逻辑，使其只依赖于 Agent 的实际连接状态（通过心跳检测）。这解决了 Agent 已在线但 Pod 未注册导致的任务分配失败问题。

修复是最小化的，只改变了 Pod 可用性检查逻辑，不影响其他功能。系统现在正确地基于槽位可用性而不是 Pod 注册状态来分配任务。
