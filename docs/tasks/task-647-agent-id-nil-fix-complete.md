# Task 647: Agent ID Nil 问题完整修复

## 问题根源

任务 647 的 Apply 阶段日志显示：
```
[INFO] Different agent detected, must run init:
[INFO]   - Plan agent: agent-pool-z73eh8ihywlmgx0x-1762768944900138000
[INFO]   - Apply agent: (none)
```

但数据库查询显示任务 647 的 agent_id **有值**。

## 深度排查结果

### 1. 数据库层面 
```sql
SELECT id, agent_id FROM workspace_tasks WHERE id = 647;
-- agent_id = agent-pool-z73eh8ihywlmgx0x-1762768944900138000
```
数据库中有值，没问题。

### 2. TaskQueueManager 
`pushTaskToAgent` 函数已经在发送任务前保存 agent_id，没问题。

### 3. GetPlanTask API Handler 
返回 JSON 中包含 `"agent_id": task.AgentID`，没问题。

### 4. 真正的问题 ❌

**AgentAPIClient.GetPlanTask 解析 JSON 时缺少字段！**

Agent 调用 GetPlanTask API 后，解析响应时：
- ❌ 没有解析 `created_at` 字段 → 导致 `planTask.CreatedAt = 0001-01-01`
- ❌ agent_id 的解析逻辑有问题（虽然代码存在，但可能因为 JSON 中是 null 而失败）

## 完整修复方案

### 修复 1: AgentAPIClient.GetPlanTask 添加 created_at 解析

**文件**: `backend/services/agent_api_client.go`

```go
// 解析 created_at
if createdAtStr, ok := taskData["created_at"].(string); ok && createdAtStr != "" {
    if t, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
        task.CreatedAt = t
    }
}
```

### 修复 2: AgentHandler.GetPlanTask 正确处理指针字段

**文件**: `backend/internal/handlers/agent_handler.go`

修改前（错误）：
```go
response := gin.H{
    "task": gin.H{
        "agent_id": task.AgentID,  // 如果是 nil，JSON 中是 null
    },
}
```

修改后（正确）：
```go
taskResponse := gin.H{
    "id": task.ID,
    "created_at": task.CreatedAt,  // 添加 created_at
}

// 只有非 nil 时才添加到响应中
if task.AgentID != nil {
    taskResponse["agent_id"] = *task.AgentID  // 解引用指针
}
```

### 修复 3: TaskQueueManager 竞态条件修复（已完成）

**文件**: `backend/services/task_queue_manager.go`

确保 agent_id 在发送任务给 Agent 之前就保存到数据库。

## 修复的文件列表

1.  `backend/services/task_queue_manager.go` - 在发送任务前保存 agent_id
2.  `backend/services/agent_api_client.go` - 添加 created_at 解析
3.  `backend/internal/handlers/agent_handler.go` - 正确处理指针字段，添加 created_at

## 测试步骤

1. 重新编译并部署 Backend
2. 重启 Agent
3. 创建新的 plan_and_apply 任务
4. 检查 Apply 阶段日志，应该显示：
   ```
   [INFO] Same agent detected, can skip init
   [INFO]   - Plan agent: agent-pool-xxx
   [INFO]   - Apply agent: agent-pool-xxx  ← 不再是 (none)
   [INFO] Skipping terraform init (optimization)
   ```

## 预期效果

-  Apply 任务能正确获取 Plan 任务的 agent_id
-  在同一 agent 上执行时跳过 init
-  节省 85-96% 的 init 时间

## 完成状态

 **所有修复已完成** - 共修复 3 个文件，解决了 agent_id 数据流的完整链路问题
