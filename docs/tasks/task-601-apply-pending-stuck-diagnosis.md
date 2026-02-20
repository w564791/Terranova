# 任务601 Apply Pending卡住问题诊断报告

## 问题概述

**任务ID:** 601  
**Workspace:** ws-mb7m9ii5ey  
**问题:** 用户点击"Confirm Apply"后,任务停留在`apply_pending`状态,没有继续执行apply操作

## 数据库状态

```
id:              601
workspace_id:    ws-mb7m9ii5ey
task_type:       plan_and_apply
status:          apply_pending
stage:           apply_pending
execution_mode:  k8s
agent_id:        agent-pool-z73eh8ihywlmgx0x-1762590434899879000
k8s_pod_name:    (空) ← 关键问题
plan_task_id:    601
created_at:      2025-11-08 16:15:58
updated_at:      2025-11-08 16:32:13
started_at:      2025-11-08 16:29:45
```

## 问题根源分析

### 1. K8s模式下的Pod槽位管理机制

在K8s执行模式下,系统使用Pod槽位管理来优化资源使用:

- **Plan阶段:** 任务分配到一个Pod的槽位执行plan
- **Plan完成后:** 槽位被"保留"(reserved),等待用户confirm apply
- **Apply阶段:** 复用同一个Pod槽位执行apply

### 2. 问题发生场景

当用户点击"Confirm Apply"时,`ConfirmApply`函数会:

1. 更新任务状态为`apply_pending`
2. 调用`queueManager.TryExecuteNextTask()`触发执行

然后`pushTaskToAgent`函数会尝试查找保留的Pod槽位:

```go
if task.Status == models.TaskStatusApplyPending {
    // 查找为这个任务保留的Pod槽位
    pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(task.ID)
    if err == nil {
        // 找到保留的槽位,复用它执行apply
        selectedPodName = pod.PodName
        selectedSlotID = slotID
        selectedAgentID = pod.AgentID
    } else {
        // 没找到保留的槽位 - Pod被删除了
        // 将任务重置为pending,重新执行plan阶段
        task.Status = models.TaskStatusPending
        task.Stage = "pending"
        task.AgentID = nil
        task.StartedAt = nil
        m.db.Save(task)
        
        // 触发重新执行
        go m.TryExecuteNextTask(task.WorkspaceID)
        return nil
    }
}
```

### 3. 任务601的具体情况

**问题:** 任务601处于`apply_pending`状态,但`k8s_pod_name`为空,说明:

1. Plan阶段执行完成后,用户点击了"Confirm Apply"
2. 但在用户confirm之前或之后,执行plan的Pod被删除了(可能原因):
   - K8s自动缩容(HPA scale down)
   - Pod崩溃或被手动删除
   - 节点故障导致Pod丢失

3. 当系统尝试执行apply时,找不到保留的Pod槽位
4. 按照代码逻辑,应该将任务重置为`pending`状态,重新执行plan
5. **但任务仍然停留在`apply_pending`状态**

### 4. 可能的原因

任务没有被重置的可能原因:

1. **重置逻辑没有被触发:** `TryExecuteNextTask`可能没有被成功调用
2. **重置后没有触发重新执行:** goroutine可能失败或被阻塞
3. **数据库更新失败:** 重置操作可能因为某种原因失败
4. **并发问题:** 多个goroutine同时操作导致状态不一致

## 解决方案

### 立即解决方案(针对任务601)

**方法1: 在浏览器中重新触发**

1. 打开任务详情页面: http://localhost:5173/workspaces/ws-mb7m9ii5ey/tasks/601
2. 重新点击"Confirm & Apply"按钮
3. 系统会重新尝试执行,如果Pod不存在,会自动重置任务为pending

**方法2: 使用浏览器Console**

```javascript
fetch('http://localhost:8080/api/v1/workspaces/ws-mb7m9ii5ey/tasks/601/confirm-apply', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer ' + localStorage.getItem('token')
  },
  body: JSON.stringify({
    apply_description: 'Manual retry after Pod deletion'
  })
}).then(r => r.json()).then(console.log)
```

**方法3: 手动重置任务状态(如果上述方法失败)**

```sql
-- 将任务重置为pending状态
UPDATE workspace_tasks 
SET status = 'pending',
    stage = 'pending',
    agent_id = NULL,
    started_at = NULL,
    error_message = 'Reset due to Pod deletion'
WHERE id = 601;
```

