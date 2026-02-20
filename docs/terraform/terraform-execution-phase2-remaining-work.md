# Phase 2 Pod槽位管理 - 剩余工作清单

## 生成时间: 2025-11-08 14:29

## 总体进度: 20% 完成

---

##  已完成的工作 (20%)

### Phase 2 Step 2.1-2.2: Pod管理器核心 (100%完成)
**完成时间**: 2025-11-08 14:19

**实现内容**:
-  创建`backend/services/k8s_pod_manager.go` (500+行代码)
-  数据结构：PodSlot, ManagedPod, K8sPodManager
-  Pod生命周期管理：CreatePod, DeletePod, ListPods, FindIdlePods
-  槽位管理：FindPodWithFreeSlot, AssignTaskToSlot, ReleaseSlot, ReserveSlot
-  槽位统计：GetSlotStats, GetPodSlotStatus
-  Pod同步：SyncPodsFromK8s, ReconcilePods
-  Agent注册：RegisterAgent, UpdateHeartbeat

### Phase 2 Step 2.3: K8sDeploymentService重构 (20%完成)
**开始时间**: 2025-11-08 14:24

**已完成**:
-  添加`podManager *K8sPodManager`字段到K8sDeploymentService
-  更新`NewK8sDeploymentService`构造函数初始化PodManager
-  创建进度跟踪文档

---

## ❌ 未完成的工作 (80%)

### 1. Step 2.3: 完成K8sDeploymentService重构 (剩余80% - 约1.6天)

#### A. 重构 EnsureDeploymentForPool → EnsurePodsForPool
**状态**: ❌ 未开始  
**工作量**: 0.4天

**当前行为**:
- 创建/更新K8s Deployment
- 设置副本数为0，等待auto-scaler扩容

**目标行为**:
- 同步K8s中的Pod到PodManager
- 确保最小Pod数量（根据min_replicas配置）
- 不再使用Deployment

**实现步骤**:
```go
func (s *K8sDeploymentService) EnsurePodsForPool(ctx context.Context, pool *models.AgentPool) error {
    // 1. 确保Secret存在
    secretName, err := s.EnsureSecretForPool(ctx, pool)
    
    // 2. 从K8s同步Pod状态
    err = s.podManager.SyncPodsFromK8s(ctx, pool.PoolID)
    
    // 3. 获取当前Pod数量
    currentCount := s.podManager.GetPodCount(pool.PoolID)
    
    // 4. 解析K8s配置获取min_replicas
    var k8sConfig models.K8sJobTemplateConfig
    json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig)
    
    // 5. 如果Pod数量少于min_replicas，创建新Pod
    for currentCount < k8sConfig.MinReplicas {
        _, err = s.podManager.CreatePod(ctx, pool.PoolID, &k8sConfig, secretName)
        currentCount++
    }
    
    return nil
}
```

#### B. 重构 ScaleDeployment → ScalePods
**状态**: ❌ 未开始  
**工作量**: 0.4天

**当前行为**:
- 更新Deployment的replicas字段
- K8s随机删除Pod（可能删除正在执行任务的Pod）

**目标行为**:
- 直接创建/删除单个Pod
- 缩容时只删除所有槽位都是idle的Pod
- 保护有running或reserved槽位的Pod

**实现步骤**:
```go
func (s *K8sDeploymentService) ScalePods(ctx context.Context, poolID string, desiredCount int) error {
    // 1. 获取当前Pod数量
    currentCount := s.podManager.GetPodCount(poolID)
    
    // 2. 如果需要扩容
    if desiredCount > currentCount {
        // 获取pool配置
        var pool models.AgentPool
        s.db.First(&pool, "pool_id = ?", poolID)
        
        var k8sConfig models.K8sJobTemplateConfig
        json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig)
        
        secretName, _ := s.EnsureSecretForPool(ctx, &pool)
        
        // 创建新Pod
        for i := currentCount; i < desiredCount; i++ {
            s.podManager.CreatePod(ctx, poolID, &k8sConfig, secretName)
        }
    }
    
    // 3. 如果需要缩容
    if desiredCount < currentCount {
        // 只删除完全空闲的Pod
        idlePods := s.podManager.FindIdlePods(poolID)
        deleteCount := currentCount - desiredCount
        
        for i := 0; i < deleteCount && i < len(idlePods); i++ {
            s.podManager.DeletePod(ctx, idlePods[i].PodName)
        }
    }
    
    return nil
}
```

