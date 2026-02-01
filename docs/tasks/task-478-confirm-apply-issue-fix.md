# Task 478 Confirm Apply Issue - 修复方案

## 问题描述

任务478在执行confirm apply时失败，但任务状态没有正确更新，导致：
1. 任务卡在某个中间状态
2. 无法重新confirm
3. 后续任务无法执行

## 根本原因

在 `workspace_task_controller.go` 的 `ConfirmApply` 函数中，当资源版本快照验证失败时：

```go
if err := c.executor.ValidateResourceVersionSnapshot(&task, logger); err != nil {
    c.streamManager.Close(task.ID)
    ctx.JSON(http.StatusConflict, gin.H{
        "error":   "Resources have changed since plan",
        "details": err.Error(),
    })
    return  // ❌ 直接返回，没有更新任务状态
}
```

**问题**：
1. 只返回了HTTP错误，但任务状态仍然是 `apply_pending`
2. 没有通知TaskQueueManager执行下一个任务
3. 任务被"卡住"了

## 修复方案

### 方案1：在验证失败时更新任务状态（推荐）

修改 `ConfirmApply` 函数，在验证失败时：

```go
if err := c.executor.ValidateResourceVersionSnapshot(&task, logger); err != nil {
    c.streamManager.Close(task.ID)
    
    // 更新任务状态为failed
    task.Status = models.TaskStatusFailed
    task.ErrorMessage = fmt.Sprintf("Resources have changed since plan: %v", err)
    task.CompletedAt = timePtr(time.Now())
    c.db.Save(&task)
    
    // 解锁workspace（如果被锁定）
    var workspace models.Workspace
    if err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error; err == nil {
        if workspace.IsLocked {
            workspace.IsLocked = false
            workspace.LockedBy = nil
            workspace.LockedAt = nil
            workspace.LockReason = ""
            c.db.Save(&workspace)
        }
    }
    
    // 通知队列管理器执行下一个任务
    go func() {
        if err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID); err != nil {
            log.Printf("Failed to start next task after validation failure: %v", err)
        }
    }()
    
    ctx.JSON(http.StatusConflict, gin.H{
        "error":   "Resources have changed since plan",
        "details": err.Error(),
    })
    return
}
```

### 方案2：临时修复 - 手动更新任务478状态

如果需要立即解决任务478的问题，可以手动更新数据库：

```sql
-- 查看任务478当前状态
SELECT id, workspace_id, task_type, status, stage, error_message 
FROM workspace_tasks 
WHERE id = 478;

-- 更新任务状态为failed
UPDATE workspace_tasks 
SET status = 'failed',
    error_message = 'Resources changed between plan and apply confirmation',
    completed_at = NOW()
WHERE id = 478;

-- 检查workspace是否被锁定
SELECT workspace_id, is_locked, locked_by, lock_reason 
FROM workspaces 
WHERE workspace_id = (SELECT workspace_id FROM workspace_tasks WHERE id = 478);

-- 如果被锁定，解锁workspace
UPDATE workspaces 
SET is_locked = false,
    locked_by = NULL,
    locked_at = NULL,
    lock_reason = ''
WHERE workspace_id = (SELECT workspace_id FROM workspace_tasks WHERE id = 478)
  AND is_locked = true;
```

## 实施步骤

### 立即修复（临时方案）

1. 执行SQL脚本更新任务478状态
2. 重启后端服务，触发任务队列恢复

### 永久修复（代码修改）

1. 修改 `backend/controllers/workspace_task_controller.go` 的 `ConfirmApply` 函数
2. 添加验证失败时的状态更新逻辑
3. 测试验证
4. 部署上线

## 预防措施

为了避免类似问题，建议：

1. **所有可能失败的操作都应该更新任务状态**
2. **失败时应该通知TaskQueueManager**
3. **失败时应该解锁相关资源**
4. **添加更详细的错误日志**

## 测试验证

修复后需要测试：

1. 创建plan_and_apply任务
2. Plan成功后，修改资源配置
3.
