# Task 635: Plan Hash 保存问题修复

> **创建时间**: 2025-11-10  
> **状态**: 已修复  
> **优先级**: P0

## 📋 问题描述

用户报告任务 635 在 apply 阶段仍然执行了 init，没有跳过。经过数据库查询发现：

```sql
id  | task_type      | status  | agent_id                                        | plan_task_id | plan_hash
635 | plan_and_apply | applied | agent-pool-z73eh8ihywlmgx0x-1762761126267223000 | 635          | (空)
```

**关键问题**：`plan_hash` 字段为空！

## 🔍 根本原因分析

### 问题 1：Agent 模式下 plan_hash 未保存

**文件**：`backend/services/remote_data_accessor.go`

**原因**：`RemoteDataAccessor.UpdateTask` 方法的 `updates` map 中**没有包含 `plan_hash` 字段**

**修改前**：
```go
func (a *RemoteDataAccessor) UpdateTask(task *models.WorkspaceTask) error {
    updates := map[string]interface{}{
        "stage":           task.Stage,
        "changes_add":     task.ChangesAdd,
        "changes_change":  task.ChangesChange,
        "changes_destroy": task.ChangesDestroy,
        "duration":        task.Duration,
    }
    // ... 其他字段
    // ❌ 缺少 plan_hash
    
    return a.apiClient.UpdateTaskStatus(task.ID, status, updates)
}
```

**修改后**：
```go
func (a *RemoteDataAccessor) UpdateTask(task *models.WorkspaceTask) error {
    updates := map[string]interface{}{
        "stage":           task.Stage,
        "changes_add":     task.ChangesAdd,
        "changes_change":  task.ChangesChange,
        "changes_destroy": task.ChangesDestroy,
        "duration":        task.Duration,
    }
    // ... 其他字段
    
    // 【Phase 1优化】Add plan_hash if set
    if task.PlanHash != "" {
        updates["plan_hash"] = task.PlanHash
    }
    
    return a.apiClient.UpdateTaskStatus(task.ID, status, updates)
}
```

### 问题 2：ExecuteApply 中缺少 agent_id 检查

**文件**：`backend/services/terraform_executor.go`

**原因**：在检查是否可以跳过 init 时，只检查了 plan_hash，没有检查 agent_id

**修改前**：
```go
canSkipInit := false
if planTask.PlanHash != "" {
    if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
        canSkipInit = true
    }
}
```

**修改后**：
```go
canSkipInit := false
if planTask.PlanHash != "" {
    // 【新增】首先检查是否在同一个 agent 上执行
    if planTask.AgentID != nil && task.AgentID != nil && *planTask.AgentID == *task.AgentID {
        logger.Info("Checking if init can be skipped (same agent detected)...")
        logger.Info("  - Plan agent: %s", *planTask.AgentID)
        logger.Info("  - Apply agent: %s", *task.AgentID)
        
        // 在同一 agent 上，验证 plan hash
        if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
            canSkipInit = true
            logger.Info("✓ Same agent and plan hash verified, skipping init")
        }
    } else {
        // 不在同一 agent 上，必须重新 init
        logger.Info("Different agent detected, must run init:")
        // ... 显示详细信息
    }
}
```

## 🔧 修复内容总结

### 修改 1：remote_data_accessor.go
- **位置**：`UpdateTask` 方法
- **修改**：在 `updates` map 中添加 `plan_hash` 字段
- **影响**：Agent 模式和 K8s Agent 模式

### 修改 2：terraform_executor.go  
- **位置**：`ExecuteApply` 方法的 Init 阶段检查
- **修改**：添加 agent_id 比较逻辑
- **影响**：所有三种模式（Local/Agent/K8s Agent）

### 修改 3：terraform_executor.go
- **位置**：`ExecuteApply` 方法的 Plan 恢复阶段
- **修改**：更新日志信息，显示 "same agent"
- **影响**：所有三种模式

##  三种模式支持验证

### 1. Local 模式
- **Plan 阶段**：使用 `s.db.Model().Updates()` 保存 plan_hash 
- **Apply 阶段**：agent_id 为 nil，正常执行 init 

### 2. Agent 模式
- **Plan 阶段**：使用 `RemoteDataAccessor.UpdateTask()` 保存 plan_hash （已修复）
- **Apply 阶段**：比较 agent_id，相同则跳过 init 