然后重启后端服务,系统会自动恢复pending任务。

### 长期解决方案

#### 1. 增强Pod槽位保留机制

**问题:** 当前的槽位保留机制不够健壮,Pod删除后任务会卡住

**建议改进:**

```go
// 在pushTaskToAgent中增加重试和降级逻辑
if task.Status == models.TaskStatusApplyPending {
    pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(task.ID)
    if err != nil {
        // Pod被删除,记录日志
        log.Printf("[TaskQueue] Apply_pending task %d has no reserved slot (Pod deleted)", task.ID)
        
        // 选项A: 重置为pending,重新执行plan(当前实现)
        // 选项B: 尝试分配新的Pod槽位,直接执行apply(需要验证plan数据仍然有效)
        
        // 增加重试计数,避免无限循环
        if task.RetryCount >= 3 {
            task.Status = models.TaskStatusFailed
            task.ErrorMessage = "Pod deleted, exceeded max retries"
            m.db.Save(task)
            return fmt.Errorf("max retries exceeded")
        }
        
        // 重置任务
        task.Status = models.TaskStatusPending
        task.Stage = "pending"
        task.AgentID = nil
        task.StartedAt = nil
        task.RetryCount++
        task.ErrorMessage = fmt.Sprintf("Pod deleted, resetting to pending (retry %d/3)", task.RetryCount)
        
        if err := m.db.Save(task).Error; err != nil {
            log.Printf("[ERROR] Failed to reset task %d: %v", task.ID, err)
            return err
        }
        
        log.Printf("[TaskQueue] Task %d reset to pending, will re-execute plan", task.ID)
        
        // 确保触发重新执行
        time.Sleep(2 * time.Second) // 短暂延迟避免立即重试
        return m.TryExecuteNextTask(task.WorkspaceID)
    }
    // ... 继续正常流程
}
```

#### 2. 增加apply_pending任务监控

在`StartPendingTasksMonitor`中增加对`apply_pending`任务的监控:

```go
func (m *TaskQueueManager) checkAndRetryPendingTasks() {
    // 检查pending任务
    var pendingWorkspaces []string
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", models.TaskStatusPending).
        Distinct("workspace_id").
        Pluck("workspace_id", &pendingWorkspaces)
    
    // 检查apply_pending任务(新增)
    var applyPendingWorkspaces []string
    m.db.Model(&models.WorkspaceTask{}).
        Where("status = ?", models.TaskStatusApplyPending).
        Distinct("workspace_id").
        Pluck("workspace_id", &applyPendingWorkspaces)
    
    // 合并并去重
    allWorkspaces := append(pendingWorkspaces, applyPendingWorkspaces...)
    uniqueWorkspaces := make(map[string]bool)
    for _, ws := range allWorkspaces {
        uniqueWorkspaces[ws] = true
    }
    
    // 触发执行
    for ws := range uniqueWorkspaces {
        go m.TryExecuteNextTask(ws)
    }
}
```

#### 3. 前端增加状态提示

在前端任务详情页面,当检测到`apply_pending`状态但长时间没有进展时:

```typescript
// 检测任务是否卡住
if (task.status === 'apply_pending' && task.updated_at) {
    const minutesSinceUpdate = (Date.now() - new Date(task.updated_at).getTime()) / 60000;
    if (minutesSinceUpdate > 5) {
        // 显示警告和重试按钮
        showWarning('Task may be stuck. Click "Retry Apply" to continue.');
    }
}
```

## 预防措施

1. **增强Pod生命周期管理:**
   - 为保留槽位的Pod设置更长的空闲超时
   - 在HPA缩容时优先删除没有保留槽位的Pod

2. **增加任务状态监控:**
   - 定期检查`apply_pending`任务的健康状态
   - 自动重试卡住的任务

3. **改进用户体验:**
   - 在UI上明确提示用户尽快confirm apply
   - 显示Pod状态和槽位信息
   - 提供手动重试按钮

## 总结

任务601卡在`apply_pending`状态的根本原因是:

1. Plan执行完成后,Pod被删除(自动缩容或其他原因)
2. 用户confirm apply时,系统找不到保留的Pod槽位
3. 任务重置逻辑可能没有正确执行或触发

**立即解决:** 在浏览器中重新点击"Confirm & Apply"按钮

**长期改进:** 增强Pod槽位管理机制,增加监控和自动恢复能力
