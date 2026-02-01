# Terraform执行流程优化分析

> **文档版本**: v1.0  
> **创建日期**: 2025-11-08  
> **状态**: 优化建议分析  
> **相关文档**: [terraform-execution-states-and-sequential-guarantee.md](terraform-execution-states-and-sequential-guarantee.md)

## 📋 概述

本文档分析Terraform执行流程中的两个关键优化点，评估其合理性、可行性和实施方案。

## 🎯 优化点1: Plan到Apply的工作目录保持

### 当前实现问题

```go
// 当前流程
Plan阶段:
1. 创建工作目录 /tmp/iac-platform/workspaces/{ws_id}/{task_id}/
2. 生成配置文件（main.tf.json, provider.tf.json等）
3. 拉取State文件
4. terraform init
5. terraform plan -out=plan.out
6. 保存plan.out到数据库
7. 清理工作目录 ❌

Apply阶段（用户确认后）:
1. 重新创建工作目录
2. 重新生成配置文件 ❌ 重复
3. 重新拉取State文件 ❌ 重复
4. terraform init ❌ 重复
5. 从数据库恢复plan.out
6. terraform apply plan.out
7. 清理工作目录
```

**问题**:
- 重复执行init（下载Provider插件，耗时）
- 重复生成配置文件
- 重复拉取State文件
- 增加了Apply阶段的启动时间

### 优化方案

#### 方案A: 保持工作目录（推荐）

```go
Plan阶段:
1. 创建工作目录 /tmp/iac-platform/workspaces/{ws_id}/{task_id}/
2. 生成配置文件
3. 拉取State文件
4. terraform init
5. terraform plan -out=plan.out
6. 保存plan.out到数据库
7. 计算工作目录hash（可选，用于验证）
8. 保持工作目录不清理 

Apply阶段:
1. 验证工作目录是否存在
2. 验证plan.out文件hash（可选）
3. 直接执行 terraform apply plan.out 
4. 清理工作目录
```

**优点**:
-  节省init时间（通常5-30秒）
-  节省配置文件生成时间
-  节省State文件拉取时间
-  减少网络IO
-  减少磁盘IO
-  Apply启动更快

**缺点**:
- ❌ 占用磁盘空间（直到Apply完成）
- ❌ 需要处理工作目录丢失的情况
- ❌ 需要处理服务器重启的情况

#### 方案B: 使用Plan文件hash验证（折中方案）

```go
Plan阶段:
1. 执行Plan
2. 保存plan.out到数据库
3. 计算plan.out的hash
4. 保存hash到task.plan_hash字段
5. 清理工作目录

Apply阶段:
1. 创建工作目录
2. 从数据库恢复plan.out
3. 验证plan.out的hash 
4. 如果hash匹配，跳过init，直接apply 
5. 如果hash不匹配，重新init后apply
```

**优点**:
-  提供了数据完整性验证
-  不占用长期磁盘空间
-  可以检测plan文件损坏

**缺点**:
- ❌ 仍然需要重新生成配置文件
- ❌ 仍然需要重新拉取State
- ❌ 仍然需要init（虽然可以优化）

### 合理性评估

| 维度 | 方案A（保持目录） | 方案B（hash验证） | 评分 |
|------|------------------|------------------|------|
| 性能提升 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | A更优 |
| 实现复杂度 | ⭐⭐⭐ | ⭐⭐⭐⭐ | 相当 |
| 可靠性 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | B更优 |
| 资源占用 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | B更优 |

#### 方案C: 混合方案 - 保持目录 + Hash验证 + Agent感知（最优方案）

```go
Plan阶段:
1. 创建工作目录 /tmp/iac-platform/workspaces/{ws_id}/{task_id}/
2. 生成配置文件
3. 拉取State文件
4. terraform init
5. terraform plan -out=plan.out
6. 计算plan.out的hash并保存到task.plan_hash 
7. 保存plan.out到数据库（作为备份）
8. 保持工作目录不清理 
9. 记录执行Agent ID到task.warmup_agent_id 

Apply阶段（Agent未销毁）:
1. 检查当前Agent ID是否等于warmup_agent_id 
2. 如果相同，验证本地plan.out的hash 
3. Hash匹配 → 直接执行apply（最快路径）
4. Hash不匹配 → 从数据库恢复plan.out
5. 执行terraform apply

Apply阶段（Agent已销毁/重建）:
1. 新Agent启动时，检查是否有apply_pending任务 
2. 如果有，自动执行预热流程: 
   - 创建工作目录
   - 生成配置文件
   - 拉取State文件
   - terraform init
   - 从数据库恢复plan.out
   - 验证hash
   - 标记为ready
3. 等待用户confirm apply
4. 用户确认后立即执行apply
```

**优点**:
-  Agent未销毁时性能最优（直接apply，无需init）
-  Agent销毁后自动预热，用户无感知
-  Hash验证保证数据完整性
-  数据库备份提供容灾能力
-  用户体验最佳

**缺点**:
-  实现复杂度较高
-  需要Agent感知机制

**推荐**: **方案C（混合方案）** ⭐⭐⭐⭐⭐

