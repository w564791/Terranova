# Terraform执行流程优化实施计划

> **文档版本**: v1.0  
> **创建日期**: 2025-11-08  
> **状态**: 实施计划  
> **相关文档**: [terraform-execution-optimization-analysis.md](terraform-execution-optimization-analysis.md)

## 📋 概述

本文档列出Terraform执行流程优化的完整实施清单，包括所有需要开发的功能点、数据库变更、代码修改等。

## 🎯 Phase 1: 保持工作目录优化（优先级P0）

### 1.1 数据库Schema变更

**文件**: `scripts/add_plan_optimization_fields.sql`

```sql
-- 添加plan hash字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_hash VARCHAR(64);

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_plan_hash 
ON workspace_tasks(plan_hash);
```

**工作量**: 0.5天  
**风险**: 低

### 1.2 TerraformExecutor修改

**文件**: `backend/services/terraform_executor.go`

**修改点**:

1. **Plan完成后不清理工作目录**
   ```go
   func (e *TerraformExecutor) ExecutePlan() {
       // ... 执行plan
       
       // 计算并保存plan hash
       planData, _ := os.ReadFile(planFile)
       hash := sha256.Sum256(planData)
       task.PlanHash = hex.EncodeToString(hash[:])
       e.db.Save(task)
       
       // ❌ 删除这行: defer e.CleanupWorkspace(workDir)
       //  不清理工作目录
       log.Printf("[Optimization] Keeping work directory: %s", workDir)
   }
   ```

2. **Apply阶段优化**
   ```go
   func (e *TerraformExecutor) ExecuteApply() {
       workDir := e.getWorkDir(task)
       
       // 检查工作目录是否存在
       if e.workDirExists(workDir) {
           // 验证plan.out的hash
           if e.verifyPlanHash(task, workDir) {
               log.Printf("[Optimization] Using existing work directory (FAST PATH)")
               // 直接执行apply，跳过init
               return e.terraformApplyDirect(workDir)
           }
       }
       
       // Fallback: 正常流程
       return e.executeApplyNormal(task)
   }
   ```

3. **添加hash验证方法**
   ```go
   func (e *TerraformExecutor) verifyPlanHash(task *models.WorkspaceTask, workDir string) bool {
       planFile := filepath.Join(workDir, "plan.out")
       planData, err := os.ReadFile(planFile)
       if err != nil {
           return false
       }
       
       hash := sha256.Sum256(planData)
       currentHash := hex.EncodeToString(hash[:])
       
       return currentHash == task.PlanHash
   }
   ```

4. **添加直接apply方法**
   ```go
   func (e *TerraformExecutor) terraformApplyDirect(workDir string) error {
       cmd := exec.Command("terraform", "apply", "-no-color", "-auto-approve", "plan.out")
       cmd.Dir = workDir
       // ... 执行
   }
   ```

**工作量**: 1天  
**风险**: 低

### 1.3 工作目录清理机制

**文件**: `backend/services/task_queue_manager.go`

**新增功能**:

```go
// 定期清理过期的工作目录
func (m *TaskQueueManager) CleanupExpiredWorkDirs() {
    baseDir := "/tmp/iac-platform/workspaces"
    
    // 1. 清理已完成任务的工作目录（超过1小时）
    var completedTasks []models.WorkspaceTask
    m.db.Where("status IN (?)", []string{"success", "applied", "failed", "cancelled"}).
        Where("completed_at < ?", time.Now().Add(-1*time.Hour)).
        Find(&completedTasks)
    
    for _, task := range completedTasks {
        workDir := filepath.Join(baseDir, task.WorkspaceID, fmt.Sprintf("%d", task.ID))
        os.RemoveAll(workDir)
    }
    
    // 2. 清理apply_pending任务的工作目录（超过24小时）
    var pendingTasks []models.WorkspaceTask
    m.db.Where("status = ?", models.TaskStatusApplyPending).
        Where("updated_at < ?", time.Now().Add(-24*time.Hour)).
        Find(&pendingTasks)
    
    for _, task := range pendingTasks {
        workDir := filepath.Join(baseDir, task.WorkspaceID, fmt.Sprintf("%d", task.ID))
        os.RemoveAll(workDir)
    }
}

// 启动定期清理
func (m *TaskQueueManager) StartWorkDirCleaner(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.CleanupExpiredWorkDirs()
        }
    }
}
```