#### C. 重构 GetDeploymentReplicas → GetPodCount
**状态**: ❌ 未开始  
**工作量**: 0.2天

**当前行为**:
- 查询Deployment的当前和期望副本数

**目标行为**:
- 返回当前Pod数量和期望Pod数量

**实现步骤**:
```go
func (s *K8sDeploymentService) GetPodCount(ctx context.Context, poolID string) (current, desired int, err error) {
    // 1. 同步Pod状态
    err = s.podManager.SyncPodsFromK8s(ctx, poolID)
    if err != nil {
        return 0, 0, err
    }
    
    // 2. 获取当前Pod数量
    current = s.podManager.GetPodCount(poolID)
    
    // 3. 从配置获取期望数量（可以基于任务数计算）
    // 这里简化为返回当前数量作为期望数量
    desired = current
    
    return current, desired, nil
}
```

#### D. 重构 AutoScaleDeployment → AutoScalePods
**状态**: ❌ 未开始  
**工作量**: 0.6天

**当前行为**:
- 基于任务数量计算所需agent数
- 调用ScaleDeployment更新副本数

**目标行为**:
- 基于槽位利用率计算所需Pod数
- 调用ScalePods创建/删除Pod
- 智能缩容：只删除空闲Pod

**关键变更**:
```go
func (s *K8sDeploymentService) AutoScalePods(ctx context.Context, pool *models.AgentPool) (int, bool, error) {
    // 1. 获取槽位统计
    total, used, reserved, idle := s.podManager.GetSlotStats(pool.PoolID)
    
    // 2. 计算所需Pod数
    // 如果有reserved槽位，说明有apply_pending任务，需要保持Pod
    // 如果槽位利用率高（>80%），需要扩容
    utilizationRate := float64(used+reserved) / float64(total)
    
    currentPodCount := s.podManager.GetPodCount(pool.PoolID)
    var desiredPodCount int
    
    if utilizationRate > 0.8 {
        // 扩容：增加1个Pod
        desiredPodCount = currentPodCount + 1
    } else if utilizationRate < 0.2 && currentPodCount > minReplicas {
        // 缩容：减少1个Pod（但只删除空闲Pod）
        desiredPodCount = currentPodCount - 1
    } else {
        desiredPodCount = currentPodCount
    }
    
    // 3. 执行扩缩容
    if desiredPodCount != currentPodCount {
        err := s.ScalePods(ctx, pool.PoolID, desiredPodCount)
        return desiredPodCount, true, err
    }
    
    return currentPodCount, false, nil
}
```

---

### 2. Step 2.4: Auto-scaler更新 (0%完成 - 约2天)

**文件**: `backend/services/k8s_deployment_service.go`

#### A. 更新 runAutoScalerCycle
**状态**: ❌ 未开始  
**工作量**: 1天

**需要修改**:
```go
func (s *K8sDeploymentService) runAutoScalerCycle(ctx context.Context) {
    var pools []models.AgentPool
    s.db.Where("pool_type = ?", models.AgentPoolTypeK8s).Find(&pools)
    
    for _, pool := range pools {
        // 1. Pod协调（同步状态）
        if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
            log.Printf("[K8sDeployment] Error reconciling pods for pool %s: %v", pool.PoolID, err)
        }
        
        // 2. 检查并轮换Secret
        if err := s.checkAndRotateSecret(ctx, &pool); err != nil {
            log.Printf("[K8sDeployment] Error checking secret rotation: %v", err)
        }
        
        // 3. 自动扩缩容（使用新的AutoScalePods）
        _, scaled, err := s.AutoScalePods(ctx, &pool)
        if err != nil {
            log.Printf("[K8sDeployment] Error auto-scaling pool %s: %v", pool.PoolID, err)
            continue
        }
        
        if scaled {
            log.Printf("[K8sDeployment] Successfully scaled pool %s", pool.PoolID)
        }
    }
}
```

