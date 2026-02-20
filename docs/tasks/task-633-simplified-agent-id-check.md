# Task 633: 简化的 Agent ID 检查方案

> **创建时间**: 2025-11-10  
> **状态**: 简化方案  
> **优先级**: P0

## 📋 关键发现

用户指出：**WorkspaceTask 表已经有 `agent_id` 字段，只需要比较 agent_id 就可以确认是否在同一个 slot 中执行！**

这大大简化了实现方案。

## 🔍 当前数据库结构

```go
type WorkspaceTask struct {
    ID             int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
    AgentID       *string    `json:"agent_id" gorm:"type:varchar(50);index"` //  已存在
    // ... 其他字段
    PlanHash      string     `json:"plan_hash" gorm:"type:varchar(64)"` //  已存在
}
```

**关键点**:
-  `agent_id` 字段已存在
-  `plan_hash` 字段已存在
-  不需要额外的 `warmup_pod_name` 和 `warmup_slot_id` 字段

## 🎯 简化的实现方案

### 核心逻辑

**同一个 slot = 同一个 agent**

因此，只需要检查：
1. Plan task 的 `agent_id` 
2. Apply task 的 `agent_id`
3. 如果相同 + plan hash 匹配 → 跳过 init

### 当前代码（Line ~1400-1450）

```go
// ========== 阶段2: Init（可能跳过）==========
// 【Phase 1优化】检查是否可以跳过init
canSkipInit := false
if planTask.PlanHash != "" {
    logger.Info("Checking if init can be skipped (plan hash exists)...")
    if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
        canSkipInit = true
        logger.Info("✓ Plan hash verified, skipping init (optimization)")
        log.Printf("Task %d: Skipping init due to plan hash match (saved ~85-96%% time)", task.ID)
    } else {
        logger.Warn("Plan hash mismatch or plan file missing, will run init")
    }
}
```

**问题**: 没有检查 agent_id！

### 修复方案（只需修改一处）

```go
// ========== 阶段2: Init（可能跳过）==========
// 【Phase 1优化】检查是否可以跳过init
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
            log.Printf("Task %d: Skipping init (same agent optimization, saved ~85-96%% time)", task.ID)
        } else {
            logger.Warn("Plan hash mismatch, will run init")
        }
    } else {
        // 不在同一 agent 上，必须重新 init
        logger.Info("Different agent detected, must run init:")
        if planTask.AgentID != nil {
            logger.Info("  - Plan agent: %s", *planTask.AgentID)
        } else {
            logger.Info("  - Plan agent: (none)")
        }
        if task.AgentID != nil {
            logger.Info("  - Apply agent: %s", *task.AgentID)
        } else {
            logger.Info("  - Apply agent: (none)")
        }
    }
}

if !canSkipInit {
    logger.StageBegin("init")
    // ... 执行 init
    logger.StageEnd("init")
} else {
    logger.Info("Init stage skipped (using preserved workspace from plan on same agent)")
}
```

## 📝 完整实施步骤

### Step 1: 修改 ExecuteApply 中的 canSkipInit 逻辑（15分钟）

**文件**: `backend/services/terraform_executor.go` Line ~1400-1450

**修改内容**: 在检查 plan hash 之前，先检查 agent_id 是否匹配

### Step 2: 同样优化 Plan 恢复逻辑（10分钟）

**文件**: `backend/services/terraform_executor.go` Line ~1450-1500

```go
// ========== 阶段3: Restoring Plan（可能跳过）==========
planFile := filepath.Join(workDir, "plan.out")
needRestorePlan := true

// 【Phase 1优化】如果在同一 agent 且 plan 文件已存在，跳过恢复
if canSkipInit && planTask.PlanHash != "" {
    logger.Info("Checking if plan file already exists on same agent...")
    if _, err := os.Stat(planFile); err == nil {
        if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
            needRestorePlan = false
            logger.Info("✓ Plan file already exists on same agent, skipping restore")
            log.Printf("Task %d: Reusing existing plan file (same agent optimization)", task.ID)
        }
    }
}

if needRestorePlan {
    logger.StageBegin("restoring_plan")
    // ... 恢复 plan 文件
    logger.StageEnd("restoring_plan")
} else {
    logger.Info("Plan restore skipped (using preserved plan file from same agent)")
}
```

