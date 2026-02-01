# Phase 2 Pod槽位管理 - 继续实施指南

> **文档版本**: v1.0  
> **创建日期**: 2025-11-08  
> **当前状态**: Step 2.1-2.2已完成，准备继续Step 2.3

## 📊 当前进度总结

###  已完成工作（25%）

**Phase 1**: 100%完成 
- Apply启动时间减少85-96%
- 工作目录保持和清理机制
- 6个文件已修改

**Phase 2 Step 2.1-2.2**: 已完成 
- Pod管理器核心框架（500+行代码）
- 槽位分配算法
- 文件已备份

### ⏳ 待完成工作（75%，剩余9天）

**Step 2.3**: 替换Deployment为Pod管理（2天）
**Step 2.4**: Auto-scaler更新（2天）
**Step 2.5**: TaskQueueManager更新（1天）
**Step 2.6**: Agent端槽位管理（1天）
**Step 2.7**: main.go更新（0.5天）
**Step 2.8**: 测试验证（1.5天）

---

## 🎯 Step 2.3 实施详情

### 目标

重构`k8s_deployment_service.go`，从Deployment管理改为Pod管理。

### 核心修改

#### 1. 添加PodManager到K8sDeploymentService

```go
type K8sDeploymentService struct {
    db                    *gorm.DB
    clientset             *kubernetes.Clientset
    podManager            *K8sPodManager  // 【新增】Pod管理器
    freezeScheduleService *FreezeScheduleService
    hostIP                string
    poolTokenService      *service.PoolTokenService
    poolIdleTimes         map[string]time.Time
}

func NewK8sDeploymentService(db *gorm.DB) (*K8sDeploymentService, error) {
    // ... 现有代码
    
    // 【新增】初始化PodManager
    podManager := NewK8sPodManager(db, clientset)
    
    return &K8sDeploymentService{
        // ... 现有字段
        podManager: podManager,  // 【新增】
    }, nil
}
```

#### 2. 修改EnsureDeploymentForPool

```go
// 【重命名】EnsureDeploymentForPool → EnsurePodsForPool
func (s *K8sDeploymentService) EnsurePodsForPool(ctx context.Context, pool *models.AgentPool) error {
    // 1. 确保Secret存在（保持不变）
    secretName, err := s.EnsureSecretForPool(ctx, pool)
    if err != nil {
        return fmt.Errorf("failed to ensure secret: %w", err)
    }
    
    // 2. 【新增】从K8s同步Pod状态
    if err := s.podManager.SyncPodsFromK8s(ctx, pool.PoolID); err != nil {
        return fmt.Errorf("failed to sync pods: %w", err)
    }
    
    // 3. 【新增】协调Pod状态
    if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
        return fmt.Errorf("failed to reconcile pods: %w", err)
    }
    
    log.Printf("[K8sPodService] Ensured pods for pool %s", pool.PoolID)
    return nil
}
```

#### 3. 修改ScaleDeployment

```go
// 【重命名】ScaleDeployment → ScalePods
func (s *K8sDeploymentService) ScalePods(ctx context.Context, poolID string, desiredCount int) error {
    currentCount := s.podManager.GetPodCount(poolID)
    
    if desiredCount > currentCount {
        // 扩容：创建新Pod
        var pool models.AgentPool
        if err := s.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
            return fmt.Errorf("failed to get pool: %w", err)
        }
        
        var k8sConfig models.K8sJobTemplateConfig
        if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
            return fmt.Errorf("failed to parse K8s config: %w", err)
        }
        
        secretName, err := s.EnsureSecretForPool(ctx, &pool)
        if err != nil {
            return fmt.Errorf("failed to ensure secret: %w", err)
        }
        
        for i := 0; i < desiredCount - currentCount; i++ {
            _, err := s.podManager.CreatePod(ctx, poolID, &k8sConfig, secretName)
            if err != nil {
                return fmt.Errorf("failed to create pod: %w", err)
            }
        }
        
        log.Printf("[K8sPodService] Scaled up pool %s from %d to %d pods", poolID, currentCount, desiredCount)
    } else if desiredCount < currentCount {
        // 缩容：只删除完全空闲的Pod
        idlePods := s.podManager.FindIdlePods(poolID)
        deleteCount := currentCount - desiredCount
        
        if len(idlePods) < deleteCount {
            log.Printf("[K8sPodService] Cannot scale down pool %s: need to delete %d pods but only %d are idle",
                poolID, deleteCount, len(idlePods))
            deleteCount = len(idlePods)
        }
        
        for i := 0; i < deleteCount; i++ {
            if err := s.podManager.DeletePod(ctx, idlePods[i].PodName); err != nil {
                log.Printf("[K8sPodService] Failed to delete pod %s: %v", idlePods[i].PodName, err)
            }
        }
        
        log.Printf("[K8sPodService] Scaled down pool %s from %d to %d pods (deleted %d idle pods)",
            poolID, currentCount, currentCount - deleteCount, deleteCount)
    }
    
    return nil
}
```

#### 4. 修改GetDeploymentReplicas

