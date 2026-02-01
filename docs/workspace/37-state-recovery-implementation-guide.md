# State保存失败恢复机制 - 实施指南

## 当前状态
-  SaveStateToDatabase方法已改为公开方法
- ⏳ 需要添加RetryStateSave API
- ⏳ 需要添加DownloadStateBackup API
- ⏳ 需要添加前端UI

## 实施步骤

### Step 1: 添加后端API方法

在`backend/controllers/workspace_task_controller.go`末尾添加：

```go
// RetryStateSave 重试State保存
// POST /api/v1/workspaces/:id/tasks/:task_id/retry-state-save
func (c *WorkspaceTaskController) RetryStateSave(ctx *gin.Context) {
	workspaceID, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	taskID, _ := strconv.ParseUint(ctx.Param("task_id"), 10, 32)

	var task models.WorkspaceTask
	if err := c.db.Where("id = ? AND workspace_id = ?", taskID, workspaceID).
		First(&task).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 检查是否是State保存失败的任务
	if !strings.Contains(task.ErrorMessage, "state save failed") {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Task is not in state save failed status",
		})
		return
	}

	// 从错误信息中提取备份路径
	backupPath := extractBackupPath(task.ErrorMessage)
	if backupPath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Cannot find backup path in error message",
		})
		return
	}

	// 读取备份文件
	stateData, err := os.ReadFile(backupPath)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to read backup file: %v", err),
		})
		return
	}

	// 获取workspace
	var workspace models.Workspace
	if err := c.db.First(&workspace, workspaceID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
		return
	}

	// 重新保存到数据库
	if err := c.executor.SaveStateToDatabase(&workspace, &task, stateData); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save state: %v", err),
		})
		return
	}

	// 更新任务状态
	task.Status = models.TaskStatusSuccess
	task.ErrorMessage = ""
	c.db.Save(&task)

	// 解锁workspace
	workspace.IsLocked = false
	workspace.LockedBy = nil
	workspace.LockedAt = nil
	workspace.LockReason = ""
	c.db.Save(&workspace)

	ctx.JSON(http.StatusOK, gin.H{
		"message": "State saved successfully, workspace unlocked",
		"task":    task,
	})
}

// DownloadStateBackup 下载State备份
// GET /api/v1/workspaces/:id/tasks/:task_id/state-backup
func (c *WorkspaceTaskController) DownloadStateBackup(ctx *gin.Context) {
	workspaceID, _ := strconv.ParseUint(ctx.Param("id"), 10, 32)
	taskID, _ := strconv.ParseUint(ctx.Param("task_id"), 10, 32)

	var task models.WorkspaceTask
	if err := c.db.Where("id = ? AND workspace_id = ?", taskID, workspaceID).
		First(&task).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 从错误信息中提取备份路径
	backupPath := extractBackupPath(task.ErrorMessage)
	if backupPath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Cannot find backup path in error message",
		})
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "Backup file not found",
		})
		return
	}

	// 返回文件
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=terraform_%d.tfstate", taskID))
	ctx.File(backupPath)
}

// extractBackupPath 从错误信息中提取备份路径
func extractBackupPath(errorMessage string) string {
	// "backup at: /var/backup/states/ws_10_task_63_1760251780.tfstate"
	parts := strings.Split(errorMessage, "backup at: ")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}
```

### Step 2: 添加路由

在`backend/internal/router/router.go`的任务管理部分添加：

```go
workspaces.POST("/:id/tasks/:task_id/retry-state-save", taskController.RetryStateSave)
workspaces.GET("/:id/tasks/:task_id/state-backup", taskController.DownloadStateBackup)
```

### Step 3: 添加前端UI

在`frontend/src/pages/TaskDetail.tsx`中：

1. 添加状态和方法：
```tsx
const isStateSaveFailure = task.error_message?.includes('state save failed');

const extractBackupPath = (errorMessage?: string): string | null => {
  if (!errorMessage) return null;
  const match = errorMessage.match(/backup at: (.+)$/);
  return match ? match[1].trim() : null;
};

const backupPath = extractBackupPath(task.error_message);

const handleRetryStateSave = async () => {
  if (!confirm('确定要重试State保存吗？')) {
    return;
  }

  try {
    await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/retry-state-save`);
    alert('State保存成功，workspace已解锁');
    fetchTask();
  } catch (err: any) {
    const message = err.response?.data?.error || err.message || 'Failed to retry';
    alert(`重试失败: ${message}`);
  }
};

const handleDownloadStateBackup = () => {
  window.open(
    `http://localhost:8080/api/v1/workspaces/${workspaceId}/tasks/${taskId}/state-backup`,
    '_blank'
  );
};
```

2. 修改错误卡片：
```tsx
{task.error_message && (
  <div className={styles.errorCard}>
    <div className={styles.errorHeader}>
      <span className={styles.errorIcon}>✗</span>
      <span className={styles.errorTitle}>Error</span>
    </div>
    <pre className={styles.errorContent}>{task.error_message}</pre>
    
    {isStateSaveFailure && (
      <div className={styles.errorActions}>
        <button
          className={styles.retryButton}
          onClick={handleRetryStateSave}
        >
          🔄 Retry State Save
        </button>
        <button
          className={styles.downloadButton}
          onClick={handleDownloadStateBackup}
        >
          ⬇ Download State Backup
        </button>
        {backupPath && (
          <div className={styles.backupPath}>
            Backup: <code>{backupPath}</code>
          </div>
        )}
      </div>
    )}
  </div>
)}
```

### Step 4: 添加CSS样式

在`frontend/src/pages/TaskDetail.module.css`中添加：

```css
.errorActions {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid #FECACA;
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.retryButton {
  padding: 8px 16px;
  background: var(--color-blue-600);
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s;
}

.retryButton:hover {
  background: var(--color-blue-700);
  transform: translateY(-1px);
}

.downloadButton {
  padding: 8px 16px;
  background: var(--color-green-600);
  color: white;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  transition: all 0.2s;
}

.downloadButton:hover {
  background: var(--color-green-700);
  transform: translateY(-1px);
}

.backupPath {
  font-size: 12px;
  color: #7F1D1D;
  flex: 1 1 100%;
}

.backupPath code {
  background: white;
  padding: 4px 8px;
  border-radius: 4px;
  font-family: var(--font-mono);
  border: 1px solid #FECACA;
}
```

## 测试步骤

1. 模拟State保存失败（可以临时修改数据库权限）
2. 验证错误信息显示
3. 验证重试按钮显示
4. 验证下载按钮显示
5. 点击重试按钮，验证State保存成功
6. 验证workspace自动解锁
7. 点击下载按钮，验证文件下载

## 预期效果

当State保存失败时：
1. 显示详细的错误信息
2. 显示"Retry State Save"按钮（蓝色）
3. 显示"Download State Backup"按钮（绿色）
4. 显示备份文件路径
5. 点击重试后，State保存成功，workspace解锁
6. 点击下载后，获得State备份文件

## 注意事项

1. 需要导入`os`和`strings`包到controller
2. 需要确保备份目录有写权限
3. 重试成功后需要解锁workspace
4. 下载时设置正确的Content-Disposition头
