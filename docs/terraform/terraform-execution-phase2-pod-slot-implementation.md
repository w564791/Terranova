# Phase 2: Pod槽位管理架构实施指南

> **文档版本**: v1.0  
> **创建日期**: 2025-11-08  
> **状态**: 实施中  
> **前置条件**: Phase 1已完成

## 🎯 目标

将K8s agent pool从Deployment管理改为直接Pod管理，实现精确的槽位控制，解决缩容影响测试的问题。

## 📊 当前问题

### Deployment模式的缺陷

**问题**: Deployment缩容时随机删除Pod
```
有3个Pod，每个执行1个任务
缩容到2个Pod时 → K8s随机删除1个Pod
可能删除正在执行任务的Pod ❌
```

**影响**:
- 正在执行的任务被中断
- apply_pending任务的预热环境被销毁
- 测试过程中频繁出现任务失败

### Pod槽位管理的优势

**解决方案**: 直接管理Pod，精确控制槽位
```
每个Pod有3个槽位
缩容时只删除完全空闲的Pod 
有任务的Pod不会被删除 
```

## 🏗️ 架构设计

### 核心概念

#### 1. Pod槽位（Slot）

每个Agent Pod有**3个槽位**：
- **Slot 0**: 可执行plan或plan_and_apply任务
- **Slot 1**: 可执行plan任务
- **Slot 2**: 可执行plan任务

**槽位状态**:
- `idle`: 空闲，可分配任务
- `reserved`: 已预留给apply_pending任务
- `running`: 正在执行任务

#### 2. Pod类型

- **Worker Pod**: 执行实际任务的Pod
- **Reserved Pod**: 专门为apply_pending任务预留的Pod

#### 3. 槽位分配规则

**Plan任务**:
- 优先使用Worker Pod的空闲槽位
- 如果所有Worker Pod满载，创建新Pod

**Plan_and_apply任务**:
- Plan阶段：使用Slot 0
- Apply阶段：继续使用同一个Slot 0（已预热）

**Apply_pending任务**:
- 保持Slot 0为reserved状态
- Pod不会被缩容删除

## 📋 实施步骤

### Step 2.1: 创建Pod管理器（核心）

**文件**: `backend/services/k8s_pod_manager.go` (新建)

**数据结构**:
```go
// PodSlot 槽位
type PodSlot struct {
    SlotID    int        // 0, 1, 2
    TaskID    *uint      // 分配的任务ID
    TaskType  string     // plan, plan_and_apply
    Status    string     // idle, reserved, running
    UpdatedAt time.Time
}

// ManagedPod 管理的Pod
type ManagedPod struct {
    PodName       string
    AgentID       string
    PoolID        string
    Slots         [3]PodSlot // 固定3个槽位
    CreatedAt     time.Time
    LastHeartbeat time.Time
}

// K8sPodManager Pod管理器
type K8sPodManager struct {
    db        *gorm.DB
    clientset *kubernetes.Clientset
    pods      map[string]*ManagedPod // podName -> ManagedPod
    mu        sync.RWMutex
}
```

**核心方法**:
```go
// Pod生命周期
func (m *K8sPodManager) CreatePod(poolID string) (*ManagedPod, error)
func (m *K8sPodManager) DeletePod(podName string) error
func (m *K8sPodManager) ListPods(poolID string) []*ManagedPod

// 槽位管理
func (m *K8sPodManager) FindPodWithFreeSlot(poolID string, taskType string) (*ManagedPod, int, error)
func (m *K8sPodManager) AssignTaskToSlot(podName string, slotID int, taskID uint, taskType string) error
func (m *K8sPodManager) ReleaseSlot(podName string, slotID int) error
func (m *K8sPodManager) ReserveSlot(podName string, slotID int, taskID uint) error

// Pod协调
func (m *K8sPodManager) ReconcilePods(poolID string) error
func (m *K8sPodManager) SyncPodsFromK8s(poolID string) error
```

**工作量**: 3天

### Step 2.2: 实现槽位分配算法