**工作量**: 0.5天  
**风险**: 低

### 1.4 main.go启动清理器

**文件**: `backend/main.go`

```go
// 启动工作目录清理器
go taskQueueManager.StartWorkDirCleaner(ctx)
```

**工作量**: 0.1天  
**风险**: 低

**Phase 1 总工作量**: 2天  
**Phase 1 总风险**: 低

---

## 🎯 Phase 2: Agent预热机制（优先级P1）

### 2.1 数据库Schema变更

**文件**: `scripts/add_warmup_fields.sql`

```sql
-- 添加预热相关字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_agent_id VARCHAR(50);
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_status VARCHAR(20) DEFAULT 'none';
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_at TIMESTAMP;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_expires_at TIMESTAMP;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_retry_count INTEGER DEFAULT 0;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_warmup_agent 
ON workspace_tasks(warmup_agent_id, warmup_status);

CREATE INDEX IF NOT EXISTS idx_workspace_tasks_apply_pending_warmup
ON workspace_tasks(status, warmup_status, warmup_expires_at) 
WHERE status = 'apply_pending';
```

**工作量**: 0.5天  
**风险**: 低

### 2.2 模型定义更新

**文件**: `backend/internal/models/workspace.go`

```go
type WorkspaceTask struct {
    // ... 现有字段
    
    // 预热相关字段
    PlanHash         string     `json:"plan_hash" gorm:"type:varchar(64)"`
    WarmupAgentID    *string    `json:"warmup_agent_id" gorm:"type:varchar(50)"`
    WarmupStatus     string     `json:"warmup_status" gorm:"type:varchar(20);default:none"`
    WarmupAt         *time.Time `json:"warmup_at"`
    WarmupExpiresAt  *time.Time `json:"warmup_expires_at"`
    WarmupRetryCount int        `json:"warmup_retry_count" gorm:"default:0"`
}
```

**工作量**: 0.2天  
**风险**: 低

### 2.3 Agent预热逻辑

**文件**: `backend/agent/worker/warmup.go` (新建)

```go
// Agent启动时检查预热任务
func (a *Agent) OnStart() error
func (a *Agent) warmupTask(task *models.WorkspaceTask) error
func (a *Agent) handleWarmupError(task *models.WorkspaceTask, err error) error
```

**工作量**: 2天  
**风险**: 中等

### 2.4 Apply执行逻辑优化

**文件**: `backend/agent/worker/executor.go`

```go
func (a *Agent) ExecuteApply(task *models.WorkspaceTask) error {
    // 场景1: 同一个Agent，验证本地hash
    if task.WarmupAgentID == a.agentID {
        if verifyLocalPlan() {
            return terraformApplyDirect() // 最快路径
        }
    }
    
    // 场景2: 预热完成
    if task.WarmupStatus == "ready" {
        return terraformApplyDirect()
    }
    
    // 场景3: Fallback
    return executeApplyNormal()
}
```

**工作量**: 1天  
**风险**: 中等

### 2.5 Agent离线检测

**文件**: `backend/services/task_queue_manager.go`

```go
func (m *TaskQueueManager) MonitorAgentHealth(ctx context.Context)
func (m *TaskQueueManager) checkOfflineAgents()
```

**工作量**: 1天  
**风险**: 低

### 2.6 Plan完成后触发预热

**文件**: `backend/services/terraform_executor.go`

```go
func (e *TerraformExecutor) OnPlanComplete(task *models.WorkspaceTask, workDir string) error {
    // 计算hash
    // 记录warmup_agent_id
    // 触发预热（如果是Agent模式）
}
```

**工作量**: 0.5天  
**风险**: 低

**Phase 2 总工作量**: 5天  
**Phase 2 总风险**: 中等

