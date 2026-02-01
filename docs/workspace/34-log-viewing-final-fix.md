# 日志查看问题 - 最终修复方案

## 问题描述

从用户截图看到：
1. plan_and_apply任务执行Apply后失败
2. 错误："apply succeeded but state save failed"
3. 显示Apply阶段的Tab（Restoring Plan, Applying等）
4. 所有Tab都是灰色，无法查看日志

## 根本原因分析

### 问题1: 日志API返回问题
```tsx
// StageLogViewer请求
GET /api/v1/tasks/${taskId}/logs?type=apply&format=text

// 但对于plan_and_apply任务：
// - Plan日志保存在 task.plan_output
// - Apply日志保存在 task.apply_output
// - API需要根据type返回对应的日志
```

### 问题2: plan_and_apply任务的日志存储
```go
// ExecutePlan完成后
task.PlanOutput = planOutput  // Plan日志
task.Status = TaskStatusPlanCompleted

// ExecuteApply完成后
task.ApplyOutput = applyOutput  // Apply日志
task.Status = TaskStatusSuccess/Failed
```

### 问题3: 日志API实现
需要检查 `/api/v1/tasks/:task_id/logs` 的实现，确保：
1. 支持type参数（plan/apply）
2. 对于plan_and_apply任务，正确返回对应的日志
3. 返回格式正确

## 解决方案

### 方案1: 修改StageLogViewer的日志获取逻辑

对于plan_and_apply任务，需要特殊处理：

```tsx
const fetchAndParseLogs = async () => {
  try {
    // 获取任务详情
    const pathParts = window.location.pathname.split('/');
    const workspaceId = pathParts[2];
    const taskData: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}`);
    const task = taskData.task || taskData;
    
    let logText = '';
    
    // 根据taskType获取对应的日志
    if (taskType === 'plan') {
      logText = task.plan_output || '';
    } else if (taskType === 'apply') {
      logText = task.apply_output || '';
    }
    
    // 如果没有日志，尝试从API获取
    if (!logText) {
      const response = await fetch(
        `http://localhost:8080/api/v1/tasks/${taskId}/logs?type=${taskType}&format=text`
      );
      logText = await response.text();
    }
    
    const parsedStages = parseStages(logText);
    setStages(parsedStages);
  } catch (err: any) {
    setError(err.message || 'Failed to fetch logs');
  } finally {
    setLoading(false);
  }
};
```

### 方案2: 直接从task对象获取日志

更简单的方案：直接从task对象获取plan_output或apply_output

```tsx
const fetchAndParseLogs = async () => {
  try {
    const pathParts = window.location.pathname.split('/');
    const workspaceId = pathParts[2];
    
    // 获取任务详情
    const data: any = await api.get(`/workspaces/${workspaceId}/tasks/${taskId}`);
    const task = data.task || data;
    
    // 根据taskType获取对应的日志
    let logText = '';
    if (taskType === 'plan') {
      logText = task.plan_output || '';
    } else if (taskType === 'apply') {
      logText = task.apply_output || '';
    }
    
    console.log('[StageLogViewer] Task type:', taskType, 'Log length:', logText.length);
    
    if (!logText) {
      setError('No logs available for this task');
      return;
    }
    
    const parsedStages = parseStages(logText);
    console.log('[StageLogViewer] Parsed stages:', parsedStages.length);
    setStages(parsedStages);
  } catch (err: any) {
    setError(err.message || 'Failed to fetch logs');
  } finally {
    setLoading(false);
  }
};
```

## 推荐实施方案2

方案2更简单、更可靠：
1. 直接从task对象获取日志
2. 不依赖额外的API
3. 对于plan_and_apply任务，plan_output和apply_output都已保存

## 实施步骤

1. 修改StageLogViewer的fetchAndParseLogs方法
2. 直接从task对象获取plan_output或apply_output
3. 添加调试日志
4. 测试验证

## 预期效果

修复后：
1. Plan阶段的日志可以正常查看（Fetching, Init, Planning, Saving Plan）
2. Apply阶段的日志可以正常查看（Fetching, Init, Restoring Plan, Applying, Saving State）
3. 所有Tab都可以点击
4. 日志内容正确显示

## 测试验证

1. 创建plan_and_apply任务
2. Plan完成后查看日志（应该能看到Fetching, Init, Planning, Saving Plan）
3. Confirm Apply
4. Apply完成后查看日志（应该能看到Fetching, Init, Restoring Plan, Applying, Saving State）
5. 验证所有Tab都可以点击
