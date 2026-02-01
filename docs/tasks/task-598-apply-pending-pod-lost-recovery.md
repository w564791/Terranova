# Apply_Pending任务Pod丢失后的恢复逻辑

## 需求

**场景**: Apply_pending任务的预留slot所在的Pod被删除/清理

**期望行为**:
1. 检测到预留Pod丢失
2. 自动重新执行plan阶段
3. Plan完成后再次进入apply_pending状态
4. 等待用户确认

**原因**: 
- Plan的上下文（工作目录、状态文件）在Pod上
- Pod丢失后，需要重新建立上下文
- 不能直接执行apply，因为缺少plan上下文

## 当前实现分析

### pushTaskToAgent的Fallback逻辑

**当前代码**:
```go
if task.Status == models.TaskStatusApplyPending {
    // 查找预留槽位
    pod, slotID, err := FindPodByTaskID(task.ID)
    if err == nil {
        // 找到预留槽位 → 重用，执行apply
    } else {
        // 没找到预留槽位 → Fallback
        pod, slotID, err := FindPodWithFreeSlot(...)
        if err == nil {
            // 找到空闲Slot 0 → 在新Pod上执行apply
        } else {
            // 没有空闲槽位 → 重试
        }
    }
}
```

**问题**: 
- ❌ Fallback时直接执行apply
- ❌ 没有重新执行plan
- ❌ 新Pod上没有plan上下文

### 正确的逻辑应该是

**方案1: 重置任务状态**
```go
if task.Status == models.TaskStatusApplyPending {
    pod, slotID, err := FindPodByTaskID(task.ID)
    if err == nil {
        // 找到预留槽位 → 重用，执行apply
    } else {
        // 没找到预留槽位 → Pod已丢失
        log.Printf("[TaskQueue] Apply_pending task %d lost its reserved Pod, resetting to pending to re-execute plan", task.ID)
        
        // 重置任务状态为pending
        task.Status = models.TaskStatusPending
        task.Stage = "pending"
        task.AgentID = nil
        task.StartedAt = nil
        s.db.Save(task)
        
        // 触发重新执行
        m.TryExecuteNextTask(task.WorkspaceID)
        return nil
    }
}
```

**方案2: 检查工作目录**
```go
if task.Status == models.TaskStatusApplyPending {
    pod, slotID, err := FindPodByTaskID(task.ID)
    if err == nil {
        // 找到预留槽位 → 重用
    } else {
        // 没找到预留槽位 → 检查plan文件是否存在
        planFile := fmt.Sprintf("/tmp/iac-platform/workspaces/%s/%d/terraform.tfplan", task.WorkspaceID, task.ID)
        if _, err := os.Stat(planFile); err != nil {
            // Plan文件不存在 → 重置为pending重新执行plan
            log.Printf("[TaskQueue] Apply_pending task %d has no plan file, resetting to pending", task.ID)
            task.Status = models.TaskStatusPending
            task.Stage = "pending"
            s.db.Save(task)
            m.TryExecuteNextTask(task.WorkspaceID)
            return nil
        } else {
            // Plan文件存在 → 可以在新Pod上执行apply
            // Fallback逻辑...
        }
    }
}
```

## 推荐方案

**方案1更简单可靠**:
- 不依赖文件系统状态
- 逻辑清晰
- 确保plan上下文正确

## 实施步骤

### 修改pushTaskToAgent方法

**文件**: `backend/services/task_queue_manager.go`

**修改位置**: apply_pending任务的fallback逻辑

**新逻辑**:
```go
if task.Status == models.TaskStatusApplyPending {
    log.Printf("[TaskQueue] Task %d is apply_pending, looking for reserved slot", task.ID)
    
    // Find the reserved slot for this task
    pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(task.ID)
    if err == nil {
        // Found the reserved slot, reuse it for apply
        selectedPodName = pod.PodName
        selectedSlotID = slotID
        selectedAgentID = pod.AgentID
        
        log.Printf("[TaskQueue] Found reserved slot %d on pod %s for apply_pending task %d (will reuse for apply)", 
            slotID, pod.PodName, task.ID)
    } else {
        // No reserved slot found - Pod was deleted
        log.Printf("[TaskQueue] Apply_pending task %d has no reserved slot (Pod was deleted)", task.ID)
        log.Printf("[TaskQueue] Resetting task %d to pending to re-execute plan phase", task.ID)
        
        // Reset task to pending to re-execute from plan
        task.Status = models.TaskStatusPending
        task.Stage = "pending"
        task.AgentID = nil
        task.StartedAt = nil
        task.ErrorMessage = ""
        m.db.Save(task)
        
        // Trigger re-execution
        log.Printf("[TaskQueue] Triggering re-execution of task %d from plan phase", task.ID)
        go m.TryExecuteNextTask(task.WorkspaceID)
        
        return nil
    }
}
```

## 完整的恢复流程

### 场景: Pod被删除后的恢复

```
1. Task 598: plan_and_apply任务
2. Plan完成 → apply_pending状态，槽位reserved
3. Pod被删除（手动/配置更新/清理）
4. 用户确认apply
5. pushTaskToAgent:
   - 查找预留槽位 → 找不到
   - 检测到Pod丢失
   - 重置任务为pending状态 
   - 触发重新执行
6. TryExecuteNextTask:
   - 获取pending任务
   - 分配新槽位
   - 执行plan 
7. Plan完成 → 再次进入apply_pending 
8. 等待用户确认 
9. 用户确认 → 执行apply
```

## 当前实现的Gap

### 问题
当前的fallback逻辑会尝试在新Pod上直接执行apply，这是**不正确的**，因为：
1. ❌ 新Pod上没有plan上下文
2. ❌ 工作目录需要重新init
3. ❌ 可能导致apply失败或不一致

### 需要实施的修复
1. 检测到预留Pod丢失时，重置任务为pending
2. 重新执行plan阶段
3. Plan完成后再次进入apply_pending
4. 等待用户确认

## 实施优先级

**优先级**: 高

**原因**:
- 这是正确性问题，不是性能问题
- 影响apply_pending任务的可靠性
- 需要尽快修复

## 预计工作量

- 代码修改: 0.2天
- 测试验证: 0.2天
- 总计: 0.4天

## 相关文档

- `docs/task-598-apply-pending-slot-reuse-fix.md` - 槽位重用修复
- `docs/task-598-server-restart-pod-deletion-issue.md` - Server重启问题

## 下一步行动

1. 修改pushTaskToAgent的fallback逻辑
2. 添加任务状态重置
3. 测试Pod丢失后的恢复流程
4. 验证plan重新执行并进入apply_pending