---

## 🎯 Phase 3: Pod槽位管理架构（优先级P2）

### 3.1 Pod管理器重构

**文件**: `backend/services/k8s_pod_manager.go` (新建)

**核心功能**:

1. **Pod槽位数据结构**
   ```go
   type PodSlot struct {
       SlotID    int
       TaskID    *uint
       TaskType  string
       Status    string // idle/running/reserved
       UpdatedAt time.Time
   }
   
   type ManagedPod struct {
       PodName       string
       AgentID       string
       PoolID        string
       Slots         []PodSlot // 3个槽位
       CreatedAt     time.Time
       LastHeartbeat time.Time
   }
   ```

2. **Pod协调逻辑**
   ```go
   func (s *K8sPodManager) ReconcilePods(poolID string) error
   func (s *K8sPodManager) reconcileWorkerPods() error
   func (s *K8sPodManager) reconcileReservedPods() error
   ```

3. **槽位分配**
   ```go
   func (s *K8sPodManager) findPodWithFreeSlot() *ManagedPod
   func (s *K8sPodManager) assignTaskToSlot() error
   func (s *K8sPodManager) releaseSlot() error
   ```

4. **Pod生命周期管理**
   ```go
   func (s *K8sPodManager) createPod() error
   func (s *K8sPodManager) deletePod() error
   func (s *K8sPodManager) listPods() []ManagedPod
   func (s *K8sPodManager) findIdlePods() []ManagedPod
   ```

**工作量**: 5天  
**风险**: 高

### 3.2 替换Deployment为直接Pod管理

**文件**: `backend/services/k8s_deployment_service.go`

**修改点**:
- 移除Deployment相关代码
- 改为直接创建/删除Pod
- 使用PodManager管理Pod生命周期

**工作量**: 3天  
**风险**: 高

### 3.3 Auto-scaler逻辑更新

**文件**: `backend/services/k8s_deployment_service.go`

```go
func (s *K8sDeploymentService) CalculateDesiredPods(poolID string) int {
    // 统计running任务
    runningCount := countRunningTasks(poolID)
    
    // 统计apply_pending任务（已预热）
    applyPendingCount := countApplyPendingTasks(poolID)
    
    // 计算总槽位数
    totalSlots := runningCount + applyPendingCount
    
    // 计算Pod数量（每个Pod 3个槽位）
    desiredPods := (totalSlots + 2) / 3
    
    return desiredPods
}
```

**工作量**: 2天  
**风险**: 中等

### 3.4 Freeze Schedule集成

**文件**: `backend/services/freeze_schedule_service.go`

```go
func (s *FreezeScheduleService) EnterFreezeWindow(poolID string) error {
    // 1. 标记Pool为frozen
    // 2. 强制删除所有Pod
    // 3. 重置所有预热状态
}

func (s *FreezeScheduleService) ExitFreezeWindow(poolID string) error {
    // 1. 标记Pool为unfrozen
    // 2. 触发Pod重建
    // 3. 自动预热apply_pending任务
}
```

**工作量**: 1天  
**风险**: 中等

**Phase 3 总工作量**: 11天  
**Phase 3 总风险**: 高

---

## 📝 完整优化项目清单

### Phase 1: 保持工作目录（2天，低风险）

- [ ] 1.1 数据库Schema变更
  - [ ] 创建 `scripts/add_plan_optimization_fields.sql`
  - [ ] 添加 `plan_hash` 字段
  - [ ] 添加索引
  - [ ] 执行SQL脚本

- [ ] 1.2 模型定义更新
  - [ ] 在 `WorkspaceTask` 中添加 `PlanHash` 字段
  - [ ] 更新模型注释

- [ ] 1.3 TerraformExecutor修改
  - [ ] Plan完成后计算并保存hash
  - [ ] 移除Plan阶段的工作目录清理
  - [ ] Apply阶段添加工作目录存在性检查
  - [ ] Apply阶段添加hash验证
  - [ ] 实现 `verifyPlanHash()` 方法
  - [ ] 实现 `terraformApplyDirect()` 方法
  - [ ] Apply完成后清理工作目录

