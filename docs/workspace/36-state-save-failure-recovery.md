# State保存失败恢复机制

## 需求
当State保存失败时，提供重试和下载按钮，让用户可以：
1. 手动重试State保存
2. 下载备份的State文件

## 当前实现

### 错误信息
```
apply succeeded but state save failed: state save failed, workspace locked, backup at: /var/backup/states/ws_10_task_63_1760251780.tfstate
```

### 问题
- 用户只能看到错误信息
- 无法手动重试
- 无法下载备份文件
- 需要手动SSH到服务器获取备份

## 解决方案

### 方案1: 在错误信息中添加操作按钮

在TaskDetail页面，当检测到State保存失败时，显示：
1. 重试按钮 - 调用API重新保存State
2. 下载按钮 - 下载备份的State文件
3. 备份路径 - 显示备份文件位置

### 方案2: 添加State恢复API

#### API 1: 重试State保存
```
POST /api/v1/workspaces/:id/tasks/:task_id/retry-state-save
```

功能：
- 从备份文件读取State
- 重新尝试保存到数据库
- 如果成功，解锁workspace

#### API 2: 下载State备份
```
GET /api/v1/workspaces/:id/tasks/:task_id/state-backup
```

功能：
- 返回备份的State文件
- 支持下载

## 实施步骤

### Step 1: 后端添加API

**文件**: `backend/controllers/workspace_task_controller.go`

```go
// RetryStateSave 重试State保存
// POST /api/v1/workspaces/:id/tasks/:task_id/retry-state-save
func (c *WorkspaceTaskController) RetryStateSave(ctx *gin.Context) {
    taskID, _ := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
    
    var task models.WorkspaceTask
    c.db.First(&task, taskID)
    
    // 检查是否是State保存失败的任务
    if !strings.Contains(task.ErrorMessage, "state save failed") {
        ctx.JSON(400, gin.H{"error": "Task is not in state save failed status"})
        return
    }
    
    // 从错误信息中提取备份路径
    backupPath := extractBackupPath(task.ErrorMessage)
    
    // 读取备份文件
    stateData, err := os.ReadFile(backupPath)
    if err != nil {
        ctx.JSON(500, gin.H{"error": "Failed to read backup file"})
        return
    }
    
    // 重新保存到数据库
    var workspace models.Workspace
    c.db.First(&workspace, task.WorkspaceID)
    
    if err := c.executor.SaveStateToDatabase(&workspace, &task, stateData); err != nil {
        ctx.JSON(500, gin.H{"error": "Failed to save state"})
        return
    }
    
    // 更新任务状态
    task.Status = models.TaskStatusSuccess
    task.ErrorMessage = ""
    c.db.Save(&task)
    
    // 解锁workspace
    workspace.IsLocked = false
    c.db.Save(&workspace)
    
    ctx.JSON(200, gin.H{"message": "State saved successfully"})
}

// DownloadStateBackup 下载State备份
// GET /api/v1/workspaces/:id/tasks/:task_id/state-backup
func (c *WorkspaceTaskController) DownloadStateBackup(ctx *gin.Context) {
    taskID, _ := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
    
    var task models.WorkspaceTask
    c.db.First(&task, taskID)
    
    // 从错误信息中提取备份路径
    backupPath := extractBackupPath(task.ErrorMessage)
    
    // 检查文件是否存在
    if _, err := os.Stat(backupPath); os.IsNotExist(err) {
        ctx.JSON(404, gin.H{"error": "Backup file not found"})
        return
    }
    
    // 返回文件
    ctx.File(backupPath)
}

func extractBackupPath(errorMessage string) string {
    // 从错误信息中提取备份路径
    // "backup at: /var/backup/states/ws_10_task_63_1760251780.tfstate"
    parts := strings.Split(errorMessage, "backup at: ")
    if len(parts) > 1 {
        return strings.TrimSpace(parts[1])
    }
    return ""
}
```

### Step 2: 添加路由

**文件**: `backend/internal/router/router.go`

```go
workspaces.POST("/:id/tasks/:task_id/retry-state-save", taskController.RetryStateSave)
workspaces.GET("/:id/tasks/:task_id/state-backup", taskController.DownloadStateBackup)
```

### Step 3: 前端添加UI

**文件**: `frontend/src/pages/TaskDetail.tsx`

```tsx
// 检测State保存失败
const isStateSaveFailure = task.error_message?.includes('state save failed');
const backupPath = extractBackupPath(task.error_message);

// 在错误卡片中添加操作按钮
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

// 处理方法
const handleRetryStateSave = async () => {
  try {
    await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/retry-state-save`);
    alert('State保存成功，workspace已解锁');
    fetchTask();
  } catch (err: any) {
    alert('重试失败: ' + err.message);
  }
};

const handleDownloadStateBackup = () => {
  window.open(
    `http://localhost:8080/api/v1/workspaces/${workspaceId}/tasks/${taskId}/state-backup`,
    '_blank'
  );
};

function extractBackupPath(errorMessage?: string): string | null {
  if (!errorMessage) return null;
  const match = errorMessage.match(/backup at: (.+)$/);
  return match ? match[1] : null;
}
```

### Step 4: 添加样式

**文件**: `frontend/src/pages/TaskDetail.module.css`

```css
.errorActions {
  margin-top: 16px;
  display: flex;
  gap: 12px;
  align-items: center;
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
}

.retryButton:hover {
  background: var(--color-blue-700);
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
}

.downloadButton:hover {
  background: var(--color-green-700);
}

.backupPath {
  font-size: 12px;
  color: var(--color-gray-600);
}

.backupPath code {
  background: var(--color-gray-100);
  padding: 2px 6px;
  border-radius: 3px;
  font-family: var(--font-mono);
}
```

## 预期效果

当State保存失败时，用户可以：
1. 点击"Retry State Save"按钮重新保存
2. 点击"Download State Backup"按钮下载备份文件
3. 看到备份文件的完整路径
4. 重试成功后，workspace自动解锁

## 优先级

这是一个重要的用户体验改进，建议优先实施。

## 测试计划

1. 模拟State保存失败
2. 验证错误信息显示
3. 验证重试按钮功能
4. 验证下载按钮功能
5. 验证重试成功后workspace解锁