**理由**:
1. 结合了方案A和方案B的所有优点
2. 处理了Agent销毁重建的场景
3. 性能和可靠性都达到最优
4. 用户体验最好

## 🎯 优化点2: Agent预热机制

### 当前实现问题

```go
// 当前流程
Plan完成 → apply_pending状态 → 等待用户确认（可能很久）
                                    ↓
用户确认 → Agent接收任务 → 创建工作目录 → 生成配置 → 拉取State → init → apply
                          ↑
                          这些步骤耗时较长（10-60秒）
```

**问题**:
- 用户确认后需要等待较长时间才能看到Apply开始
- 用户体验不好
- 在Agent重启/销毁重建的场景下，问题更明显

### 优化方案

#### 方案A: Agent任务预热（推荐）

```go
// 优化后流程
Plan完成 → apply_pending状态 → 立即推送"预热任务"到Agent
                                    ↓
                              Agent预热:
                              1. 创建工作目录
                              2. 生成配置文件
                              3. 拉取State文件
                              4. terraform init
                              5. 从数据库恢复plan.out
                              6. 标记为"ready"状态
                              7. 等待用户确认
                                    ↓
用户确认 → Agent立即执行 terraform apply plan.out 
          （几乎无延迟）
```

**实现细节**:

```go
// 1. 新增任务预热状态
type TaskWarmupStatus string

const (
    WarmupStatusNone       TaskWarmupStatus = "none"        // 未预热
    WarmupStatusWarming    TaskWarmupStatus = "warming"     // 预热中
    WarmupStatusReady      TaskWarmupStatus = "ready"       // 预热完成
    WarmupStatusExpired    TaskWarmupStatus = "expired"     // 预热过期
)

// 2. 在WorkspaceTask中添加字段
type WorkspaceTask struct {
    // ... 现有字段
    WarmupStatus    TaskWarmupStatus `json:"warmup_status" gorm:"default:none"`
    WarmupAgentID   *string          `json:"warmup_agent_id"`
    WarmupAt        *time.Time       `json:"warmup_at"`
    WarmupExpiresAt *time.Time       `json:"warmup_expires_at"` // 预热过期时间
}

// 3. Plan完成后触发预热
func (e *TerraformExecutor) OnPlanComplete(task *models.WorkspaceTask) error {
    if task.TaskType == models.TaskTypePlanAndApply {
        // 推送预热任务到Agent
        return e.warmupTaskOnAgent(task)
    }
    return nil
}

// 4. Agent预热逻辑
func (a *Agent) WarmupTask(taskID uint) error {
    // 1. 创建工作目录
    workDir := a.createWorkDir(taskID)
    
    // 2. 生成配置文件
    if err := a.generateConfigFiles(taskID, workDir); err != nil {
        return err
    }
    
    // 3. 拉取State
    if err := a.fetchState(taskID, workDir); err != nil {
        return err
    }
    
    // 4. Terraform init
    if err := a.terraformInit(workDir); err != nil {
        return err
    }
    
    // 5. 恢复plan.out
    if err := a.restorePlanFile(taskID, workDir); err != nil {
        return err
    }
    
    // 6. 标记为ready
    return a.updateTaskWarmupStatus(taskID, WarmupStatusReady)
}

// 5. 用户确认后直接执行
func (a *Agent) ExecuteApply(taskID uint) error {
    task := a.getTask(taskID)
    
    if task.WarmupStatus == WarmupStatusReady {
        // 工作目录已准备好，直接执行
        return a.terraformApply(task.WorkDir)
    } else {
        // 预热失败或过期，走正常流程
        return a.executeApplyNormal(taskID)
    }
}
```

**优点**:
-  用户确认后几乎立即开始Apply
-  用户体验大幅提升
-  充分利用等待时间
-  对Agent重启场景友好

**缺点**:
- ❌ 增加了系统复杂度
- ❌ 需要处理预热过期的情况
- ❌ 需要处理Agent切换的情况
- ❌ 占用Agent资源（但可接受）

#### 方案B: 延迟预热（保守方案）

```go
// 只在用户即将确认时预热
用户打开"Confirm Apply"对话框 → 触发预热
                                    ↓
                              后台预热（5-10秒）
                                    ↓
用户点击确认 → 如果预热完成，立即执行
              如果预热未完成，等待预热完成
```

**优点**:
-  减少不必要的预热
-  实现相对简单

**缺点**:
- ❌ 仍然需要等待
- ❌ 用户体验提升有限

### 合理性评估

| 维度 | 方案A（立即预热） | 方案B（延迟预热） | 评分 |
|------|------------------|------------------|------|
| 用户体验 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | A更优 |
| 实现复杂度 | ⭐⭐⭐ | ⭐⭐⭐⭐ | B更简单 |
| 资源利用 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | B更优 |
| 可靠性 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | B更优 |

**推荐**: **方案A（立即预热）**

**理由**:
1. 用户体验提升最明显
2. 充分利用等待时间
3. 实现复杂度可接受
4. 资源占用可控

## 🔄 方案C详细实现

### 数据库Schema变更