- [ ] 1.4 工作目录清理机制
  - [ ] 实现 `CleanupExpiredWorkDirs()` 方法
  - [ ] 实现 `StartWorkDirCleaner()` 方法
  - [ ] 在main.go中启动清理器

- [ ] 1.5 测试
  - [ ] 单元测试：hash计算和验证
  - [ ] 集成测试：Plan到Apply完整流程
  - [ ] 性能测试：对比优化前后的耗时

### Phase 2: Agent预热机制（5天，中等风险）

- [ ] 2.1 数据库Schema变更
  - [ ] 创建 `scripts/add_warmup_fields.sql`
  - [ ] 添加预热相关字段（warmup_agent_id, warmup_status等）
  - [ ] 添加索引
  - [ ] 执行SQL脚本

- [ ] 2.2 模型定义更新
  - [ ] 在 `WorkspaceTask` 中添加预热字段
  - [ ] 定义 `WarmupStatus` 枚举

- [ ] 2.3 Agent预热逻辑
  - [ ] 创建 `backend/agent/worker/warmup.go`
  - [ ] 实现 `OnStart()` - Agent启动时检查预热任务
  - [ ] 实现 `warmupTask()` - 执行预热流程
  - [ ] 实现 `handleWarmupError()` - 处理预热错误
  - [ ] 添加重试计数逻辑

- [ ] 2.4 Plan完成后触发预热
  - [ ] 修改 `TerraformExecutor.OnPlanComplete()`
  - [ ] 记录 `warmup_agent_id`
  - [ ] 触发Agent预热（如果是Agent/K8s模式）

- [ ] 2.5 Apply执行逻辑优化
  - [ ] 修改 `Agent.ExecuteApply()`
  - [ ] 场景1：同一Agent，验证本地hash
  - [ ] 场景2：预热完成，直接执行
  - [ ] 场景3：Fallback到正常流程
  - [ ] 添加预热过期检查

- [ ] 2.6 Agent离线检测
  - [ ] 实现 `MonitorAgentHealth()`
  - [ ] 实现 `checkOfflineAgents()`
  - [ ] Agent离线时重置预热状态
  - [ ] 添加重试计数保护

- [ ] 2.7 Agent注册时触发预热
  - [ ] 修改 `AgentHandler.RegisterAgent()`
  - [ ] 注册成功后延迟触发 `OnStart()`

- [ ] 2.8 测试
  - [ ] 单元测试：预热逻辑
  - [ ] 集成测试：Agent销毁重建场景
  - [ ] 集成测试：预热过期场景
  - [ ] 性能测试：用户确认后的响应时间

### Phase 3: Pod槽位管理架构（11天，高风险）

- [ ] 3.1 Pod管理器设计
  - [ ] 创建 `backend/services/k8s_pod_manager.go`
  - [ ] 定义 `PodSlot` 数据结构
  - [ ] 定义 `ManagedPod` 数据结构
  - [ ] 设计Pod状态管理机制

- [ ] 3.2 Pod协调逻辑
  - [ ] 实现 `ReconcilePods()` - 主协调逻辑
  - [ ] 实现 `reconcileWorkerPods()` - Worker Pod管理
  - [ ] 实现 `reconcileReservedPods()` - Reserved Pod管理
  - [ ] 实现槽位分配算法

- [ ] 3.3 Pod生命周期管理
  - [ ] 实现 `createPod()` - 创建Pod
  - [ ] 实现 `deletePod()` - 删除Pod
  - [ ] 实现 `listPods()` - 列出Pod
  - [ ] 实现 `findIdlePods()` - 查找空闲Pod
  - [ ] 实现 `findPodWithFreeSlot()` - 查找有空闲槽位的Pod

- [ ] 3.4 槽位管理
  - [ ] 实现 `assignTaskToSlot()` - 分配任务到槽位
  - [ ] 实现 `releaseSlot()` - 释放槽位
  - [ ] 实现 `getSlotStatus()` - 获取槽位状态
  - [ ] 实现槽位状态同步机制