##  验证方案

### 测试场景 1: 同一 Agent（应该跳过 init）

```
1. 创建 plan_and_apply 任务
2. Plan 阶段在 agent-abc 上执行
3. Apply 阶段也在 agent-abc 上执行
4. 预期：跳过 init 和 plan restore
5. 日志应显示：
   - "Same agent detected"
   - "Skipping init (same agent optimization)"
```

### 测试场景 2: 不同 Agent（应该执行 init）

```
1. 创建 plan 任务，在 agent-abc 上执行
2. Agent-abc 被删除
3. 创建 apply 任务，在 agent-xyz 上执行
4. 预期：执行完整 init
5. 日志应显示：
   - "Different agent detected"
   - "Plan agent: agent-abc"
   - "Apply agent: agent-xyz"
```

### 测试场景 3: Local 模式（agent_id 为 nil）

```
1. 在 Local 模式下执行 plan_and_apply
2. agent_id 为 nil
3. 预期：正常执行（不跳过 init）
4. 日志应显示：
   - "Plan agent: (none)"
   - "Apply agent: (none)"
```

## 📈 性能影响

### Before（当前）
- Plan: 60s (init: 54s + plan: 6s)
- Apply: 65s (init: 54s + restore: 1s + apply: 10s)
- **Total: 125s**

### After（修复后 - 同一 Agent）
- Plan: 60s (init: 54s + plan: 6s)
- Apply (same agent): **11s** (skip init + skip restore + apply: 10s)
- **Total: 71s**

**改进**: **43% 更快**（节省 54 秒）

### After（修复后 - 不同 Agent）
- Plan: 60s
- Apply (different agent): 65s (正常执行 init)
- **Total: 125s**（与之前相同，正确行为）

## 🎯 实施清单

- [ ] Step 1: 修改 ExecuteApply 中的 canSkipInit 逻辑（15分钟）
  - [ ] 添加 agent_id 比较
  - [ ] 添加详细日志
- [ ] Step 2: 优化 Plan 恢复逻辑（10分钟）
  - [ ] 同样检查 agent_id
- [ ] Step 3: 测试同一 Agent 场景
- [ ] Step 4: 测试不同 Agent 场景
- [ ] Step 5: 测试 Local 模式场景
- [ ] Step 6: 验证性能改进

**总时间**: 25分钟代码修改 + 30分钟测试 = **约1小时**

## 🚨 风险评估

| 风险 | 影响 | 概率 | 缓解措施 |
|------|------|------|---------|
| agent_id 为 nil | 低 | 低 | 检查 nil 值，fallback 到正常流程 |
| agent_id 不匹配 | 无 | 中 | 正确执行 init（预期行为）|
| plan hash 不匹配 | 低 | 低 | 重新执行 init（预期行为）|

**总体风险**: **极低**

## 💡 关键优势

1. **极简实现**: 只需修改一处代码（~20行）
2. **无需新字段**: 使用现有的 `agent_id` 字段
3. **向后兼容**: Local 模式（agent_id=nil）自动 fallback
4. **性能提升**: 同一 agent 场景下节省 43% 时间
5. **安全可靠**: 不同 agent 场景下正确执行 init

## 📊 与原方案对比

| 项目 | 原方案（pod+slot） | 简化方案（agent_id） |
|------|-------------------|---------------------|
| 新增字段 | 2个（pod_name, slot_id） | 0个（使用现有字段）|
| 代码修改 | 8个文件 | 1个文件 |
| 实施时间 | 1.5-2小时 | 1小时 |
| 复杂度 | 中 | 低 |
| 准确性 | 高 | 高 |
| 风险 | 中 | 极低 |

**结论**: 简化方案更优！

---

**下一步**: 立即实施简化方案，预计1小时完成。