```sql
-- 添加预热相关字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_hash VARCHAR(64);
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_agent_id VARCHAR(50);
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_status VARCHAR(20) DEFAULT 'none';
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_at TIMESTAMP;
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_expires_at TIMESTAMP;

-- 添加索引
CREATE INDEX IF NOT EXISTS idx_workspace_tasks_warmup_agent 
ON workspace_tasks(warmup_agent_id, warmup_status);

CREATE INDEX IF NOT EXISTS idx_workspace_tasks_apply_pending_pool
ON workspace_tasks(status, warmup_status) 
WHERE status = 'apply_pending';
```

### 核心实现代码

```go
// 1. Plan完成后的处理
func (e *TerraformExecutor) OnPlanComplete(task *models.WorkspaceTask, workDir string) error {
    // 计算plan.out的hash
    planFile := filepath.Join(workDir, "plan.out")
    planData, err := os.ReadFile(planFile)
    if err != nil {
        return fmt.Errorf("failed to read plan file: %w", err)
    }
    
    hash := sha256.Sum256(planData)
    task.PlanHash = hex.EncodeToString(hash[:])
    
    // 记录当前Agent ID（如果是Agent模式）
    if task.ExecutionMode == models.ExecutionModeAgent && task.AgentID != nil {
        task.WarmupAgentID = task.AgentID
    }
    
    // 保存到数据库
    if err := e.db.Save(task).Error; err != nil {
        return err
    }
    
    // 不清理工作目录！
    log.Printf("[Optimization] Keeping work directory for task %d at %s", task.ID, workDir)
    
    return nil
}

// 2. Agent启动时的预热检查
func (a *Agent) OnStart() error {
    log.Printf("[Agent] Starting, checking for apply_pending tasks...")
    
    // 查询分配给当前pool的apply_pending任务
    var tasks []models.WorkspaceTask
    err := a.db.Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", a.poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
        Where("workspace_tasks.warmup_status != ?", "ready").
        Find(&tasks).Error
    
    if err != nil {
        return err
    }
    
    if len(tasks) == 0 {
        log.Printf("[Agent] No apply_pending tasks need warmup")
        return nil
    }
    
    log.Printf("[Agent] Found %d apply_pending tasks, starting warmup...", len(tasks))
    
    // 为每个任务执行预热
    for _, task := range tasks {
        go a.warmupTask(&task)
    }
    
    return nil
}

// 3. Agent预热逻辑
func (a *Agent) warmupTask(task *models.WorkspaceTask) error {
    log.Printf("[Agent] Warming up task %d", task.ID)
    
    // 更新预热状态
    task.WarmupStatus = "warming"
    task.WarmupAgentID = &a.agentID
    task.WarmupAt = timePtr(time.Now())
    task.WarmupExpiresAt = timePtr(time.Now().Add(1 * time.Hour))
    a.db.Save(task)
    
    // 创建工作目录
    workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d", 
        task.WorkspaceID, task.ID)
    
    if err := os.MkdirAll(workDir, 0755); err != nil {
        return a.handleWarmupError(task, err)
    }
    
    // 生成配置文件
    if err := a.generateConfigFiles(task, workDir); err != nil {
        return a.handleWarmupError(task, err)
    }
    
    // 拉取State文件
    if err := a.fetchStateFile(task, workDir); err != nil {
        return a.handleWarmupError(task, err)
    }
    
    // Terraform init
    if err := a.terraformInit(workDir); err != nil {
        return a.handleWarmupError(task, err)
    }
    
    // 从数据库恢复plan.out
    if err := a.restorePlanFile(task, workDir); err != nil {
        return a.handleWarmupError(task, err)
    }
    
    // 验证plan.out的hash
    planFile := filepath.Join(workDir, "plan.out")
    planData, err := os.ReadFile(planFile)
    if err != nil {
        return a.handleWarmupError(task, err)
    }
    
    hash := sha256.Sum256(planData)
    currentHash := hex.EncodeToString(hash[:])
    
    if currentHash != task.PlanHash {
        return a.handleWarmupError(task, 
            fmt.Errorf("plan hash mismatch: expected %s, got %s", 
                task.PlanHash, currentHash))
    }
    
    // 标记为ready
    task.WarmupStatus = "ready"
    a.db.Save(task)
    
    log.Printf("[Agent] Task %d warmup completed successfully", task.ID)
    return nil
}

// 4. Apply执行逻辑（优化版）
func (a *Agent) ExecuteApply(task *models.WorkspaceTask) error {
    log.Printf("[Agent] Executing apply for task %d", task.ID)
    
    workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d", 
        task.WorkspaceID, task.ID)
    
    // 场景1: Agent未销毁，工作目录存在
    if task.WarmupAgentID != nil && *task.WarmupAgentID == a.agentID {
        log.Printf("[Agent] Same agent, checking local plan file...")
        
        planFile := filepath.Join(workDir, "plan.out")
        if _, err := os.Stat(planFile); err == nil {
            // 验证hash
            planData, err := os.ReadFile(planFile)
            if err == nil {
                hash := sha256.Sum256(planData)
                currentHash := hex.EncodeToString(hash[:])
                
                if currentHash == task.PlanHash {
                    log.Printf("[Agent]  Hash verified, using local plan file (FAST PATH)")
                    // 直接执行apply，无需init！
                    return a.terraformApplyDirect(workDir)
                }
            }
            log.Printf("[Agent] Hash mismatch or read error, falling back to normal flow")
        }
    }
    
    // 场景2: Agent已销毁或预热完成
    if task.WarmupStatus == "ready" {
        log.Printf("[Agent]  Warmup ready, executing apply immediately")
        
        // 验证预热是否过期
        if task.WarmupExpiresAt != nil && time.Now().After(*task.WarmupExpiresAt) {
            log.Printf("[Agent] Warmup expired, re-warming...")
            if err := a.warmupTask(task); err != nil {
                return err
            }
        }
        
        return a.terraformApplyDirect(workDir)
    }
    
    // 场景3: 需要完整准备（fallback）
    log.Printf("[Agent] No warmup, executing normal flow...")
    return a.executeApplyNormal(task)
}

// 5. 直接执行apply（最快路径）
func (a *Agent) terraformApplyDirect(workDir string) error {
    cmd := exec.Command("terraform", "apply", "-no-color", "-auto-approve", "plan.out")
    cmd.Dir = workDir
    
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    
    log.Printf("[Agent] Executing: terraform apply (direct)")
    startTime := time.Now()
    
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("terraform apply failed: %w\n%s", err, stderr.String())
    }
    
    duration := time.Since(startTime)
    log.Printf("[Agent]  Apply completed in %v (OPTIMIZED)", duration)
    
    return nil
}

// 6. 处理预热错误
func (a *Agent) handleWarmupError(task *models.WorkspaceTask, err error) error {
    log.Printf("[Agent] ❌ Warmup failed for task %d: %v", task.ID, err)
    
    task.WarmupStatus = "failed"
    a.db.Save(task)
    
    // 预热失败不影响任务执行，用户确认后会走正常流程
    return err
}
```