#### B. 更新槽位统计逻辑
**状态**: ❌ 未开始  
**工作量**: 1天

**需要实现**:
- 基于槽位状态计算扩缩容需求
- 考虑reserved槽位（apply_pending任务）
- 实现渐进式扩缩容策略

---

### 3. Step 2.5: TaskQueueManager更新 (0%完成 - 约1天)

**文件**: `backend/services/task_queue_manager.go`

#### A. 修改 pushTaskToAgent 使用槽位分配
**状态**: ❌ 未开始  
**工作量**: 0.5天

**当前行为**:
- 直接将任务推送给agent
- 不考虑agent容量

**目标行为**:
- 查找有空闲槽位的Pod
- 分配任务到特定槽位
- 记录槽位分配信息

**实现步骤**:
```go
func (m *TaskQueueManager) pushTaskToAgent(task *models.WorkspaceTask) error {
    // 1. 获取workspace的pool_id
    var workspace models.Workspace
    m.db.First(&workspace, "workspace_id = ?", task.WorkspaceID)
    
    // 2. 查找有空闲槽位的Pod
    pod, slotID, err := m.k8sService.podManager.FindPodWithFreeSlot(
        workspace.CurrentPoolID,
        string(task.TaskType),
    )
    if err != nil {
        return fmt.Errorf("no free slot available: %w", err)
    }
    
    // 3. 分配任务到槽位
    err = m.k8sService.podManager.AssignTaskToSlot(
        pod.PodName,
        slotID,
        task.ID,
        string(task.TaskType),
    )
    
    // 4. 更新任务的agent_id
    task.AgentID = &pod.AgentID
    m.db.Save(task)
    
    // 5. 推送任务到agent（现有逻辑）
    // ...
    
    return nil
}
```

#### B. 任务完成后释放槽位
**状态**: ❌ 未开始  
**工作量**: 0.3天

**需要在任务完成时调用**:
```go
// 任务完成时
pod, slotID, _ := m.k8sService.podManager.FindPodByTaskID(task.ID)
m.k8sService.podManager.ReleaseSlot(pod.PodName, slotID)
```

#### C. Plan完成后预留Slot 0
**状态**: ❌ 未开始  
**工作量**: 0.2天

**用于plan_and_apply任务**:
```go
// Plan阶段完成，进入apply_pending状态时
if task.TaskType == models.TaskTypePlanAndApply && task.Status == models.TaskStatusApplyPending {
    pod, _, _ := m.k8sService.podManager.FindPodByTaskID(task.ID)
    m.k8sService.podManager.ReserveSlot(pod.PodName, 0, task.ID)
}
```

---

### 4. Step 2.6: Agent端槽位管理 (0%完成 - 约1天)

**需要创建**: `backend/agent/worker/slot_manager.go`

#### A. 槽位管理器结构
**状态**: ❌ 未开始  
**工作量**: 0.5天

```go
type SlotManager struct {
    slots     [3]*Slot
    mu        sync.RWMutex
    apiClient *AgentAPIClient
}

type Slot struct {
    ID        int
    TaskID    *uint
    Status    string // idle, running
    StartTime time.Time
}

func NewSlotManager(apiClient *AgentAPIClient) *SlotManager {
    sm := &SlotManager{
        apiClient: apiClient,
    }
    
    // 初始化3个槽位
    for i := 0; i < 3; i++ {
        sm.slots[i] = &Slot{
            ID:     i,
            Status: "idle",
        }
    }
    
    return sm
}
```