**文件**: `backend/services/k8s_pod_manager.go`

**算法逻辑**:
```go
func (m *K8sPodManager) FindPodWithFreeSlot(poolID string, taskType string) (*ManagedPod, int, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    for _, pod := range m.pods {
        if pod.PoolID != poolID {
            continue
        }
        
        // 检查每个槽位
        for i, slot := range pod.Slots {
            if slot.Status != "idle" {
                continue
            }
            
            // Slot 0: 可以执行任何任务
            if i == 0 {
                return pod, i, nil
            }
            
            // Slot 1, 2: 只能执行plan任务
            if i > 0 && taskType == "plan" {
                return pod, i, nil
            }
        }
    }
    
    return nil, -1, fmt.Errorf("no free slot available")
}
```

**工作量**: 1天

### Step 2.3: 替换Deployment为Pod管理

**文件**: `backend/services/k8s_deployment_service.go`

**重构步骤**:

1. **重命名服务**
   ```go
   // 旧: K8sDeploymentService
   // 新: K8sPodService
   ```

2. **移除Deployment相关代码**
   - 删除 `EnsureDeploymentForPool()`
   - 删除 `ScaleDeployment()`
   - 删除 `buildDeployment()`

3. **添加Pod管理代码**
   ```go
   func (s *K8sPodService) EnsurePodsForPool(poolID string) error {
       // 使用PodManager创建/删除Pod
   }
   
   func (s *K8sPodService) ScalePods(poolID string, desiredCount int) error {
       // 智能缩容：只删除空闲Pod
   }
   ```

**工作量**: 2天

### Step 2.4: 更新Auto-scaler逻辑

**文件**: `backend/services/k8s_deployment_service.go`

**新的扩缩容逻辑**:
```go
func (s *K8sPodService) AutoScalePods(ctx context.Context, pool *models.AgentPool) error {
    // 1. 统计槽位使用情况
    totalSlots, usedSlots, reservedSlots := s.podManager.GetSlotStats(pool.PoolID)
    
    // 2. 计算所需Pod数量
    // 每个Pod 3个槽位
    requiredSlots := usedSlots + reservedSlots
    desiredPods := (requiredSlots + 2) / 3 // 向上取整
    
    // 3. 获取当前Pod数量
    currentPods := len(s.podManager.ListPods(pool.PoolID))
    
    // 4. 扩容或缩容
    if desiredPods > currentPods {
        // 扩容：创建新Pod
        for i := 0; i < desiredPods - currentPods; i++ {
            s.podManager.CreatePod(pool.PoolID)
        }
    } else if desiredPods < currentPods {
        // 缩容：只删除完全空闲的Pod
        idlePods := s.podManager.FindIdlePods(pool.PoolID)
        deleteCount := currentPods - desiredPods
        
        for i := 0; i < deleteCount && i < len(idlePods); i++ {
            s.podManager.DeletePod(idlePods[i].PodName)
        }
    }
}
```

**工作量**: 2天

### Step 2.5: 更新TaskQueueManager

**文件**: `backend/services/task_queue_manager.go`

**修改点**:
```go
func (m *TaskQueueManager) pushTaskToAgent(task *models.WorkspaceTask, workspace *models.Workspace) error {
    // 旧: 查找可用Agent
    // 新: 查找有空闲槽位的Pod
    
    pod, slotID, err := m.k8sPodService.FindPodWithFreeSlot(workspace.CurrentPoolID, task.TaskType)
    if err != nil {
        // 没有空闲槽位，触发扩容
        m.k8sPodService.ScalePods(workspace.CurrentPoolID, currentPods + 1)
        m.scheduleRetry(task.WorkspaceID, 5*time.Second)
        return nil
    }
    
    // 分配槽位
    m.k8sPodService.AssignTaskToSlot(pod.PodName, slotID, task.ID, task.TaskType)
    
    // 发送任务到Agent
    m.agentCCHandler.SendTaskToAgent(pod.AgentID, task.ID, task.WorkspaceID, action)
}
```

**工作量**: 1天