### Agent销毁感知机制

```go
// 1. Agent心跳机制
func (a *Agent) StartHeartbeat(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            a.sendHeartbeat()
        }
    }
}

// 2. 服务端检测Agent离线
func (m *TaskQueueManager) MonitorAgentHealth(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.checkOfflineAgents()
        }
    }
}

func (m *TaskQueueManager) checkOfflineAgents() {
    // 查找超过2分钟未心跳的Agent
    var offlineAgents []models.Agent
    m.db.Where("last_heartbeat_at < ?", time.Now().Add(-2*time.Minute)).
        Where("status = ?", "online").
        Find(&offlineAgents)
    
    for _, agent := range offlineAgents {
        log.Printf("[Monitor] Agent %s is offline, marking tasks for re-warmup", agent.AgentID)
        
        // 将该Agent预热的任务标记为需要重新预热
        m.db.Model(&models.WorkspaceTask{}).
            Where("warmup_agent_id = ?", agent.AgentID).
            Where("warmup_status = ?", "ready").
            Where("status = ?", models.TaskStatusApplyPending).
            Updates(map[string]interface{}{
                "warmup_status": "none",
                "warmup_agent_id": nil,
            })
        
        // 标记Agent为offline
        agent.Status = "offline"
        m.db.Save(&agent)
    }
}

// 3. 新Agent注册时触发预热
func (h *AgentHandler) RegisterAgent(c *gin.Context) {
    // ... 注册逻辑
    
    // 注册成功后，触发预热检查
    go func() {
        time.Sleep(5 * time.Second) // 等待Agent完全启动
        agent.OnStart()
    }()
}
```

## 🔄 综合优化方案

### 推荐实施方案

结合两个优化点，推荐以下综合方案（方案C）：

```go
// 完整优化流程
Plan阶段:
1. 创建工作目录
2. 生成配置文件
3. 拉取State文件
4. terraform init
5. terraform plan -out=plan.out
6. 保存plan.out到数据库
7. 保持工作目录不清理  优化1
8. 任务状态 → apply_pending

Apply_Pending阶段（新增）:
1. 如果是Agent/K8s模式:
   - 立即推送预热任务到Agent  优化2
   - Agent创建工作目录（或复用Plan的目录）
   - Agent执行init（如果需要）
   - Agent恢复plan.out
   - Agent标记为ready
2. 等待用户确认

Apply阶段:
1. 用户确认
2. Agent检查预热状态
3. 如果ready，立即执行apply 
4. 如果未ready，等待预热完成或重新准备
5. 执行terraform apply
6. 清理工作目录
```

### 实施优先级

**Phase 1: 优化点1（保持工作目录）**
- 优先级: P0（高）
- 工作量: 2-3天
- 风险: 低
- 收益: 中等

**Phase 2: 优化点2（Agent预热）**
- 优先级: P1（中）
- 工作量: 5-7天
- 风险: 中等
- 收益: 高

## 🚨 需要注意的问题

### 1. Agent自动缩容与预热任务冲突

**问题**: Agent在没有running任务时会自动缩容，但apply_pending任务需要Agent保持预热状态

