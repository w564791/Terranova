# WorkspaceDetail页面集成指南

## 概述
本文档说明如何将状态徽章和任务管理功能集成到WorkspaceDetail页面。

## 已完成的工作

### 1. 组件准备
-  WorkspaceStateBadge组件（7种状态）
-  TaskStateBadge组件（5种任务状态）
-  TypeScript类型定义
-  导入语句修复

### 2. 接口定义
```typescript
interface Workspace {
  id: number;
  name: string;
  description: string;
  state_backend: string;
  terraform_version: string;
  execution_mode: string;
  current_state: WorkspaceState;  // 新增
  is_locked: boolean;              // 新增
  created_at: string;
  updated_at: string;
}

interface Task {
  id: number;
  workspace_id: number;
  task_type: TaskType;
  status: TaskStatus;
  output: string;
  error_message: string;
  created_at: string;
  started_at: string;
  completed_at: string;
}
```

## 待完成的集成步骤

### 步骤1: 添加任务列表状态管理

在`WorkspaceDetail`组件中添加：

```typescript
const [tasks, setTasks] = useState<Task[]>([]);
const [tasksLoading, setTasksLoading] = useState(false);
const [creating, setCreating] = useState(false);
```

### 步骤2: 添加任务获取函数

```typescript
const fetchTasks = async () => {
  if (!id) return;
  
  setTasksLoading(true);
  try {
    const response = await api.get(`/workspaces/${id}/tasks`);
    setTasks(response.data.data || []);
  } catch (error) {
    const message = extractErrorMessage(error);
    showToast(message, 'error');
  } finally {
    setTasksLoading(false);
  }
};

useEffect(() => {
  if (id && workspace) {
    fetchTasks();
    // 每10秒刷新一次任务列表
    const interval = setInterval(fetchTasks, 10000);
    return () => clearInterval(interval);
  }
}, [id, workspace]);
```

### 步骤3: 添加Plan/Apply按钮处理函数

```typescript
const handleCreatePlan = async () => {
  if (!workspace || creating) return;
  
  setCreating(true);
  try {
    await api.post(`/workspaces/${workspace.id}/tasks/plan`);
    showToast('Plan任务创建成功', 'success');
    fetchTasks();
  } catch (error) {
    const message = extractErrorMessage(error);
    showToast(message, 'error');
  } finally {
    setCreating(false);
  }
};

const handleCreateApply = async () => {
  if (!workspace || creating) return;
  
  if (!window.confirm('确定要执行Apply操作吗？这将修改基础设施。')) {
    return;
  }
  
  setCreating(true);
  try {
    await api.post(`/workspaces/${workspace.id}/tasks/apply`);
    showToast('Apply任务创建成功', 'success');
    fetchTasks();
  } catch (error) {
    const message = extractErrorMessage(error);
    showToast(message, 'error');
  } finally {
    setCreating(false);
  }
};

const handleLock = async () => {
  if (!workspace) return;
  
  try {
    await api.post(`/workspaces/${workspace.id}/lock`);
    showToast('工作空间已锁定', 'success');
    // 重新获取workspace信息
    const response = await api.get(`/workspaces/${id}`);
    setWorkspace(response.data);
  } catch (error) {
    const message = extractErrorMessage(error);
    showToast(message, 'error');
  }
};

const handleUnlock = async () => {
  if (!workspace) return;
  
  try {
    await api.post(`/workspaces/${workspace.id}/unlock`);
    showToast('工作空间已解锁', 'success');
    // 重新获取workspace信息
    const response = await api.get(`/workspaces/${id}`);
    setWorkspace(response.data);
  } catch (error) {
    const message = extractErrorMessage(error);
    showToast(message, 'error');
  }
};
```

### 步骤4: 在标题区域添加状态徽章

替换`titleSection`部分：

```tsx
<div className={styles.titleSection}>
  <div className={styles.titleRow}>
    <h1 className={styles.title}>{workspace.name}</h1>
    <WorkspaceStateBadge state={workspace.current_state} size="large" />
    {workspace.is_locked && (
      <span className={styles.lockBadge}>🔒 已锁定</span>
    )}
  </div>
  <div className={styles.executionMode}>{workspace.execution_mode.toUpperCase()}</div>
</div>
```

### 步骤5: 添加任务管理卡片

在"基本信息"卡片之后添加：

