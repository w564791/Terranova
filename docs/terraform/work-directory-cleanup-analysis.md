# Work Directory Cleanup Analysis

## Issue Report

**Date**: 2025-11-10  
**Reporter**: User  
**Status**: Confirmed - Daemon process is unnecessary

## Problem Description

Server startup shows:
```
2025/11/10 14:43:19 [TaskQueue] Starting work directory cleaner (interval: 1 hour)
2025/11/10 14:43:19 [TaskQueue] Starting work directory cleanup...
```

This indicates a daemon process is running to clean up work directories, but this is unnecessary overhead.

## Current Implementation Analysis

### Location: `backend/services/task_queue_manager.go`

#### 1. Daemon Process (Lines 1089-1110)
```go
// StartWorkDirCleaner 启动工作目录清理器（定期清理过期的工作目录）
func (m *TaskQueueManager) StartWorkDirCleaner(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // 每小时执行一次
	defer ticker.Stop()

	log.Printf("[TaskQueue] Starting work directory cleaner (interval: 1 hour)")

	// 启动时立即执行一次清理
	m.CleanupExpiredWorkDirs()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[TaskQueue] Work directory cleaner stopped")
			return
		case <-ticker.C:
			m.CleanupExpiredWorkDirs()
		}
	}
}
```

#### 2. Cleanup Logic (Lines 1112-1180)
The cleanup logic has these rules:
- **Completed tasks** (success/applied/failed/cancelled): Keep for 1 hour
- **apply_pending tasks**: Keep for 24 hours (for plan file reuse)
- **pending/running tasks**: Never clean (in use)

### Location: `backend/services/terraform_executor.go`

#### Current Behavior in ExecutePlan (Lines 1050-1055)
```go
// 【Phase 1优化】Plan完成后不清理工作目录，保留给Apply使用
logger.Info("Preserving work directory for potential apply: %s", workDir)
log.Printf("Task %d: Work directory preserved at %s (plan_hash: %s)", 
    task.ID, workDir, task.PlanHash[:16]+"...")
```

**Plan tasks preserve the work directory** - this is correct for plan+apply optimization.

#### Current Behavior in ExecuteApply (End of function)
```go
// Apply成功完成后，解锁workspace
logger.Info("Unlocking workspace after successful apply...")
if err := s.dataAccessor.UnlockWorkspace(workspace.WorkspaceID); err != nil {
    logger.Warn("Failed to unlock workspace: %v", err)
} else {
    logger.Info("✓ Workspace unlocked successfully")
}

log.Printf("Task %d applied successfully", task.ID)

return nil
```

**Apply tasks do NOT clean up the work directory** - this is the issue!

## Correct Cleanup Strategy

### For Plan Tasks
- **Plan-only tasks**: Clean up immediately after completion
- **Plan+Apply tasks**: Preserve work directory for apply phase

### For Apply Tasks  
- **After apply completes**: Clean up work directory immediately
- **After apply fails**: Clean up work directory immediately

## Recommended Solution

### 1. Remove Daemon Process

**File**: `backend/main.go`

Remove the call to `StartWorkDirCleaner`:
```go
// Remove this:
go taskQueueManager.StartWorkDirCleaner(ctx)
```

### 2. Add Cleanup to ExecutePlan

**File**: `backend/services/terraform_executor.go`

At the end of `ExecutePlan`, add cleanup for plan-only tasks:

```go
// 根据任务类型决定最终状态
if task.TaskType == models.TaskTypePlanAndApply {
    // ... existing code for plan+apply ...
    // Preserve work directory for apply
    logger.Info("Preserving work directory for potential apply: %s", workDir)
} else {
    // 单独的Plan任务：直接完成并清理
    task.Status = models.TaskStatusSuccess
    task.Stage = "completed"
    log.Printf("Task %d (plan) completed successfully", task.ID)
    
    // Clean up work directory for plan-only tasks
    logger.Info("Cleaning up work directory for plan-only task: %s", workDir)
    if err := s.CleanupWorkspace(workDir); err != nil {
        logger.Warn("Failed to cleanup work directory: %v", err)
    } else {
        logger.Info("✓ Work directory cleaned up successfully")
    }
}
```