**场景分析**:
```
1. Plan完成 → apply_pending状态
2. Agent预热完成 → warmup_status = ready
3. 没有running任务 → Auto-scaler检测到空闲
4. Agent被缩容销毁 ❌
5. 用户确认Apply → 需要重新创建Agent并预热
```

**解决方案A: 修改缩容逻辑（推荐）** 

```go
// 在计算需要的Agent数量时，考虑apply_pending任务
func (s *K8sDeploymentService) CalculateDesiredReplicas(poolID string) int {
    // 1. 统计running任务
    var runningCount int64
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusRunning).
        Count(&runningCount)
    
    // 2. 统计apply_pending任务（需要保持预热）
    var applyPendingCount int64
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
        Where("workspace_tasks.warmup_status = ?", "ready"). // 只计算已预热的
        Count(&applyPendingCount)
    
    // 3. 计算所需Agent数量
    // 每个Agent可以处理3个任务（running + apply_pending）
    totalTasks := runningCount + applyPendingCount
    desiredReplicas := (totalTasks + 2) / 3 // 向上取整
    
    // 4. 最小副本数
    if desiredReplicas < s.minReplicas {
        desiredReplicas = s.minReplicas
    }
    
    log.Printf("[AutoScaler] Pool %s: running=%d, apply_pending(ready)=%d, desired=%d",
        poolID, runningCount, applyPendingCount, desiredReplicas)
    
    return int(desiredReplicas)
}
```

**解决方案B: 预热过期时间配合缩容延迟**

```go
// 1. 设置合理的预热过期时间
task.WarmupExpiresAt = time.Now().Add(30 * time.Minute) // 30分钟

// 2. 缩容延迟时间应该小于预热过期时间
// 例如：缩容延迟15分钟，预热过期30分钟
// 这样即使Agent被缩容，预热也还没过期，新Agent可以重新预热

// 3. 在Auto-scaler中添加缩容延迟
func (s *K8sDeploymentService) ShouldScaleDown(poolID string) bool {
    // 检查最后一次有任务的时间
    var lastTaskTime time.Time
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status IN (?)", []string{"running", "apply_pending"}).
        Select("MAX(workspace_tasks.updated_at)").
        Scan(&lastTaskTime)
    
    // 15分钟内有任务，不缩容
    if time.Since(lastTaskTime) < 15*time.Minute {
        return false
    }
    
    return true
}
```

**解决方案C: 混合方案 + 预热失败计数（最佳）** ⭐⭐⭐⭐⭐

```go
// 结合方案A和B，并添加预热失败保护
func (s *K8sDeploymentService) CalculateDesiredReplicasV2(poolID string) int {
    // 1. 统计running任务
    var runningCount int64
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusRunning).
        Count(&runningCount)
    
    // 2. 统计apply_pending任务（已预热且未过期，且预热失败次数<3）
    var applyPendingCount int64
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
        Where("workspace_tasks.warmup_status = ?", "ready").
        Where("workspace_tasks.warmup_expires_at > ?", time.Now()).
        Where("COALESCE(workspace_tasks.warmup_retry_count, 0) < ?", 3). //  防止死循环
        Count(&applyPendingCount)
    
    // 3. 计算所需Agent数量
    totalTasks := runningCount + applyPendingCount
    desiredReplicas := (totalTasks + 2) / 3
    
    // 4. 应用最小副本数
    if desiredReplicas < s.minReplicas {
        desiredReplicas = s.minReplicas
    }
    
    // 5. 缩容保护：如果要缩容，检查是否在冷却期内
    currentReplicas := s.getCurrentReplicas(poolID)
    if desiredReplicas < currentReplicas {
        if !s.canScaleDown(poolID) {
            log.Printf("[AutoScaler] Pool %s in cooldown, keeping current replicas", poolID)
            return currentReplicas
        }
    }
    
    return int(desiredReplicas)
}

// Agent预热逻辑（添加重试计数）
func (a *Agent) warmupTask(task *models.WorkspaceTask) error {
    log.Printf("[Agent] Warming up task %d (retry count: %d)", task.ID, task.WarmupRetryCount)
    
    //  检查重试次数，防止死循环
    if task.WarmupRetryCount >= 3 {
        log.Printf("[Agent] Task %d warmup retry limit reached, giving up", task.ID)
        task.WarmupStatus = "failed"
        task.WarmupRetryCount = 3
        a.db.Save(task)
        return fmt.Errorf("warmup retry limit reached")
    }
    
    // 增加重试计数
    task.WarmupRetryCount++
    task.WarmupStatus = "warming"
    task.WarmupAgentID = &a.agentID
    task.WarmupAt = timePtr(time.Now())
    task.WarmupExpiresAt = timePtr(time.Now().Add(30 * time.Minute))
    a.db.Save(task)
    
    // ... 执行预热逻辑
    
    // 预热成功，重置重试计数
    task.WarmupRetryCount = 0 //  成功后重置
    task.WarmupStatus = "ready"
    a.db.Save(task)
    
    return nil
}

// Agent离线检测（添加重试计数检查）
func (m *TaskQueueManager) checkOfflineAgents() {
    var offlineAgents []models.Agent
    m.db.Where("last_heartbeat_at < ?", time.Now().Add(-2*time.Minute)).
        Where("status = ?", "online").
        Find(&offlineAgents)
    
    for _, agent := range offlineAgents {
        log.Printf("[Monitor] Agent %s is offline", agent.AgentID)
        
        //  只重置重试次数<3的任务，避免死循环
        m.db.Model(&models.WorkspaceTask{}).
            Where("warmup_agent_id = ?", agent.AgentID).
            Where("warmup_status = ?", "ready").
            Where("status = ?", models.TaskStatusApplyPending).
            Where("COALESCE(warmup_retry_count, 0) < ?", 3). //  关键保护
            Updates(map[string]interface{}{
                "warmup_status": "none",
                "warmup_agent_id": nil,
            })
        
        agent.Status = "offline"
        m.db.Save(&agent)
    }
}
```