#### B. 槽位获取和释放
**状态**: ❌ 未开始  
**工作量**: 0.3天

```go
func (sm *SlotManager) AcquireSlot(taskID uint, taskType string) (int, error) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    // Slot 0可以执行任何任务
    if sm.slots[0].Status == "idle" {
        sm.slots[0].TaskID = &taskID
        sm.slots[0].Status = "running"
        sm.slots[0].StartTime = time.Now()
        return 0, nil
    }
    
    // Slot 1, 2只能执行plan任务
    if taskType == "plan" {
        for i := 1; i < 3; i++ {
            if sm.slots[i].Status == "idle" {
                sm.slots[i].TaskID = &taskID
                sm.slots[i].Status = "running"
                sm.slots[i].StartTime = time.Now()
                return i, nil
            }
        }
    }
    
    return -1, fmt.Errorf("no free slot available")
}

func (sm *SlotManager) ReleaseSlot(slotID int) {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    
    sm.slots[slotID].TaskID = nil
    sm.slots[slotID].Status = "idle"
}
```

#### C. 上报槽位状态
**状态**: ❌ 未开始  
**工作量**: 0.2天

```go
func (sm *SlotManager) ReportStatus() {
    sm.mu.RLock()
    defer sm.mu.RUnlock()
    
    // 定期上报槽位状态到平台
    status := make([]SlotStatus, 3)
    for i := 0; i < 3; i++ {
        status[i] = SlotStatus{
            SlotID: i,
            Status: sm.slots[i].Status,
            TaskID: sm.slots[i].TaskID,
        }
    }
    
    sm.apiClient.ReportSlotStatus(status)
}
```

---

### 5. Step 2.7: main.go更新 (0%完成 - 约0.5天)

**文件**: `backend/main.go`

#### A. 启动Pod协调器
**状态**: ❌ 未开始  
**工作量**: 0.3天

```go
// 在main函数中添加
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // 协调所有K8s pool的Pod状态
            var pools []models.AgentPool
            db.Where("pool_type = ?", models.AgentPoolTypeK8s).Find(&pools)
            
            for _, pool := range pools {
                if err := k8sService.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
                    log.Printf("Error reconciling pods for pool %s: %v", pool.PoolID, err)
                }
            }
        }
    }
}()
```

#### B. 更新服务初始化
**状态**: ❌ 未开始  
**工作量**: 0.2天

确保K8sDeploymentService正确初始化并传递给需要的服务。

---

### 6. Step 2.8: 测试验证 (0%完成 - 约1.5天)

#### A. 编译测试
**状态**: ❌ 未开始  
**工作量**: 0.2天

- 验证所有代码编译通过
- 修复类型错误和导入问题

#### B. 基本功能测试
**状态**: ❌ 未开始  
**工作量**: 0.5天

- 测试Pod创建和删除
- 测试槽位分配和释放
- 测试任务执行流程

#### C. 缩容保护测试
**状态**: ❌ 未开始  
**工作量**: 0.4天

**测试场景**:
1. 创建3个Pod，每个执行1个任务
2. 触发缩容到2个Pod
3. 验证：只删除空闲Pod，有任务的Pod不被删除

#### D. Apply_pending保护测试
**状态**: ❌ 未开始  
**工作量**: 0.4天

**测试场景**:
1. 执行plan_and_apply任务
2. Plan完成后进入apply_pending状态
3. 触发缩容
4. 验证：reserved槽位的Pod不被删除

---

## 工作量总结

