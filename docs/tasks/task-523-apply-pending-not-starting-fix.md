# Task 523 Apply Pending Not Starting Fix

## 问题描述

任务 523 在用户点击 "Confirm Apply" 后没有启动执行。

## 根本原因

`GetNextExecutableTask` 函数只查找 `status = 'pending'` 的任务，完全忽略了 `status = 'apply_pending'` 的任务。

当用户确认 apply 后：
1. `ConfirmApply` 将任务状态设置为 `apply_pending`
2. 调用 `TryExecuteNextTask` 
3. `GetNextExecutableTask` 查找可执行任务
4. **BUG**: 只查询 `status = 'pending'`，找不到 `apply_pending` 任务
5. 返回 "No executable tasks"，任务卡住

## 如何判断用户已经confirm

### 状态流转

```
Plan完成 -> status='apply_pending', stage='apply_pending' (等待用户确认)
         ↓
用户Confirm -> status='apply_pending', stage='apply_pending' + apply_description有值
         ↓
开始执行 -> status='running', stage='applying'
```

### 关键标志

1. **status = 'apply_pending'**: 任务处于等待apply状态
2. **apply_description 有值**: 用户已经点击确认并输入了备注
3. **TryExecuteNextTask被调用**: ConfirmApply会主动触发任务调度

## 修复方案

修改 `backend/services/task_queue_manager.go` 中的 `GetNextExecutableTask` 函数：

### 修复前

```go
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
	log.Printf("[TaskQueue] GetNextExecutableTask for workspace %s", workspaceID)

	// 注意：不检查 apply_pending 任务！
	// apply_pending 任务需要用户通过 ConfirmApply 显式确认后才能执行

	// 2. 检查plan_and_apply pending任务
	var planAndApplyTask models.WorkspaceTask
	err := m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
		workspaceID, models.TaskTypePlanAndApply, models.TaskStatusPending).
		Order("created_at ASC").
		First(&planAndApplyTask).Error
	// ...
}
```

### 修复后

```go
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
	log.Printf("[TaskQueue] GetNextExecutableTask for workspace %s", workspaceID)

	// 1. 首先检查是否有 apply_pending 任务（用户已确认，等待执行）
	var applyPendingTask models.WorkspaceTask
	err := m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
		workspaceID, models.TaskTypePlanAndApply, models.TaskStatusApplyPending).
		Order("created_at ASC").
		First(&applyPendingTask).Error

	if err == nil {
		// 找到 apply_pending 任务，这是用户已确认的任务，应该优先执行
		log.Printf("[TaskQueue] Found apply_pending task %d for workspace %s (user confirmed)", applyPendingTask.ID, workspaceID)
		return &applyPendingTask, nil
	}

	if err != gorm.ErrRecordNotFound {
		log.Printf("[TaskQueue] Error checking apply_pending tasks: %v", err)
		return nil, err
	}

	// 2. 检查plan_and_apply pending任务
	var planAndApplyTask models.WorkspaceTask
	err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
		workspaceID, models.TaskTypePlanAndApply, models.TaskStatusPending).
		Order("created_at ASC").
		First(&planAndApplyTask).Error
	// ...
}
```

## 修复要点

1. **优先级**: `apply_pending` 任务优先级最高（用户已确认）
2. **防止自动执行**: 服务器重启时不会自动执行 `apply_pending` 任务
3. **显式触发**: 只有 `ConfirmApply` 调用 `TryExecuteNextTask` 时才会执行

## 验证步骤

1. 重启后端服务
2. 检查任务 523 的状态：
   ```sql
   SELECT id, task_type, status, stage, apply_description 
   FROM workspace_tasks 
   WHERE id = 523;
   ```
3. 如果 `status = 'apply_pending'` 且 `apply_description` 有值，说明用户已确认
4. 观察日志，应该看到：
   ```
   [TaskQueue] Found apply_pending task 523 for workspace ws-mb7m9ii5ey (user confirmed)
   [TaskQueue] Successfully pushed task 523 to agent xxx (action: apply)
   ```

## 相关文件

- `backend/services/task_queue_manager.go` - 任务队列管理器
- `backend/controllers/workspace_task_controller.go` - ConfirmApply 实现

## 日期

2025-11-07
