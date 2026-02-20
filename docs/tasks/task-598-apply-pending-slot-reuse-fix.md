# Task 598 Apply_Pending槽位重用修复

## 问题
Task 598处于apply_pending状态，但无法执行apply，错误信息：
```
[TaskQueue] No free slot available for task 598: no free slot available in pool
```

## 根本原因

apply_pending任务在plan完成后，槽位被预留为`reserved`状态。但当用户确认apply时，`pushTaskToAgent`方法调用`FindPodWithFreeSlot`查找空闲槽位，而`FindPodWithFreeSlot`只查找`idle`状态的槽位，不会返回`reserved`状态的槽位。

**问题流程**:
```
1. Plan完成 → 槽位状态: running → reserved
2. 用户确认apply
3. pushTaskToAgent调用FindPodWithFreeSlot
4. FindPodWithFreeSlot只查找idle槽位
5. 找不到idle的Slot 0 → 报错 ❌
```

## 解决方案

在`pushTaskToAgent`中为apply_pending任务添加特殊处理：
1. 检测任务状态是否为apply_pending
2. 如果是，使用`FindPodByTaskID`查找已预留的槽位
3. 重用该槽位执行apply
4. 如果找不到预留槽位（异常情况），fallback到查找空闲Slot 0

## 实施的修复

### 文件: `backend/services/task_queue_manager.go`

**修改位置**: pushTaskToAgent方法的槽位分配部分

**修复代码**:
```go
// Special handling for apply_pending tasks: reuse the reserved slot
if task.Status == models.TaskStatusApplyPending {
    log.Printf("[TaskQueue] Task %d is apply_pending, looking for reserved slot", task.ID)
    
    // Find the reserved slot for this task
    pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(task.ID)
    if err == nil {
        // Found the reserved slot, reuse it
        selectedPodName = pod.PodName
        selectedSlotID = slotID
        selectedAgentID = pod.AgentID
        
        log.Printf("[TaskQueue] Found reserved slot %d on pod %s for apply_pending task %d (will reuse for apply)", 
            slotID, pod.PodName, task.ID)
    } else {
        // Fallback: try to find a free Slot 0
        log.Printf("[TaskQueue] Warning: apply_pending task %d has no reserved slot: %v", task.ID, err)
        // ... fallback logic
    }
}
```

## 修复后的流程

**正确流程**:
```
1. Plan完成 → 槽位状态: running → reserved
2. 用户确认apply
3. pushTaskToAgent检测到apply_pending状态
4. 调用FindPodByTaskID(task.ID)查找预留槽位
5. 找到reserved槽位 → 重用该槽位 
6. Apply在同一个Pod上执行
7. Apply完成 → 槽位状态: reserved → idle
```

## 关键特性

### 1. 槽位重用
- apply_pending任务自动重用已预留的槽位
- 确保apply在同一个Pod上执行
- 保持plan和apply的上下文一致性

### 2. Fallback机制
- 如果找不到预留槽位（异常情况）
- 自动fallback到查找空闲Slot 0
- 记录警告日志便于诊断

### 3. 详细日志
- 记录槽位查找过程
- 记录是否找到预留槽位
- 记录fallback情况

## 测试验证

### 测试场景
```
1. 提交plan_and_apply任务
2. Plan完成 → apply_pending状态
3. 验证：槽位被预留（reserved状态）
4. 用户确认apply
5. 验证：找到预留槽位
6. 验证：apply在同一Pod上执行
7. 验证：apply完成后槽位释放
```

### 预期日志
```
[TaskQueue] Task 598 is apply_pending, looking for reserved slot
[TaskQueue] Found reserved slot 0 on pod iac-agent-xxx for apply_pending task 598 (will reuse for apply)
[TaskQueue] Using pre-selected agent agent-xxx from slot allocation
[TaskQueue] Successfully pushed task 598 to agent agent-xxx (action: apply)
[TaskQueue] Task 598 allocated to pod iac-agent-xxx slot 0
```

## 相关文档

- `docs/task-598-apply-pending-no-slot-diagnosis.md` - 问题诊断
- `docs/terraform-execution-phase2-step-2.5-complete.md` - TaskQueueManager槽位集成

## 部署说明

### 编译和部署
```bash
cd backend
go build -o iac-platform .
# 重启服务器
```

### 验证修复
1. 重启服务器后，task 598应该能够执行
2. 查看日志确认找到了预留槽位
3. 验证apply在同一Pod上执行

## 总结

这个修复解决了apply_pending任务无法重用预留槽位的问题，确保：
-  apply_pending任务能够找到并重用预留的槽位
-  apply在同一个Pod上执行（保持上下文）
-  有fallback机制处理异常情况
-  详细的日志便于诊断问题

**修复完成时间**: 2025-11-08 15:54