```tsx
<div className={styles.card}>
  <div className={styles.cardHeader}>
    <h2 className={styles.cardTitle}>任务管理</h2>
    <div className={styles.taskActions}>
      <button 
        onClick={handleCreatePlan}
        disabled={creating || workspace.is_locked}
        className={styles.primaryButton}
      >
        {creating ? '创建中...' : '创建Plan'}
      </button>
      <button 
        onClick={handleCreateApply}
        disabled={creating || workspace.is_locked || workspace.current_state !== 'plan_done'}
        className={styles.primaryButton}
      >
        {creating ? '创建中...' : '创建Apply'}
      </button>
      {workspace.is_locked ? (
        <button onClick={handleUnlock} className={styles.secondaryButton}>
          解锁
        </button>
      ) : (
        <button onClick={handleLock} className={styles.secondaryButton}>
          锁定
        </button>
      )}
    </div>
  </div>
  
  {tasksLoading ? (
    <div className={styles.loading}>加载任务列表...</div>
  ) : tasks.length === 0 ? (
    <div className={styles.emptyState}>暂无任务</div>
  ) : (
    <div className={styles.taskList}>
      {tasks.map(task => (
        <div key={task.id} className={styles.taskItem}>
          <div className={styles.taskHeader}>
            <TaskStateBadge 
              status={task.status} 
              type={task.task_type}
              size="medium"
            />
            <span className={styles.taskTime}>
              {new Date(task.created_at).toLocaleString()}
            </span>
          </div>
          {task.error_message && (
            <div className={styles.taskError}>
              错误: {task.error_message}
            </div>
          )}
          {task.output && (
            <details className={styles.taskOutput}>
              <summary>查看输出</summary>
              <pre>{task.output}</pre>
            </details>
          )}
        </div>
      ))}
    </div>
  )}
</div>
```

### 步骤6: 添加CSS样式

在`WorkspaceDetail.module.css`中添加：

```css
.titleRow {
  display: flex;
  align-items: center;
  gap: 16px;
}

.lockBadge {
  padding: 4px 12px;
  background-color: #fff3e0;
  color: #f57c00;
  border-radius: 12px;
  font-size: 14px;
  font-weight: 500;
}

.cardHeader {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.taskActions {
  display: flex;
  gap: 12px;
}

.taskList {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.taskItem {
  padding: 16px;
  border: 1px solid #e0e0e0;
  border-radius: 8px;
  background-color: #fafafa;
}

.taskHeader {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.taskTime {
  font-size: 14px;
  color: #666;
}

.taskError {
  padding: 12px;
  background-color: #ffebee;
  color: #c62828;
  border-radius: 4px;
  margin-top: 12px;
  font-size: 14px;
}

.taskOutput {
  margin-top: 12px;
}

.taskOutput summary {
  cursor: pointer;
  color: #1976d2;
  font-weight: 500;
  padding: 8px;
  background-color: #e3f2fd;
  border-radius: 4px;
}

.taskOutput pre {
  margin-top: 8px;
  padding: 12px;
  background-color: #263238;
  color: #aed581;
  border-radius: 4px;
  overflow-x: auto;
  font-size: 13px;
  line-height: 1.5;
}

.emptyState {
  text-align: center;
  padding: 40px;
  color: #999;
}
```

## 测试步骤

1. **启动后端服务**
   ```bash
   cd backend
   go run main.go
   ```

2. **启动前端服务**
   ```bash
   cd frontend
   npm run dev
   ```

3. **测试流程**
   - 访问workspace详情页
   - 查看状态徽章显示
   - 点击"创建Plan"按钮
   - 观察任务列表更新
   - 查看任务状态变化
   - 点击"创建Apply"按钮（Plan完成后）
   - 测试锁定/解锁功能

## API响应格式

### GET /api/v1/workspaces/:id
```json
{
  "code": 200,
  "data": {
    "id": 1,
    "name": "test-workspace",
    "current_state": "created",
    "is_locked": false,
    ...
  }
}
```

### GET /api/v1/workspaces/:id/tasks
```json
{
  "code": 200,
  "data": [
    {
      "id": 1,
      "task_type": "plan",
      "status": "success",
      "output": "...",
      "created_at": "2025-10-09T15:00:00Z"
    }
  ]
}
```

## 注意事项

1. **状态同步**: 使用定时器每10秒刷新任务列表
2. **按钮禁用**: 根据workspace状态和锁定状态禁用按钮
3. **错误处理**: 所有API调用都要有错误处理
4. **用户反馈**: 使用Toast提示操作结果
5. **确认对话框**: Apply操作需要用户确认

## 下一步优化

1. **实时更新**: 使用WebSocket实现实时状态更新
2. **任务详情**: 点击任务查看完整输出
3. **State版本**: 添加State版本列表和回滚功能
4. **状态时间线**: 可视化显示状态转换历史
5. **Plan差异**: 解析并展示Plan的资源变更

---

**文档版本**: v1.0  
**最后更新**: 2025-10-09  
**作者**: AI Assistant