### 3. Add Cleanup to ExecuteApply

**File**: `backend/services/terraform_executor.go`

At the end of `ExecuteApply` (both success and failure paths):

```go
// Apply成功完成后，解锁workspace
logger.Info("Unlocking workspace after successful apply...")
if err := s.dataAccessor.UnlockWorkspace(workspace.WorkspaceID); err != nil {
    logger.Warn("Failed to unlock workspace: %v", err)
} else {
    logger.Info("✓ Workspace unlocked successfully")
}

// Clean up work directory after apply completes
logger.Info("Cleaning up work directory after apply: %s", workDir)
if err := s.CleanupWorkspace(workDir); err != nil {
    logger.Warn("Failed to cleanup work directory: %v", err)
} else {
    logger.Info("✓ Work directory cleaned up successfully")
}

log.Printf("Task %d applied successfully", task.ID)
return nil
```

For failure path in `ExecuteApply`, add cleanup before returning error:
```go
// In saveTaskFailure for apply:
if taskType == "apply" {
    task.ApplyOutput = fullOutput

    // Apply失败时，解锁workspace（如果之前被锁定）
    logger.Info("Unlocking workspace after apply failure...")
    if unlockErr := s.dataAccessor.UnlockWorkspace(task.WorkspaceID); unlockErr != nil {
        logger.Warn("Failed to unlock workspace: %v", unlockErr)
    } else {
        logger.Info("✓ Workspace unlocked")
    }
    
    // Clean up work directory after apply failure
    // Note: workDir needs to be passed to saveTaskFailure or reconstructed
    workDir := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d", task.WorkspaceID, task.ID)
    logger.Info("Cleaning up work directory after apply failure: %s", workDir)
    if cleanupErr := s.CleanupWorkspace(workDir); cleanupErr != nil {
        logger.Warn("Failed to cleanup work directory: %v", cleanupErr)
    } else {
        logger.Info("✓ Work directory cleaned up")
    }
}
```

### 4. Remove Daemon-Related Code

**File**: `backend/services/task_queue_manager.go`

Remove these functions:
- `StartWorkDirCleaner` (lines 1089-1110)
- `CleanupExpiredWorkDirs` (lines 1112-1180)
- `shouldCleanupWorkDir` (lines 1182-1220)
- `calculateDirSize` (lines 1222-1233)

These are no longer needed with immediate cleanup.

## Benefits of This Approach

1. **Simpler**: No background daemon process
2. **Immediate**: Work directories cleaned up right after task completion
3. **Predictable**: Cleanup happens at well-defined points
4. **Resource-efficient**: No periodic scanning of filesystem
5. **Correct**: Matches the actual task lifecycle

## Edge Cases Handled

1. **Server restart during task execution**: 
   - Orphan tasks are marked as failed by `CleanupOrphanTasks`
   - Work directories remain until next task runs (acceptable)

2. **Apply-pending tasks**:
   - Work directory preserved during plan phase
   - Cleaned up after apply completes or fails

3. **Cancelled tasks**:
   - Should also clean up work directory in cancellation handler

## Implementation Priority

**High Priority** - This is a simple optimization that:
- Removes unnecessary daemon process
- Cleans up resources immediately
- Simplifies the codebase

## Testing Checklist

- [ ] Plan-only task cleans up work directory after completion
- [ ] Plan+Apply task preserves work directory after plan
- [ ] Plan+Apply task cleans up work directory after apply success
- [ ] Plan+Apply task cleans up work directory after apply failure
- [ ] Cancelled tasks clean up work directory
- [ ] No daemon process running after server start
- [ ] No hourly cleanup logs in server output

## Conclusion

Your understanding is correct: **work directory cleanup should happen automatically after task completion, not via a daemon process**. The current implementation has the daemon as unnecessary overhead, and cleanup should be integrated into the task execution flow itself.