| 步骤 | 子任务 | 工作量 | 状态 | 完成度 |
|------|--------|--------|------|--------|
| **2.1-2.2** | Pod管理器核心 | 2天 |  完成 | 100% |
| **2.3** | K8sDeploymentService重构 | 2天 | 🟡 进行中 | 20% |
| - | 添加podManager字段 | 0.1天 |  完成 | 100% |
| - | EnsurePodsForPool | 0.4天 | ❌ 未开始 | 0% |
| - | ScalePods | 0.4天 | ❌ 未开始 | 0% |
| - | GetPodCount | 0.2天 | ❌ 未开始 | 0% |
| - | AutoScalePods | 0.6天 | ❌ 未开始 | 0% |
| - | 文档和测试 | 0.3天 | 🟡 部分完成 | 50% |
| **2.4** | Auto-scaler更新 | 2天 | ❌ 未开始 | 0% |
| - | runAutoScalerCycle | 1天 | ❌ 未开始 | 0% |
| - | 槽位统计逻辑 | 1天 | ❌ 未开始 | 0% |
| **2.5** | TaskQueueManager更新 | 1天 | ❌ 未开始 | 0% |
| - | pushTaskToAgent | 0.5天 | ❌ 未开始 | 0% |
| - | 释放槽位 | 0.3天 | ❌ 未开始 | 0% |
| - | 预留槽位 | 0.2天 | ❌ 未开始 | 0% |
| **2.6** | Agent槽位管理 | 1天 | ❌ 未开始 | 0% |
| - | 槽位管理器结构 | 0.5天 | ❌ 未开始 | 0% |
| - | 获取和释放 | 0.3天 | ❌ 未开始 | 0% |
| - | 上报状态 | 0.2天 | ❌ 未开始 | 0% |
| **2.7** | main.go更新 | 0.5天 | ❌ 未开始 | 0% |
| - | Pod协调器 | 0.3天 | ❌ 未开始 | 0% |
| - | 服务初始化 | 0.2天 | ❌ 未开始 | 0% |
| **2.8** | 测试验证 | 1.5天 | ❌ 未开始 | 0% |
| - | 编译测试 | 0.2天 | ❌ 未开始 | 0% |
| - | 基本功能测试 | 0.5天 | ❌ 未开始 | 0% |
| - | 缩容保护测试 | 0.4天 | ❌ 未开始 | 0% |
| - | Apply_pending测试 | 0.4天 | ❌ 未开始 | 0% |
| **总计** | | **10天** | | **20%** |

---

## 下一步行动建议

### 立即开始 (优先级最高)

1. **完成Step 2.3剩余工作** (1.6天)
   - 实现EnsurePodsForPool
   - 实现ScalePods
   - 实现GetPodCount
   - 实现AutoScalePods

### 然后依次进行

2. **Step 2.4**: 更新Auto-scaler (2天)
3. **Step 2.5**: 更新TaskQueueManager (1天)
4. **Step 2.6**: 实现Agent槽位管理 (1天)
5. **Step 2.7**: 更新main.go (0.5天)
6. **Step 2.8**: 全面测试验证 (1.5天)

---

## 关键里程碑

-  **里程碑1**: Pod管理器核心完成 (2025-11-08)
- 🟡 **里程碑2**: K8sDeploymentService重构完成 (预计2025-11-10)
- ⏳ **里程碑3**: 槽位分配集成完成 (预计2025-11-12)
- ⏳ **里程碑4**: 全部功能测试通过 (预计2025-11-14)

---

## 风险和注意事项

1. **向后兼容性**: 保留现有Deployment方法作为备份
2. **渐进式迁移**: 可以先在测试环境验证Pod管理
3. **回滚计划**: 如果出现问题，可以快速回退到Deployment模式
4. **监控和日志**: 确保所有槽位操作都有详细日志
5. **性能影响**: Pod管理可能比Deployment有更多API调用，需要监控性能

---

## 文档更新

需要更新的文档：
-  `docs/terraform-execution-phase2-step-2.3-progress.md` (已创建)
-  `docs/terraform-execution-phase2-remaining-work.md` (本文档)
- ⏳ `docs/terraform-execution-phase2-progress.md` (需要更新总体进度)
- ⏳ API文档 (如果有槽位相关的API)
- ⏳ 运维文档 (Pod管理的监控和故障排查)
