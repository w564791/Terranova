# Task 633: Slot-Aware Init Skip Implementation Analysis

> **Created**: 2025-11-10  
> **Status**: Analysis Complete  
> **Priority**: P0 (Performance Optimization)

## 📋 Problem Statement

Based on the log output from task #633, the system is currently:
1.  Skipping init when plan hash matches
2. ❌ **NOT checking if plan and apply are in the same slot**

According to the design requirements, when a plan task and plan+apply task are in the **same slot**, the applying phase should:
- Skip initialization (already implemented via hash check)
- **Verify the tasks are in the same slot** (NOT implemented)

## 🔍 Current Implementation Analysis

### What's Already Implemented (90%)

From `backend/services/terraform_executor.go` line ~1400-1450:

```go
// ========== 阶段2: Init（可能跳过）==========
// 【Phase 1优化】检查是否可以跳过init
canSkipInit := false
if planTask.PlanHash != "" {
    logger.Info("Checking if init can be skipped (plan hash exists)...")
    if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
        canSkipInit = true
        logger.Info("✓ Plan hash verified, skipping init (optimization)")
    }
}
```

**Problem**: This only checks plan hash, not slot information!

### What's Missing (10%)

According to `docs/task-600-fix-complexity-assessment.md`, we need to add:

1. **Database fields** (already exist from `scripts/add_slot_fields.sql`):
   - `warmup_pod_name` VARCHAR(100)
   - `warmup_slot_id` INTEGER

2. **Slot validation logic**:
   ```go
   func (s *TerraformExecutor) isSlotValid(planTask *models.WorkspaceTask) bool {
       // Check if same agent, pod, and slot
   }
   ```

3. **Enhanced skip logic**:
   ```go
   if planTask.PlanHash != "" && s.isSlotValid(planTask) {
       // Skip init only if BOTH hash matches AND same slot
   }
   ```

## 📊 Gap Analysis

### Current Behavior (Task #633 Log)

```
[07:24:49.355] [INFO] Restoring plan file from plan task #633...
[07:24:49.355] [INFO]   - Plan data size: 55.6 KB
[07:24:49.355] [INFO] ✓ Restored plan file to work directory
[07:24:49.355] [INFO] Validating plan file...
[07:24:49.355] [INFO] ✓ Plan file is valid and ready for apply
========== RESTORING_PLAN END at 2025-11-10 07:24:49.355 ==========
========== APPLYING BEGIN at 2025-11-10 07:24:49.355 ==========
```

**Issue**: The system is restoring the plan file, which means it's NOT reusing the existing plan file from the same slot!

### Expected Behavior (With Slot Awareness)

```
[07:24:49.355] [INFO] Checking if init can be skipped (same slot)...
[07:24:49.355] [INFO] ✓ Same agent: agent-xyz
[07:24:49.355] [INFO] ✓ Same pod: pod-abc-123
[07:24:49.355] [INFO] ✓ Same slot: slot-1
[07:24:49.355] [INFO] ✓ Plan hash verified
[07:24:49.355] [INFO] ✓ Skipping init AND plan restore (using preserved workspace)
========== APPLYING BEGIN at 2025-11-10 07:24:49.355 ==========
```

## 🎯 Implementation Plan

### Step 1: Verify Database Schema (5 minutes)

Check if `warmup_pod_name` and `warmup_slot_id` fields exist:

```sql
SELECT column_name, data_type 
FROM information_schema.columns 
WHERE table_name = 'workspace_tasks' 
AND column_name IN ('warmup_pod_name', 'warmup_slot_id');
```

If not, run:
```bash
psql -U postgres -d iac_platform -f scripts/add_slot_fields.sql
```

### Step 2: Update TerraformExecutor Structure (10 minutes)

**File**: `backend/services/terraform_executor.go`

Add fields to store current execution context:

```go
type TerraformExecutor struct {
    db                 *gorm.DB
    dataAccessor       DataAccessor
    streamManager      *OutputStreamManager
    signalManager      *SignalManager
    downloader         *TerraformDownloader
    cachedBinaryPath   string
    cachedBinaryVersion string
    
    // 【新增】Agent/Pod/Slot信息
    agentID  string  // 当前执行的Agent ID
    podName  string  // 当前执行的Pod名称
    slotID   int     // 当前执行的Slot ID
}
```

### Step 3: Update Constructor (10 minutes)

Modify `NewTerraformExecutorWithAccessor`:

```go
func NewTerraformExecutorWithAccessor(
    accessor DataAccessor, 
    streamManager *OutputStreamManager,
    agentID string,
    podName string,
    slotID int,
) *TerraformExecutor {
    var downloader *TerraformDownloader
    if remoteAccessor, ok := accessor.(*RemoteDataAccessor); ok {
        downloader = NewTerraformDownloaderForAgent(remoteAccessor)
    }

    return &TerraformExecutor{
        db:            nil,
        dataAccessor:  accessor,
        streamManager: streamManager,
        signalManager: GetSignalManager(),
        downloader:    downloader,
        agentID:       agentID,  // 新增
        podName:       podName,  // 新增
        slotID:        slotID,   // 新增
    }
}
```

### Step 4: Save Slot Info in ExecutePlan (15 minutes)

**File**: `backend/services/terraform_executor.go` ~line 750

After plan completes, save slot information:

```go
// 【Phase 1优化】计算plan文件的hash
logger.Info("Calculating plan file hash for optimization...")
planHash, err := s.calculatePlanHash(planFile)
if err != nil {
    logger.Warn("Failed to calculate plan hash: %v", err)
} else {
    task.PlanHash = planHash
    logger.Info("✓ Plan hash calculated: %s", planHash[:16]+"...")
    
    // 【新增】记录Pod和Slot信息（用于Apply阶段的优化判断）
    if task.AgentID != nil && s.podName != "" && s.slotID > 0 {
        task.WarmupPodName = &s.podName
        task.WarmupSlotID = &s.slotID
        logger.Info("✓ Recorded execution context:")
        logger.Info("  - Agent ID: %s", *task.AgentID)
        logger.Info("  - Pod Name: %s", s.podName)
        logger.Info("  - Slot ID: %d", s.slotID)
    }
}
```

### Step 5: Add Slot Validation Method (15 minutes)

**File**: `backend/services/terraform_executor.go`

Add new method after `verifyPlanHash`:

```go
// isSlotValid 检查Apply任务是否在与Plan任务相同的Slot中执行
func (s *TerraformExecutor) isSlotValid(planTask *models.WorkspaceTask) bool {
    // 检查是否有slot信息
    if planTask.WarmupPodName == nil || planTask.WarmupSlotID == nil {
        return false
    }
    
    // 检查Agent ID是否匹配
    if planTask.AgentID == nil || *planTask.AgentID != s.agentID {
        return false
    }
    
    // 检查Pod名称是否匹配
    if *planTask.WarmupPodName != s.podName {
        return false
    }
    
    // 检查Slot ID是否匹配
    if *planTask.WarmupSlotID != s.slotID {
        return false
    }
    
    return true
}
```

### Step 6: Update ExecuteApply Logic (20 minutes)

**File**: `backend/services/terraform_executor.go` ~line 1400-1450

Replace current logic with slot-aware version:

```go
// ========== 阶段2: Init（可能跳过）==========
// 【Phase 1优化】检查是否可以跳过init
canSkipInit := false
if planTask.PlanHash != "" {
    // 首先检查是否在同一个Slot中执行
    if s.isSlotValid(planTask) {
        logger.Info("Checking if init can be skipped (same slot detected)...")
        logger.Info("  - Agent ID: %s ✓", s.agentID)
        logger.Info("  - Pod Name: %s ✓", s.podName)
        logger.Info("  - Slot ID: %d ✓", s.slotID)
        
        // 在同一Slot中，验证plan hash
        if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
            canSkipInit = true
            logger.Info("✓ Same slot and plan hash verified, skipping init")
            log.Printf("Task %d: Skipping init (same slot optimization, saved ~85-96%% time)", task.ID)
        } else {
            logger.Warn("Plan hash mismatch, will run init")
        }
    } else {
        // 不在同一Slot中，必须重新init
        logger.Info("Different slot detected, must run init:")
        if planTask.WarmupPodName != nil {
            logger.Info("  - Plan pod: %s, Current pod: %s", *planTask.WarmupPodName, s.podName)
        }
        if planTask.WarmupSlotID != nil {
            logger.Info("  - Plan slot: %d, Current slot: %d", *planTask.WarmupSlotID, s.slotID)
        }
    }
}

if !canSkipInit {
    logger.StageBegin("init")
    // ... 执行init
    logger.StageEnd("init")
} else {
    logger.Info("Init stage skipped (using preserved workspace from plan in same slot)")
}

// ========== 阶段3: Restoring Plan（可能跳过）==========
planFile := filepath.Join(workDir, "plan.out")
needRestorePlan := true

// 【Phase 1优化】如果在同一Slot且plan文件已存在，跳过恢复
if canSkipInit && planTask.PlanHash != "" {
    logger.Info("Checking if plan file already exists in same slot...")
    if _, err := os.Stat(planFile); err == nil {
        if s.verifyPlanHash(workDir, planTask.PlanHash, logger) {
            needRestorePlan = false
            logger.Info("✓ Plan file already exists in same slot, skipping restore")
            log.Printf("Task %d: Reusing existing plan file (same slot optimization)", task.ID)
        }
    }
}

if needRestorePlan {
    logger.StageBegin("restoring_plan")
    // ... 恢复plan文件
    logger.StageEnd("restoring_plan")
} else {
    logger.Info("Plan restore skipped (using preserved plan file from same slot)")
}
```

