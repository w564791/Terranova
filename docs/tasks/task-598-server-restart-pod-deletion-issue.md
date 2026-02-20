# Server重启时删除预留槽位Pod的问题

## 问题描述

**现象**: Server重启时，有reserved槽位（apply_pending任务）的Pod被删除了

**是否符合预期**: ❌ **不符合预期**

**预期行为**: 
- Server重启时应该保留所有Pod
- 特别是有reserved槽位的Pod（apply_pending任务等待用户确认）
- Pod应该持续运行，不受server重启影响

## 问题分析

### 可能的原因

#### 1. Pod的RestartPolicy设置问题

**当前设置**: `RestartPolicy: Never`

**问题**: 
- `RestartPolicy: Never`意味着Pod不会自动重启
- 但这不应该导致Pod被删除
- 需要检查是否有其他逻辑在重启时删除Pod

#### 2. EnsurePodsForPool在启动时的行为

**当前逻辑**:
```go
func (s *K8sDeploymentService) EnsurePodsForPool(ctx context.Context, pool *models.AgentPool) error {
    // 1. Sync existing Pods from K8s
    s.podManager.SyncPodsFromK8s(ctx, pool.PoolID)
    
    // 2. Get current Pod count
    currentCount := s.podManager.GetPodCount(pool.PoolID)
    
    // 3. Ensure minimum number of Pods
    for currentCount < minReplicas {
        s.podManager.CreatePod(...)
        currentCount++
    }
}
```

**分析**: 
-  这个逻辑是正确的
-  只会创建新Pod，不会删除现有Pod
-  SyncPodsFromK8s会保留K8s中已存在的Pod

#### 3. AutoScalePods在启动时的行为

**可能的问题**: 
- Server重启后，PodManager的内存状态丢失
- SyncPodsFromK8s重新加载Pod，但槽位状态初始化为idle
- AutoScalePods看到所有槽位都是idle，触发缩容
- 删除了"看起来空闲"的Pod

**关键代码**:
```go
// 在SyncPodsFromK8s中
managedPod = &ManagedPod{
    // ...
}
// 初始化槽位为idle
for i := 0; i < 3; i++ {
    managedPod.Slots[i] = PodSlot{
        SlotID:    i,
        Status:    "idle",  //  初始化为idle
        // ...
    }
}
```

**然后在ReconcilePods中**:
```go
// syncTaskStatusToSlots会从数据库同步任务状态
// 但这需要时间，可能在auto-scaler运行之前还没完成
```

## 根本原因

**时序问题**:
```
1. Server重启
2. EnsurePodsForPool: SyncPodsFromK8s加载Pod（槽位初始化为idle）
3. AutoScaler启动（5秒间隔）
4. 第一次AutoScaler运行：看到所有槽位都是idle
5. 触发缩容 → 删除Pod ❌
6. ReconcilePods运行：从数据库同步任务状态（但Pod已被删除）
```

## 解决方案

### 方案1: 启动时延迟AutoScaler

**实施**: 在main.go中延迟启动AutoScaler

```go
// 启动auto-scaler前先等待Pod状态同步
go func() {
    // 等待10秒让Pod状态完全同步
    time.Sleep(10 * time.Second)
    
    log.Println("[K8sDeployment] Starting auto-scaler after initial sync delay")
    k8sDeploymentService.StartAutoScaler(ctx, 5*time.Second)
}()
```

### 方案2: 在SyncPodsFromK8s后立即同步任务状态

**实施**: 在EnsurePodsForPool中

```go
func (s *K8sDeploymentService) EnsurePodsForPool(ctx context.Context, pool *models.AgentPool) error {
    // 1. Sync existing Pods from K8s
    s.podManager.SyncPodsFromK8s(ctx, pool.PoolID)
    
    // 2. 立即同步任务状态到槽位（重要！）
    s.podManager.ReconcilePods(ctx, pool.PoolID)
    
    // 3. Get current Pod count
    currentCount := s.podManager.GetPodCount(pool.PoolID)
    // ...
}
```