**关键改进**:
1.  添加 `warmup_retry_count` 字段，记录预热重试次数
2.  预热失败超过3次后，不再尝试预热（避免死循环）
3.  Auto-scaler只计算重试次数<3的任务
4.  预热成功后重置重试计数
5.  Agent离线时只重置重试次数<3的任务

**解决方案D: Pod直接管理 + 任务槽位机制（最优架构）** ⭐⭐⭐⭐⭐

**核心思想**: 
- 不使用Deployment，直接管理Pod
- 每个Pod有多个任务槽位（如3个）
- apply_pending任务占用槽位但不算running
- 预热阶段（pre-apply）占用槽位且算running
- Freeze Schedule时强制清理所有Pod

```go
// 1. Pod任务槽位管理
type PodSlot struct {
    SlotID    int       `json:"slot_id"`    // 槽位ID (0, 1, 2)
    TaskID    *uint     `json:"task_id"`    // 当前任务ID
    TaskType  string    `json:"task_type"`  // plan/plan_and_apply
    Status    string    `json:"status"`     // idle/running/reserved
    UpdatedAt time.Time `json:"updated_at"`
}

type ManagedPod struct {
    PodName       string     `json:"pod_name"`
    AgentID       string     `json:"agent_id"`
    PoolID        string     `json:"pool_id"`
    Slots         []PodSlot  `json:"slots"`         // 3个槽位
    CreatedAt     time.Time  `json:"created_at"`
    LastHeartbeat time.Time  `json:"last_heartbeat"`
}

// 2. 槽位规则
// - 每个Pod有3个槽位
// - plan任务：可以并发，占用1个槽位
// - plan_and_apply任务（running）：独占1个槽位
// - plan_and_apply任务（apply_pending）：占用1个槽位但标记为reserved
// - 预热阶段（pre-apply）：占用槽位且算running

// 3. 优化后的Auto-scaler逻辑
func (s *K8sPodManager) ReconcilePods(poolID string) error {
    // 1. 统计需要槽位的任务
    // running状态的任务（包括planning和applying阶段）
    var runningTasks []models.WorkspaceTask
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusRunning).
        Find(&runningTasks)
    
    // 2. 统计apply_pending任务（已预热，占用槽位但不算running）
    var applyPendingTasks []models.WorkspaceTask
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
        Where("workspace_tasks.warmup_status = ?", "ready").
        Find(&applyPendingTasks)
    
    // 3. 计算需要的总槽位数
    totalSlots := len(runningTasks) + len(applyPendingTasks)
    
    // 4. 计算需要的Pod数量（每个Pod 3个槽位）
    desiredPods := (totalSlots + 2) / 3 // 向上取整
    
    // 5. 应用最小副本数
    if desiredPods < s.minReplicas {
        desiredPods = s.minReplicas
    }
    
    // 6. 获取当前Pod列表
    currentPods := s.listPods(poolID)
    
    // 7. 调整Pod数量
    s.reconcilePods(poolID, desiredPods, currentPods, runningTasks, applyPendingTasks)
    
    log.Printf("[PodManager] Pool %s: running=%d, apply_pending=%d, total_slots=%d, pods=%d",
        poolID, len(runningTasks), len(applyPendingTasks), totalSlots, desiredPods)
    
    return nil
}

// 4. Pod槽位分配
func (s *K8sPodManager) reconcilePods(
    poolID string,
    desiredPods int,
    currentPods []ManagedPod,
    runningTasks []models.WorkspaceTask,
    applyPendingTasks []models.WorkspaceTask,
) error {
    // 1. 扩容：创建新Pod
    if len(currentPods) < desiredPods {
        for i := len(currentPods); i < desiredPods; i++ {
            pod := s.createPod(poolID)
            log.Printf("[PodManager] Created pod %s", pod.PodName)
        }
    }
    
    // 2. 缩容：删除空闲Pod
    if len(currentPods) > desiredPods {
        // 找出完全空闲的Pod（所有槽位都是idle）
        idlePods := s.findIdlePods(currentPods)
        
        deleteCount := len(currentPods) - desiredPods
        for i := 0; i < deleteCount && i < len(idlePods); i++ {
            s.deletePod(idlePods[i].PodName)
            log.Printf("[PodManager] Deleted idle pod %s", idlePods[i].PodName)
        }
    }
    
    // 3. 分配running任务到槽位
    for _, task := range runningTasks {
        if task.AgentID == nil {
            // 任务未分配Agent，找一个有空闲槽位的Pod
            pod := s.findPodWithFreeSlot(currentPods)
            if pod != nil {
                s.assignTaskToSlot(pod, &task, "running")
            }
        }
    }
    
    // 4. 分配apply_pending任务到槽位（标记为reserved）
    for _, task := range applyPendingTasks {
        if task.WarmupAgentID == nil {
            // 任务未预热，找一个有空闲槽位的Pod
            pod := s.findPodWithFreeSlot(currentPods)
            if pod != nil {
                s.assignTaskToSlot(pod, &task, "reserved")
                // 触发预热
                go s.triggerWarmup(pod, &task)
            }
        }
    }
    
    return nil
}

// 5. 查找有空闲槽位的Pod
func (s *K8sPodManager) findPodWithFreeSlot(pods []ManagedPod) *ManagedPod {
    for _, pod := range pods {
        for _, slot := range pod.Slots {
            if slot.Status == "idle" {
                return &pod
            }
        }
    }
    return nil
}

// 6. 分配任务到槽位
func (s *K8sPodManager) assignTaskToSlot(
    pod *ManagedPod, 
    task *models.WorkspaceTask, 
    slotStatus string,
) error {
    // 找到空闲槽位
    for i, slot := range pod.Slots {
        if slot.Status == "idle" {
            pod.Slots[i].TaskID = &task.ID
            pod.Slots[i].TaskType = string(task.TaskType)
            pod.Slots[i].Status = slotStatus // "running" 或 "reserved"
            pod.Slots[i].UpdatedAt = time.Now()
            
            // 更新任务的Agent ID
            if slotStatus == "running" {
                task.AgentID = &pod.AgentID
            } else if slotStatus == "reserved" {
                task.WarmupAgentID = &pod.AgentID
            }
            s.db.Save(task)
            
            log.Printf("[PodManager] Assigned task %d to pod %s slot %d (status: %s)",
                task.ID, pod.PodName, i, slotStatus)
            return nil
        }
    }
    return fmt.Errorf("no free slot available")
}

// 5. Freeze Schedule处理
func (s *FreezeScheduleService) EnterFreezeWindow(poolID string) error {
    log.Printf("[FreezeSchedule] Pool %s entering freeze window", poolID)
    
    // 1. 标记Pool为frozen
    s.db.Model(&models.AgentPool{}).
        Where("pool_id = ?", poolID).
        Update("is_frozen", true)
    
    // 2. 强制删除所有Pod（包括Worker和Reserved）
    pods := s.podManager.listPods(poolID)
    for _, pod := range pods {
        s.podManager.deletePod(pod.PodName)
        log.Printf("[FreezeSchedule] Deleted pod %s (freeze window)", pod.PodName)
    }
    
    // 3. 将所有预热任务标记为需要重新预热
    s.db.Model(&models.WorkspaceTask{}).
        Joins("JOIN workspaces ON workspaces.workspace_id = workspace_tasks.workspace_id").
        Where("workspaces.current_pool_id = ?", poolID).
        Where("workspace_tasks.status = ?", models.TaskStatusApplyPending).
        Updates(map[string]interface{}{
            "warmup_status": "none",
            "warmup_agent_id": nil,
        })
    
    log.Printf("[FreezeSchedule] Pool %s freeze window activated, all pods deleted", poolID)
    return nil
}

// 6. 解冻处理
func (s *FreezeScheduleService) ExitFreezeWindow(poolID string) error {
    log.Printf("[FreezeSchedule] Pool %s exiting freeze window", poolID)
    
    // 1. 标记Pool为unfrozen
    s.db.Model(&models.AgentPool{}).
        Where("pool_id = ?", poolID).
        Update("is_frozen", false)
    
    // 2. 触发Pod重建
    s.podManager.ReconcilePods(poolID)
    
    log.Printf("[FreezeSchedule] Pool %s unfrozen, pods will be recreated", poolID)
    return nil
}
```