### Step 7: Update Agent Startup Code (15 minutes)

**File**: `backend/cmd/agent/main.go`

Pass pod and slot information to TerraformExecutor:

```go
// 获取Pod和Slot信息
podName := os.Getenv("POD_NAME")
slotIDStr := os.Getenv("SLOT_ID")
slotID, _ := strconv.Atoi(slotIDStr)

log.Printf("Agent execution context:")
log.Printf("  - Agent ID: %s", agentID)
log.Printf("  - Pod Name: %s", podName)
log.Printf("  - Slot ID: %d", slotID)

// 创建TerraformExecutor（传入slot信息）
executor := services.NewTerraformExecutorWithAccessor(
    dataAccessor,
    streamManager,
    agentID,
    podName,
    slotID,
)
```

### Step 8: Update Model Definition (5 minutes)

**File**: `backend/internal/models/workspace.go`

Verify fields exist:

```go
type WorkspaceTask struct {
    // ... existing fields
    
    // 【Phase 1优化】Plan hash和Slot信息
    PlanHash         string     `json:"plan_hash" gorm:"type:varchar(64)"`
    WarmupPodName    *string    `json:"warmup_pod_name" gorm:"type:varchar(100)"`
    WarmupSlotID     *int       `json:"warmup_slot_id"`
}
```

## 📈 Expected Performance Impact

### Before (Current)
- Plan: 60s (init: 54s + plan: 6s)
- Apply: 65s (init: 54s + restore: 1s + apply: 10s)
- **Total: 125s**

### After (With Slot Awareness)
- Plan: 60s (init: 54s + plan: 6s)
- Apply (same slot): **11s** (skip init + skip restore + apply: 10s)
- **Total: 71s**

**Improvement**: **43% faster** (54 seconds saved)

##  Verification Steps

1. **Check database schema**:
   ```sql
   \d workspace_tasks
   ```

2. **Test same-slot scenario**:
   - Create plan_and_apply task
   - Verify it runs in same slot
   - Check logs for "Same slot and plan hash verified"

3. **Test different-slot scenario**:
   - Create plan task
   - Wait for pod to be deleted
   - Create apply task
   - Verify it runs init (different slot)

4. **Monitor metrics**:
   - Apply startup time should be <5s for same-slot
   - Apply startup time should be ~54s for different-slot

## 🚨 Risk Assessment

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Slot info not saved | Medium | Low | Fallback to normal init |
| Pod name mismatch | Low | Low | Strict validation |
| Slot ID mismatch | Low | Low | Strict validation |
| Hash mismatch | Medium | Low | Re-run init |

## 📝 Implementation Checklist

- [ ] Step 1: Verify database schema (5 min)
- [ ] Step 2: Update TerraformExecutor structure (10 min)
- [ ] Step 3: Update constructor (10 min)
- [ ] Step 4: Save slot info in ExecutePlan (15 min)
- [ ] Step 5: Add slot validation method (15 min)
- [ ] Step 6: Update ExecuteApply logic (20 min)
- [ ] Step 7: Update agent startup code (15 min)
- [ ] Step 8: Update model definition (5 min)
- [ ] Step 9: Test same-slot scenario
- [ ] Step 10: Test different-slot scenario
- [ ] Step 11: Verify performance improvement

**Total Estimated Time**: 1.5-2 hours

## 🎯 Success Criteria

1.  Same-slot apply skips both init and plan restore
2.  Different-slot apply runs full init
3.  Apply startup time <5s for same-slot
4.  No errors in production
5.  40%+ performance improvement

---

**Next Steps**: Begin implementation following the checklist above.