```go
// 【重命名】GetDeploymentReplicas → GetPodCount
func (s *K8sDeploymentService) GetPodCount(ctx context.Context, poolID string) (current, desired int, err error) {
    // 【新增】从PodManager获取
    current = s.podManager.GetPodCount(poolID)
    
    // 【新增】计算期望数量（基于槽位统计）
    total, used, reserved, _ := s.podManager.GetSlotStats(poolID)
    requiredSlots := used + reserved
    desired = (requiredSlots + 2) / 3 // 向上取整
    
    return current, desired, nil
}
```

#### 5. 修改AutoScaleDeployment

```go
// 【重命名】AutoScaleDeployment → AutoScalePods
func (s *K8sDeploymentService) AutoScalePods(ctx context.Context, pool *models.AgentPool) (int, bool, error) {
    // 1. 协调Pod状态
    if err := s.podManager.ReconcilePods(ctx, pool.PoolID); err != nil {
        log.Printf("[K8sPodService] Failed to reconcile pods: %v", err)
    }
    
    // 2. 获取槽位统计
    total, used, reserved, idle := s.podManager.GetSlotStats(pool.PoolID)
    
    log.Printf("[K8sPodService] Pool %s slot stats: total=%d, used=%d, reserved=%d, idle=%d",
        pool.PoolID, total, used, reserved, idle)
    
    // 3. 计算所需Pod数量
    requiredSlots := used + reserved
    desiredPods := (requiredSlots + 2) / 3 // 向上取整
    
    // 4. 获取当前Pod数量
    currentPods := s.podManager.GetPodCount(pool.PoolID)
    
    // 5. 应用min/max限制
    var k8sConfig models.K8sJobTemplateConfig
    if err := json.Unmarshal([]byte(*pool.K8sConfig), &k8sConfig); err != nil {
        return 0, false, fmt.Errorf("failed to parse K8s config: %w", err)
    }
    
    if desiredPods < k8sConfig.MinReplicas {
        desiredPods = k8sConfig.MinReplicas
    }
    if desiredPods > k8sConfig.MaxReplicas {
        desiredPods = k8sConfig.MaxReplicas
    }
    
    // 6. 执行扩缩容
    if desiredPods != currentPods {
        if err := s.ScalePods(ctx, pool.PoolID, desiredPods); err != nil {
            return 0, false, fmt.Errorf("failed to scale pods: %w", err)
        }
        
        log.Printf("[K8sPodService] Auto-scaled pool %s from %d to %d pods (required slots: %d)",
            pool.PoolID, currentPods, desiredPods, requiredSlots)
        
        return desiredPods, true, nil
    }
    
    return currentPods, false, nil
}
```

---

## 📝 实施检查清单

### Step 2.3 检查清单

- [ ] 添加PodManager字段到K8sDeploymentService
- [ ] 在NewK8sDeploymentService中初始化PodManager
- [ ] 重命名EnsureDeploymentForPool → EnsurePodsForPool
- [ ] 重命名ScaleDeployment → ScalePods
- [ ] 重命名GetDeploymentReplicas → GetPodCount
- [ ] 重命名AutoScaleDeployment → AutoScalePods
- [ ] 更新所有方法实现使用PodManager
- [ ] 移除Deployment相关代码（buildDeployment等）
- [ ] 更新日志前缀：[K8sDeployment] → [K8sPodService]
- [ ] 编译检查
- [ ] 单元测试

---

## 🔄 后续步骤概览

### Step 2.4: Auto-scaler更新（2天）
- 更新runAutoScalerCycle使用新的AutoScalePods
- 更新StartAutoScaler
- 测试自动扩缩容

### Step 2.5: TaskQueueManager更新（1天）
- 修改pushTaskToAgent使用槽位分配
- 任务完成后释放槽位
- Plan完成后预留Slot 0

### Step 2.6: Agent端槽位管理（1天）
- 创建slot_manager.go
- 实现槽位获取和释放
- 上报槽位状态

### Step 2.7: main.go更新（0.5天）
- 更新服务初始化
- 启动Pod协调器

### Step 2.8: 测试验证（1.5天）
- 基本功能测试
- 并发测试
- 缩容保护测试  关键
- Apply_pending保护测试  关键

---

## 📖 相关文件

**已创建**:
- `backend/services/k8s_pod_manager.go` - Pod管理器（500+行）
- `backend/services/k8s_deployment_service.go.backup` - 原文件备份
- `docs/terraform-execution-phase2-pod-slot-implementation.md` - 实施指南
- `docs/terraform-execution-phase2-progress.md` - 进度跟踪

**待修改**:
- `backend/services/k8s_deployment_service.go` - 重构为Pod管理
- `backend/services/task_queue_manager.go` - 使用槽位分配
- `backend/agent/worker/slot_manager.go` - Agent端槽位管理（新建）
- `backend/main.go` - 服务初始化

---

## 🎯 建议

由于Phase 2是大型重构（剩余9天工作量），建议：

1. **创建新任务继续Phase 2**
   - 使用`new_task`工具创建Phase 2专用任务
   - 预加载当前上下文和已完成工作
   - 专注于剩余的Step 2.3-2.8

2. **或者分批实施**
   - 先完成Step 2.3-2.4（4天）
   - 测试验证
   - 再继续Step 2.5-2.8（5天）

3. **保持风险控制**
   - 备份文件已创建
   - 可随时回滚
   - Phase 1优化不受影响

---

**下一步**: 继续实施Step 2.3 - 重构K8sDeploymentService使用PodManager