**优点**:
-  完全避免死循环（预留Pod不会被自动缩容）
-  预留Pod不占用任务容量
-  架构更清晰（Worker Pod vs Reserved Pod）
-  Freeze Schedule强制清理所有Pod
-  解冻后自动重建Pod和预热

**缺点**:
-  需要重构K8s管理逻辑（从Deployment改为直接管理Pod）
-  实现复杂度最高

**Pod槽位机制**:
```
每个Pod有3个槽位:

槽位状态:
- idle: 空闲，可以接受新任务
- running: 正在执行任务（plan或apply阶段）
- reserved: 预留给apply_pending任务（已预热）

容量计算:
- running状态的plan+apply任务：占用1个槽位，算running
- apply_pending状态的任务：占用1个槽位，但不算running（标记为reserved）
- 预热阶段（pre-apply）：占用槽位且算running

示例:
Pod-1: [running: task-1, reserved: task-2, idle]
- task-1: plan_and_apply, status=running, stage=applying
- task-2: plan_and_apply, status=apply_pending, warmup_status=ready
- 槽位3: 空闲

Pod-2: [running: task-3, running: task-4, running: task-5]
- task-3/4/5: plan任务，可以并发

缩容规则:
- 只删除所有槽位都是idle的Pod
- 有reserved槽位的Pod不会被删除
- 有running槽位的Pod不会被删除
```