### 方案3: AutoScalePods添加启动保护

**实施**: 在AutoScalePods中

```go
func (s *K8sDeploymentService) AutoScalePods(ctx context.Context, pool *models.AgentPool) (int, bool, error) {
    // 检查是否刚启动（通过检查PodManager的初始化时间）
    // 如果启动时间<30秒，跳过缩容操作
    
    // 或者：检查是否有apply_pending任务
    var applyPendingCount int64
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", pool.PoolID).
        Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
        Count(&applyPendingCount)
    
    if applyPendingCount > 0 {
        log.Printf("[K8sPodService] Pool %s has %d apply_pending tasks, skipping scale-down to protect reserved slots", 
            pool.PoolID, applyPendingCount)
        // 不缩容
    }
}
```

## 推荐方案

**组合方案2 + 方案3**:

1. **方案2**: 在EnsurePodsForPool中立即调用ReconcilePods
   - 确保Pod加载后立即同步任务状态
   - 槽位状态正确反映reserved状态

2. **方案3**: 在AutoScalePods中检查apply_pending任务
   - 如果有apply_pending任务，不执行缩容
   - 额外的保护层

## 实施步骤

### 1. 修改EnsurePodsForPool

```go
// 在SyncPodsFromK8s后立即调用ReconcilePods
if err := s.podManager.SyncPodsFromK8s(ctx, pool.PoolID); err != nil {
    log.Printf("[K8sPodService] Warning: failed to sync pods from K8s: %v", err)
}

// 立即同步任务状态到槽位
if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
    log.Printf("[K8sPodService] Warning: failed to reconcile pods: %v", err)
}
```

### 2. 修改AutoScalePods添加apply_pending保护

```go
// 在决定缩容之前，检查是否有apply_pending任务
if desiredPodCount < currentPodCount {
    // 检查是否有apply_pending任务
    var applyPendingCount int64
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", pool.PoolID).
        Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
        Count(&applyPendingCount)
    
    if applyPendingCount > 0 {
        log.Printf("[K8sPodService] Pool %s has %d apply_pending tasks, skipping scale-down", 
            pool.PoolID, applyPendingCount)
        return currentPodCount, false, nil
    }
}
```

## 为什么会发生这个问题

### 设计缺陷

**原始设计假设**:
- ReconcilePods会在auto-scaler之前运行
- 槽位状态会在缩容决策前正确同步

**实际情况**:
- Server重启时，PodManager内存状态丢失
- SyncPodsFromK8s重新加载Pod，槽位初始化为idle
- Auto-scaler可能在ReconcilePods完成前运行
- 导致基于错误状态做出缩容决策

### 时序竞争

```
时间线:
T0: Server重启
T1: EnsurePodsForPool: SyncPodsFromK8s (槽位=idle)
T2: AutoScaler第一次运行 (看到槽位=idle，触发缩容)
T3: ReconcilePods运行 (同步任务状态，但Pod已被删除)
```

## 临时解决方案

如果task 598的Pod已被删除：

1. **重新创建Pod**: Auto-scaler会自动创建新Pod
2. **等待Pod上线**: 新Pod启动并注册
3. **重新分配任务**: Task 598会被分配到新Pod
4. **重新执行apply**: 使用保存的plan文件

**注意**: 虽然Pod被删除了，但plan文件仍然保存在工作目录中，apply可以正常执行。

## 长期解决方案

实施上述的组合方案2 + 方案3，确保：
1.  Pod加载后立即同步槽位状态
2.  有apply_pending任务时不缩容
3.  多层保护机制

## 相关文档

- `docs/task-598-apply-pending-no-slot-diagnosis.md` - 槽位查找问题
- `docs/task-598-apply-pending-slot-reuse-fix.md` - 槽位重用修复

## 下一步行动

1. 实施方案2：在EnsurePodsForPool中立即调用ReconcilePods
2. 实施方案3：在AutoScalePods中添加apply_pending保护
3. 测试server重启场景
4. 验证Pod不会被错误删除
