# 槽位数量增加到4个实施方案

## 需求变更

用户反馈：
- 将槽位数量从3个增加到4个
- Plan+Apply任务不需要独占整个Pod，只占用1个槽位即可
- 新的规则：4个槽位中，最多1个plan+apply任务，其余可以是plan任务

## 新的设计规则

### 槽位容量
- **每个Pod有4个槽位** (Slot 0, 1, 2, 3)
- **Plan任务**: 最多4个并发（可以使用任意空闲槽位）
- **Plan+Apply任务**: 最多1个（占用1个槽位，不独占Pod）
- **Reserved槽位**: 不限制数量，不占用执行容量

### 任务分配规则
1. Plan任务可以使用任意空闲槽位
2. Plan+Apply任务只能在Pod上没有其他plan+apply任务时分配
3. 一个Pod可以同时运行：1个plan+apply + 3个plan任务
4. Reserved槽位不阻止其他槽位使用

## 需要修改的文件

### 1. backend/services/k8s_pod_manager.go

#### 修改点1: ManagedPod结构体
```go
// 从 [3]PodSlot 改为 [4]PodSlot
type ManagedPod struct {
    PodName       string      `json:"pod_name"`
    AgentID       string      `json:"agent_id"`
    PoolID        string      `json:"pool_id"`
    Slots         [4]PodSlot  `json:"slots"` // 改为4个槽位
    CreatedAt     time.Time   `json:"created_at"`
    LastHeartbeat time.Time   `json:"last_heartbeat"`
    mu            sync.RWMutex
}
```

#### 修改点2: CreatePod函数 - 初始化4个槽位
```go
// 初始化4个空闲槽位（从3改为4）
for i := 0; i < 4; i++ {
    managedPod.Slots[i] = PodSlot{
        SlotID:    i,
        TaskID:    nil,
        TaskType:  "",
        Status:    "idle",
        UpdatedAt: time.Now(),
    }
}
```

#### 修改点3: FindPodWithFreeSlot函数 - 移除独占逻辑
```go
func (m *K8sPodManager) FindPodWithFreeSlot(poolID string, taskType string) (*ManagedPod, int, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    for _, pod := range m.pods {
        if pod.PoolID != poolID {
            continue
        }

        // Check Agent heartbeat
        timeSinceHeartbeat := time.Since(pod.LastHeartbeat)
        if timeSinceHeartbeat > 2*time.Minute {
            log.Printf("[PodManager] Pod %s is offline (last heartbeat: %v ago), skipping",
                pod.PodName, timeSinceHeartbeat)
            continue
        }

        pod.mu.RLock()
        
        // 如果是plan+apply任务，检查Pod上是否已有其他plan+apply任务
        if taskType == string(models.TaskTypePlanAndApply) {
            hasOtherPlanAndApply := false
            for _, slot := range pod.Slots {
                if (slot.Status == "running" || slot.Status == "reserved") && 
                   slot.TaskType == string(models.TaskTypePlanAndApply) {
                    hasOtherPlanAndApply = true
                    break
                }
            }
            
            if hasOtherPlanAndApply {
                pod.mu.RUnlock()
                log.Printf("[PodManager] Pod %s already has a plan+apply task, cannot accept another",
                    pod.PodName)
                continue
            }
        }
        
        // 查找空闲槽位
        for i, slot := range pod.Slots {
            if slot.Status == "idle" {
                pod.mu.RUnlock()
                return pod, i, nil
            }
        }
        pod.mu.RUnlock()
    }

    return nil, -1, fmt.Errorf("no free slot available in pool %s", poolID)
}
```

#### 修改点4: AssignTaskToSlot函数 - 更新槽位范围检查
```go
if slotID < 0 || slotID > 3 {  // 从 > 2 改为 > 3
    return fmt.Errorf("invalid slot ID: %d", slotID)
}
```

#### 修改点5: ReleaseSlot函数 - 更新槽位范围检查
```go
if slotID < 0 || slotID > 3 {  // 从 > 2 改为 > 3
    return fmt.Errorf("invalid slot ID: %d", slotID)
}
```

#### 修改点6: ReserveSlot函数 - 移除Slot 0限制
```go
func (m *K8sPodManager) ReserveSlot(podName string, slotID int, taskID uint) error {
    // ... 前面的代码保持不变 ...

    if slotID < 0 || slotID > 3 {  // 从 > 2 改为 > 3
        return fmt.Errorf("invalid slot ID: %d", slotID)
    }

    pod.mu.Lock()
    defer pod.mu.Unlock()

    // 移除"只有Slot 0可以被预留"的限制
    // 任何槽位都可以被预留

    // 预留槽位
    pod.Slots[slotID] = PodSlot{
        SlotID:    slotID,
        TaskID:    &taskID,
        TaskType:  string(models.TaskTypePlanAndApply),
        Status:    "reserved",
        UpdatedAt: time.Now(),
    }

    log.Printf("[PodManager] Reserved slot %d on pod %s for task %d (apply_pending)",
        slotID, podName, taskID)

    return nil
}
```

#### 修改点7: SyncPodsFromK8s函数 - 初始化4个槽位
```go
// 初始化槽位（从3改为4）
for i := 0; i < 4; i++ {
    managedPod.Slots[i] = PodSlot{
        SlotID:    i,
        TaskID:    nil,
        TaskType:  "",
        Status:    "idle",
        UpdatedAt: time.Now(),
    }
}
```

#### 修改点8: GetPodSlotStatus函数 - 返回4个槽位
```go
slots := make([]PodSlot, 4)  // 从3改为4
copy(slots, pod.Slots[:])
```

## 实施步骤

1.  创建实施文档
2.  修改 k8s_pod_manager.go
3. ⏳ 测试验证
4. ⏳ 更新相关文档

## 已完成的修改

### backend/services/k8s_pod_manager.go

1.  **ManagedPod结构体**: 将 `Slots [3]PodSlot` 改为 `Slots [4]PodSlot`
2.  **CreatePod函数**: 初始化4个槽位（从3改为4）
3.  **FindPodWithFreeSlot函数**: 
   - 移除了plan+apply任务独占整个Pod的逻辑
   - 改为只检查Pod上是否已有其他plan+apply任务（running或reserved）
   - 简化了槽位查找逻辑
4.  **AssignTaskToSlot函数**: 槽位范围检查从 `> 2` 改为 `> 3`
5.  **ReleaseSlot函数**: 槽位范围检查从 `> 2` 改为 `> 3`
6.  **ReserveSlot函数**: 
   - 槽位范围检查从 `> 2` 改为 `> 3`
   - 移除了"只有Slot 0可以被预留"的限制
7.  **SyncPodsFromK8s函数**: 初始化4个槽位（从3改为4）
8.  **GetPodSlotStatus函数**: 返回4个槽位（从3改为4）

## 预期效果

修改后：
- 每个Pod有4个槽位
- 可以同时运行：1个plan+apply + 3个plan任务
- Plan+Apply任务不再独占整个Pod
- 提高资源利用率