- [ ] 3.5 替换Deployment
  - [ ] 移除 `k8s_deployment_service.go` 中的Deployment代码
  - [ ] 改为使用PodManager
  - [ ] 更新所有调用点

- [ ] 3.6 Auto-scaler更新
  - [ ] 修改 `CalculateDesiredReplicas()` 为 `CalculateDesiredPods()`
  - [ ] 更新容量计算逻辑（考虑槽位）
  - [ ] 更新缩容逻辑（检查槽位状态）

- [ ] 3.7 Freeze Schedule集成
  - [ ] 修改 `EnterFreezeWindow()` - 删除所有Pod
  - [ ] 修改 `ExitFreezeWindow()` - 触发Pod重建
  - [ ] 重置预热状态

- [ ] 3.8 测试
  - [ ] 单元测试：槽位分配算法
  - [ ] 单元测试：Pod协调逻辑
  - [ ] 集成测试：多任务并发场景
  - [ ] 集成测试：缩容场景
  - [ ] 集成测试：Freeze Schedule场景
  - [ ] 压力测试：大量任务场景

---

## 📊 实施时间表

| Phase | 功能 | 工作量 | 风险 | 开始时间 | 完成时间 |
|-------|------|--------|------|---------|---------|
| Phase 1 | 保持工作目录 | 2天 | 低 | Week 1 | Week 1 |
| Phase 2 | Agent预热 | 5天 | 中 | Week 2 | Week 2 |
| Phase 3 | Pod槽位管理 | 11天 | 高 | Week 3-4 | Week 4 |

**总工作量**: 18天（约3.5周）

---

## 🚨 风险评估

### Phase 1 风险

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| 工作目录丢失 | 中 | 低 | Fallback到正常流程 |
| 磁盘空间不足 | 中 | 低 | 定期清理机制 |
| Hash冲突 | 低 | 极低 | 使用SHA256 |

### Phase 2 风险

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| Agent销毁导致预热丢失 | 中 | 中 | 自动重新预热 |
| 预热失败死循环 | 高 | 中 | 重试计数限制 |
| 预热过期 | 低 | 中 | 过期检查和重新预热 |

### Phase 3 风险

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| Pod管理复杂度高 | 高 | 高 | 充分测试 |
| 槽位状态不一致 | 高 | 中 | 状态同步机制 |
| Freeze Schedule冲突 | 中 | 低 | 强制清理所有Pod |

---

## 📋 依赖关系

```
Phase 1 (保持工作目录)
    ↓ 可以独立实施
Phase 2 (Agent预热)
    ↓ 依赖Phase 1
Phase 3 (Pod槽位管理)
    ↓ 依赖Phase 2
```

**说明**:
- Phase 1可以独立实施，立即见效
- Phase 2依赖Phase 1的工作目录保持机制
- Phase 3依赖Phase 2的预热机制

---

##  验收标准

### Phase 1验收

- [ ] Apply启动时间减少85%以上
- [ ] Hash验证100%准确
- [ ] 工作目录清理机制正常运行
- [ ] 无磁盘空间泄漏

### Phase 2验收

- [ ] 用户确认后1秒内开始Apply
- [ ] Agent销毁重建场景正常工作
- [ ] 预热失败不影响正常执行
- [ ] 无死循环问题

### Phase 3验收

- [ ] Pod槽位管理正常运行
- [ ] apply_pending任务不被错误缩容
- [ ] Freeze Schedule正常工作
- [ ] 多任务并发场景正常

---

## 📖 相关文档

- [terraform-execution-optimization-analysis.md](terraform-execution-optimization-analysis.md) - 优化方案分析
- [terraform-execution-states-and-sequential-guarantee.md](terraform-execution-states-and-sequential-guarantee.md) - 执行流程状态
- [15-terraform-execution-detail.md](workspace/15-terraform-execution-detail.md) - 执行流程设计

---

**总结**: 完整优化需要3个Phase，共18天工作量，建议按Phase顺序逐步实施。