### Step 2.6: Agent端槽位管理

**文件**: `backend/agent/worker/slot_manager.go` (新建)

**功能**:
```go
type SlotManager struct {
    slots [3]*TaskSlot
    mu    sync.RWMutex
}

func (s *SlotManager) AcquireSlot(slotID int, taskID uint) error
func (s *SlotManager) ReleaseSlot(slotID int) error
func (s *SlotManager) GetSlotStatus() []SlotStatus
```

**工作量**: 1天

### Step 2.7: 更新main.go

**文件**: `backend/main.go`

**修改点**:
```go
// 旧: k8sDeploymentService
// 新: k8sPodService

k8sPodService, err := services.NewK8sPodService(db)
if err != nil {
    log.Printf("Warning: Failed to initialize K8s Pod service: %v", err)
} else {
    // 初始化所有K8s pools的Pods
    go k8sPodService.EnsurePodsForAllPools(ctx)
    
    // 启动auto-scaler
    go k8sPodService.StartAutoScaler(ctx, 5*time.Second)
}
```

**工作量**: 0.5天

### Step 2.8: 测试和验证

**测试场景**:

1. **基本功能测试**
   - 创建K8s pool
   - 提交plan任务
   - 验证Pod创建和槽位分配
   - 验证任务执行成功

2. **并发测试**
   - 提交3个plan任务到同一个Pod
   - 验证槽位正确分配
   - 验证任务并发执行

3. **缩容测试**
   - 有3个Pod，每个有1个任务
   - 完成1个任务后
   - 验证只删除空闲Pod
   - 验证有任务的Pod不被删除 

4. **Apply_pending保护测试**
   - 提交plan_and_apply任务
   - Plan完成后进入apply_pending
   - 验证Slot 0被reserved
   - 验证Pod不被缩容删除 

**工作量**: 1.5天

## 📊 实施时间表

| 步骤 | 内容 | 工作量 | 依赖 |
|------|------|--------|------|
| 2.1 | 创建Pod管理器 | 3天 | - |
| 2.2 | 槽位分配算法 | 1天 | 2.1 |
| 2.3 | 替换Deployment | 2天 | 2.1, 2.2 |
| 2.4 | Auto-scaler更新 | 2天 | 2.3 |
| 2.5 | TaskQueueManager更新 | 1天 | 2.3 |
| 2.6 | Agent端槽位管理 | 1天 | 2.1 |
| 2.7 | main.go更新 | 0.5天 | 2.3 |
| 2.8 | 测试验证 | 1.5天 | 全部 |

**总计**: 12天

## 🚨 风险评估

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| Pod管理复杂度高 | 高 | 高 | 充分测试，分步实施 |
| 槽位状态不一致 | 高 | 中 | 实现状态同步机制 |
| 现有功能受影响 | 高 | 中 | 保留Deployment作为fallback |
| 性能问题 | 中 | 低 | 使用内存缓存 |

## 📝 实施建议

### 分步实施策略

**Week 1: 核心框架**
- Day 1-3: 实施Step 2.1（Pod管理器）
- Day 4: 实施Step 2.2（槽位算法）
- Day 5: 基础测试

**Week 2: 集成和替换**
- Day 1-2: 实施Step 2.3（替换Deployment）
- Day 3-4: 实施Step 2.4（Auto-scaler）
- Day 5: 实施Step 2.5（TaskQueueManager）

**Week 3: Agent端和测试**
- Day 1: 实施Step 2.6（Agent槽位管理）
- Day 2: 实施Step 2.7（main.go更新）
- Day 3-5: 实施Step 2.8（全面测试）

### 回滚计划

如果Phase 2实施遇到严重问题，可以回滚到Deployment模式：

1. 保留原有的 `k8s_deployment_service.go`（重命名为 `k8s_deployment_service.go.backup`）
2. 如果Pod管理出现问题，恢复Deployment模式
3. Phase 1的优化（保持工作目录）不受影响

## 🎯 下一步行动

开始实施Step 2.1 - 创建Pod管理器核心框架。
