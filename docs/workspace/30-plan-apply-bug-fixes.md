# Plan+Apply流程 - Bug修复

## Bug报告日期
2025-10-12

## 发现的问题

### Bug 1: Apply是否强制从数据库获取Plan数据 
**状态**: 已确认正确实现

**实现验证**:
```go
// ConfirmApply中的实现
task.PlanTaskID = &task.ID  // 指向自己
c.db.Save(&task)

// ExecuteApply中的实现
if task.PlanTaskID == nil {
    return error("no plan task associated")
}

var planTask models.WorkspaceTask
s.db.First(&planTask, *task.PlanTaskID)  // 从数据库读取

if len(planTask.PlanData) == 0 {
    return error("plan data is empty")
}

// 恢复Plan文件
os.WriteFile(planFile, planTask.PlanData, 0644)
```

**结论**:  Apply确实强制从数据库读取plan_data，实现正确

### Bug 2: 日志Tab全部灰色 ❌
**问题描述**: Confirm Apply后，所有日志Tab都显示为灰色，无法查看日志

**根本原因分析**:
1. Apply阶段的日志可能没有正确保存到task_logs表
2. SmartLogViewer可能没有正确处理plan_and_apply任务的日志
3. Apply阶段的stage名称可能与日志系统不匹配

**需要检查**:
- ExecuteApply是否正确保存日志到task_logs表
- SmartLogViewer是否支持Apply阶段的stage名称
- 日志的phase字段是否正确

**修复方案**:
1. 确认ExecuteApply保存日志时使用正确的phase
2. 确认SmartLogViewer支持所有stage
3. 添加调试日志查看日志保存情况

### Bug 3: 无法取消运行中的任务 ❌
**问题描述**: 运行中的任务没有取消按钮

**根本原因**: TaskDetail.tsx没有显示取消按钮

**修复方案**: 在TaskDetail.tsx中添加取消按钮

## 修复计划

### 修复1: 确认Apply日志保存 ⏳
检查ExecuteApply方法，确保：
1. Apply阶段的日志正确保存到task_logs表
2. phase字段设置为"apply"
3. 所有阶段的日志都被保存

### 修复2: 添加取消按钮 ⏳
在TaskDetail.tsx中：
1. 添加取消按钮（当status为running时显示）
2. 实现handleCancelTask方法
3. 调用CancelTask API

### 修复3: 检查日志查看器 ⏳
检查SmartLogViewer：
1. 是否支持Apply阶段的所有stage
2. 是否正确处理plan_and_apply任务
3. WebSocket连接是否正常

## 详细修复步骤

### Step 1: 添加取消按钮到TaskDetail

**位置**: `frontend/src/pages/TaskDetail.tsx`

```tsx
// 在taskTitleRight中添加
<div className={styles.taskTitleRight}>
  {/* 取消按钮 - 运行中的任务 */}
  {(task.status === 'running' || task.status === 'apply_pending') && (
    <button
      className={styles.cancelButton}
      onClick={handleCancelTask}
    >
      ✗ Cancel Task
    </button>
  )}
  
  {/* Confirm Apply按钮 */}
  {task.status === 'plan_completed' && task.task_type === 'plan_and_apply' && (
    <button
      className={styles.confirmApplyButton}
      onClick={() => setShowConfirmApplyDialog(true)}
    >
      ✓ Confirm Apply
    </button>
  )}
</div>

// 添加handleCancelTask方法
const handleCancelTask = async () => {
  if (!confirm('确定要取消此任务吗？')) {
    return;
  }

  try {
    await api.post(`/workspaces/${workspaceId}/tasks/${taskId}/cancel`);
    alert('任务已取消');
    fetchTask(); // 刷新任务状态
  } catch (err: any) {
    const message = err.response?.data?.error || err.message || 'Failed to cancel task';
    alert(`取消任务失败: ${message}`);
  }
};
```

### Step 2: 添加取消按钮样式

**位置**: `frontend/src/pages/TaskDetail.module.css`

```css
.cancelButton {
  background: var(--color-red-600);
  color: white;
  border: none;
  padding: 10px 20px;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  display: flex;
  align-items: center;
  gap: 6px;
}

.cancelButton:hover {
  background: var(--color-red-700);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(220, 38, 38, 0.3);
}
```

### Step 3: 检查日志保存

需要确认ExecuteApply中的日志保存：
```go
// 在ExecuteApply结束时
s.saveTaskLog(task.ID, "apply", applyOutput, "info")
```

### Step 4: 检查SmartLogViewer

需要确认SmartLogViewer支持的stage：
- fetching
- init
- planning
- saving_plan
- restoring_plan
- applying
- saving_state

## 优先级

1. **高优先级**: 添加取消按钮（影响用户体验）
2. **中优先级**: 修复日志Tab灰色问题（影响调试）
3. **低优先级**: 优化错误提示

## 测试计划

### 测试1: 取消功能
- [ ] 创建plan_and_apply任务
- [ ] 在Plan阶段点击取消
- [ ] 验证任务状态变为cancelled
- [ ] 在Apply阶段点击取消
- [ ] 验证任务状态变为cancelled

### 测试2: 日志查看
- [ ] 创建plan_and_apply任务
- [ ] Plan阶段查看实时日志
- [ ] Confirm Apply
- [ ] Apply阶段查看实时日志
- [ ] 完成后查看历史日志
- [ ] 验证所有Tab都可点击

### 测试3: Apply数据来源
- [ ] 创建plan_and_apply任务
- [ ] Plan完成后修改资源
- [ ] Confirm Apply
- [ ] 验证返回409错误
- [ ] 不修改资源
- [ ] Confirm Apply
- [ ] 验证Apply使用正确的Plan数据

## 下一步行动

1. 立即添加取消按钮
2. 调试日志Tab灰色问题
3. 完善错误处理
4. 进行完整测试