**Freeze Schedule行为**:
```
进入Freeze Window:
1. 标记Pool为frozen
2. 强制删除所有Pod（Worker + Reserved）
3. 重置所有预热状态

退出Freeze Window:
1. 标记Pool为unfrozen
2. 触发Pod重建
3. 自动预热apply_pending任务
```

**数据库Schema补充**:
```sql
-- 添加预热重试计数字段
ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS warmup_retry_count INTEGER DEFAULT 0;
```

**推荐配置**:
```yaml
auto_scaler:
  min_replicas: 1              # 最小副本数
  max_replicas: 10             # 最大副本数
  scale_up_delay: 30s          # 扩容延迟
  scale_down_delay: 15m        # 缩容延迟（重要！）
  warmup_expire_time: 30m      # 预热过期时间
  warmup_max_retries: 3        # 预热最大重试次数（防止死循环）
  cooldown_period: 15m         # 缩容冷却期
```

**关键点**:
1.  缩容时考虑apply_pending任务
2.  只计算已预热且未过期的任务
3.  **只计算预热重试次数<3的任务（防止死循环）** ⭐
4.  设置合理的缩容延迟（15分钟）
5.  预热过期时间应大于缩容延迟
6.  添加缩容冷却期，避免频繁缩容
7.  预热失败3次后放弃，等待用户手动处理

### 2. 工作目录管理

**问题**: 保持工作目录会占用磁盘空间

**解决方案**:
```go
// 定期清理过期的工作目录
func (m *TaskQueueManager) CleanupExpiredWorkDirs() {
    // 清理超过24小时的apply_pending任务的工作目录
    // 清理超过1小时的已完成任务的工作目录
}
```

### 2. Agent重启/销毁

**问题**: Agent重启后，预热的工作目录丢失

**解决方案**:
```go
// Agent启动时检查预热任务
func (a *Agent) OnStart() {
    // 1. 查询分配给自己的apply_pending任务
    // 2. 重新执行预热
    // 3. 标记为ready
}
```

### 3. 预热过期

**问题**: 预热后长时间未确认，配置可能已变更

**解决方案**:
```go
// 设置预热过期时间（如1小时）
task.WarmupExpiresAt = time.Now().Add(1 * time.Hour)

// 用户确认时检查
if task.WarmupStatus == WarmupStatusReady {
    if time.Now().After(*task.WarmupExpiresAt) {
        // 预热已过期，重新准备
        return a.executeApplyNormal(taskID)
    }
}
```

### 4. Plan文件完整性

**问题**: 保持工作目录期间，plan.out可能被篡改

**解决方案**:
```go
// 保存plan.out时计算hash
planData, _ := os.ReadFile("plan.out")
task.PlanHash = sha256.Sum256(planData)

// Apply前验证hash
currentHash := sha256.Sum256(planData)
if currentHash != task.PlanHash {
    return errors.New("plan file corrupted")
}
```

## 📊 性能提升预估

### 优化点1: 保持工作目录

| 场景 | 当前耗时 | 优化后耗时 | 提升 |
|------|---------|-----------|------|
| 小型配置 | 15-20秒 | 2-3秒 | 85% |
| 中型配置 | 30-45秒 | 2-3秒 | 93% |
| 大型配置 | 60-90秒 | 2-3秒 | 96% |

**说明**: 主要节省init时间（下载Provider插件）

### 优化点2: Agent预热

| 场景 | 当前体验 | 优化后体验 | 提升 |
|------|---------|-----------|------|
| 用户确认后等待 | 15-60秒 | <1秒 | 98% |
| Agent重启场景 | 30-90秒 | <1秒 | 99% |

**说明**: 用户感知的等待时间几乎为0

## 📝 总结

### 合理性评估

两个优化点都**非常合理**：

1. **优化点1（保持工作目录）**:
   -  技术上完全可行
   -  性能提升明显
   -  实现复杂度低
   -  风险可控
   - **推荐立即实施**

2. **优化点2（Agent预热）**:
   -  技术上可行
   -  用户体验提升巨大
   -  实现复杂度中等
   -  需要仔细处理边界情况
   - **推荐作为Phase 2实施**

### 实施建议

1. **先实施优化点1**: 快速见效，风险低
2. **再实施优化点2**: 需要更多测试和验证
3. **逐步推广**: 先在测试环境验证，再推广到生产

### 预期收益

- **性能**: Apply启动时间减少85-96%
- **用户体验**: 确认后几乎立即开始执行
- **资源**: 减少重复的网络和磁盘IO
- **成本**: 减少Agent执行时间，降低成本

---

**相关文档**:
- [terraform-execution-states-and-sequential-guarantee.md](terraform-execution-states-and-sequential-guarantee.md) - 执行流程状态
- [15-terraform-execution-detail.md](workspace/15-terraform-execution-detail.md) - 执行流程设计