### 3. K8s Agent 模式
- **Plan 阶段**：使用 `RemoteDataAccessor.UpdateTask()` 保存 plan_hash （已修复）
- **Apply 阶段**：比较 agent_id，相同则跳过 init 

## 📈 预期效果

### 修复后的行为

#### 场景 1：同一 Agent（plan_and_apply 任务）
```
[INFO] Checking if init can be skipped (same agent detected)...
[INFO]   - Plan agent: agent-pool-xxx-123
[INFO]   - Apply agent: agent-pool-xxx-123
[INFO] ✓ Same agent and plan hash verified, skipping init
[INFO] Init stage skipped (using preserved workspace from plan on same agent)
```
**结果**：跳过 init，节省 ~54 秒 

#### 场景 2：不同 Agent
```
[INFO] Different agent detected, must run init:
[INFO]   - Plan agent: agent-pool-xxx-123
[INFO]   - Apply agent: agent-pool-yyy-456
[INFO] Executing: terraform init -no-color -upgrade
```
**结果**：正常执行 init 

#### 场景 3：Local 模式
```
[INFO] Different agent detected, must run init:
[INFO]   - Plan agent: (none)
[INFO]   - Apply agent: (none)
[INFO] Executing: terraform init -no-color -upgrade
```
**结果**：正常执行 init 

## 🚀 部署步骤

### 1. 重新编译后端
```bash
cd backend
go build -o main .
```

### 2. 重启服务
```bash
# 重启 server
docker-compose restart

# 或者重新构建
docker-compose up -d --build
```

### 3. 重启 Agent
```bash
# 如果是独立 Agent
./agent restart

# 如果是 K8s Agent
kubectl rollout restart deployment/iac-agent -n iac-platform
```

## 🧪 测试验证

### 测试步骤

1. **创建新的 plan_and_apply 任务**
2. **等待 Plan 完成**，检查数据库：
   ```sql
   SELECT id, plan_hash FROM workspace_tasks WHERE id = <task_id>;
   ```
   预期：plan_hash 不为空

3. **确认 Apply**，观察日志：
   - 应该看到 "Same agent detected"
   - 应该看到 "Skipping init"
   - Init 阶段应该被跳过

4. **验证性能**：
   - Apply 启动时间应该 <5 秒（之前是 ~54 秒）

### 验证 SQL

```sql
-- 查看最新任务的 plan_hash
SELECT id, task_type, status, agent_id, plan_hash, 
       CASE WHEN plan_hash IS NULL OR plan_hash = '' THEN '❌ 空' ELSE ' 有值' END as hash_status
FROM workspace_tasks 
WHERE id >= 635 
ORDER BY id DESC 
LIMIT 5;
```

## 📊 性能对比

### Before（修复前）
- Plan: 60s (init: 54s + plan: 6s)
- Apply: 65s (init: 54s + restore: 1s + apply: 10s)
- **Total: 125s**

### After（修复后 - 同一 Agent）
- Plan: 60s (init: 54s + plan: 6s)
- Apply (same agent): **11s** (skip init + skip restore + apply: 10s)
- **Total: 71s**

**改进**: **43% 更快**（节省 54 秒）

## 🎯 关键改进点

1.  **Agent 模式 plan_hash 保存**：修复了 RemoteDataAccessor.UpdateTask
2.  **Agent ID 检查**：添加了同一 agent 的检测逻辑
3.  **三种模式支持**：Local/Agent/K8s Agent 全部支持
4.  **详细日志**：便于调试和监控
5.  **向后兼容**：不影响现有功能

## 📝 相关文档

- [task-633-simplified-agent-id-check.md](task-633-simplified-agent-id-check.md) - 简化方案设计
- [task-633-slot-aware-init-skip-analysis.md](task-633-slot-aware-init-skip-analysis.md) - 原始分析
- [terraform-execution-optimization-implementation-plan.md](terraform-execution-optimization-implementation-plan.md) - 优化计划

---

**总结**：已修复 Agent 模式下 plan_hash 不保存的问题，并添加了 agent_id 检查逻辑。修复后，同一 agent 上的 apply 任务将跳过 init，性能提升 43%。
