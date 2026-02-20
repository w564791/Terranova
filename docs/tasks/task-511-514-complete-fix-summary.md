# Task 511 & 514 Complete Fix Summary

## 修复的问题

### 问题 1: 任务 511 一直 Pending
- **现象**：任务创建后一直处于 pending 状态，无法被调度
- **根因**：`TryExecuteNextTask` 调用失败后没有重试机制

### 问题 2: 任务 514 Agent 断开后仍显示 Running
- **现象**：Agent 退出后，任务仍然显示 running 状态，且 agent_id 为空
- **根因**：
  1. `task.AgentID` 没有被设置，无法追踪任务在哪个 agent 上
  2. Agent 断开连接时没有清理正在运行的任务
  3. `WorkspaceTask.AgentID` 类型不匹配（*uint vs string）

## 完整修复方案

### 1. 添加任务调度重试机制

**文件**: `backend/controllers/workspace_task_controller.go`

在 `CreatePlanTask` 和 `ConfirmApply` 中添加重试逻辑：
- 最多重试 3 次
- 指数退避：1s, 2s, 4s
- 记录详细的重试日志
- 最后一次失败时记录 CRITICAL 级别日志

### 2. 修复 Agent ID 类型和赋值

**文件**: `backend/internal/models/workspace.go`

将 `WorkspaceTask.AgentID` 类型从 `*uint` 改为 `*string`：
```go
AgentID *string `json:"agent_id" gorm:"type:varchar(50);index"` // Agent的语义化ID
```

**文件**: `backend/services/task_queue_manager.go`

在 `pushTaskToAgent` 函数中添加：
```go
task.AgentID = &selectedAgent.AgentID // 设置 agent_id
```

**数据库迁移**: `scripts/alter_workspace_tasks_agent_id_to_varchar.sql`

将数据库字段从 `integer` 改为 `varchar(50)`。

### 3. Agent 断开连接时清理任务

**文件**: `backend/internal/handlers/agent_cc_handler_raw.go`

添加 `cleanupAgentTasks` 函数，在 agent 断开连接时：
1. 查找该 agent 正在运行的所有任务
2. **保存任务的缓冲日志到数据库**（`plan_output` 或 `apply_output`）
3. 将任务标记为 failed
4. 关闭 output stream

**关键代码**：
```go
func (h *RawAgentCCHandler) cleanupAgentTasks(agentID string) {
    // 查找正在运行的任务
    var runningTasks []models.WorkspaceTask
    h.db.Where("agent_id = ? AND status = ?", agentID, models.TaskStatusRunning).
        Find(&runningTasks)
    
    for _, task := range runningTasks {
        //  保存日志（确保在任何状态都能回传日志）
        if h.streamManager != nil {
            stream := h.streamManager.Get(task.ID)
            if stream != nil {
                bufferedLogs := stream.GetBufferedLogs()
                if bufferedLogs != "" {
                    // 根据任务类型保存
                    if task.TaskType == models.TaskTypePlan || task.TaskType == models.TaskTypePlanAndApply {
                        task.PlanOutput = bufferedLogs
                    } else if task.TaskType == models.TaskTypeApply {
                        task.ApplyOutput = bufferedLogs
                    }
                    log.Printf("[Raw] Saved %d bytes of logs for task %d", len(bufferedLogs), task.ID)
                }
                h.streamManager.Close(task.ID)
            }
        }
        
        // 标记为失败
        task.Status = models.TaskStatusFailed
        task.ErrorMessage = fmt.Sprintf("Agent %s disconnected unexpectedly", agentID)
        task.CompletedAt = timePtr(time.Now())
        h.db.Save(&task)
    }
}
```

### 4. 修复编译错误

**文件**: `backend/services/task_queue_manager.go`

修复 `GetNextExecutableTask` 中的 err 变量声明。

**文件**: `backend/services/workspace_lifecycle.go`

删除不再需要的 `workspace.AgentID` 赋值（agent 现在由 TaskQueueManager 动态分配）。

##  日志保存保证

**Agent 在任何状态断开连接都会保存任务日志**：

1. **正常完成**：Agent 发送 `task_completed` 消息，日志已经通过 `log_stream` 实时保存
2. **任务失败**：Agent 发送 `task_failed` 消息，日志已经通过 `log_stream` 实时保存
3. **Agent 断开连接**：`cleanupAgentTasks` 会从 `streamManager` 获取缓冲日志并保存到数据库
4. **Agent 崩溃/网络中断**：`cleanupAgentTasks` 同样会保存已接收的日志

##  Apply Pending 任务安全性

**重启 Server 或 Agent 完全安全**：
1. `apply_pending` 状态的任务**永远不会**被 `GetNextExecutableTask` 返回
2. `RecoverPendingTasks` **只恢复** `status = 'pending'` 的任务
3. `CleanupOrphanTasks` 会**保护** `apply_pending` 任务，不会标记为失败
4. `apply_pending` 任务**只能**通过用户点击 "Confirm Apply" 按钮来触发

详见 `docs/task-511-pending-bug-fix.md` 中的详细证据。

## 修改的文件

1. **backend/controllers/workspace_task_controller.go**
   - `CreatePlanTask`: 添加重试机制（最多3次，指数退避）
   - `ConfirmApply`: 添加重试机制（最多3次，指数退避）

2. **backend/services/task_queue_manager.go**
   - `GetNextExecutableTask`: 修复 err 变量声明
   - `pushTaskToAgent`: 添加 `task.AgentID` 赋值

3. **backend/internal/models/workspace.go**
   - `WorkspaceTask.AgentID`: 类型从 `*uint` 改为 `*string`

4. **backend/internal/handlers/agent_cc_handler_raw.go**
   - `handleConnection`: 添加 agent 断开连接时的任务清理
   - `cleanupAgentTasks`: 新增函数，清理断开连接的 agent 正在执行的任务，**保存日志**
   - `timePtr`: 添加辅助函数

5. **backend/services/workspace_lifecycle.go**
   - 删除不再需要的 `workspace.AgentID` 赋值

6. **scripts/alter_workspace_tasks_agent_id_to_varchar.sql**
   - 数据库迁移脚本：将 `agent_id` 从 `integer` 改为 `varchar(50)`

## 测试步骤

### 1. 测试任务 511（pending bug）
```bash
# 重启后端服务
cd backend
./iac-platform
```

观察日志应该看到：
```
[TaskQueue] Recovering pending tasks for X workspaces
[TaskQueue] Successfully triggered task execution for workspace ws-mb7m9ii5ey (attempt 1)
```

任务 511 应该被自动调度并开始执行。

### 2. 测试任务 514（agent 断开）

重启后端服务后，任务 514 应该：
- 被标记为 failed
- 错误信息显示 "Agent xxx disconnected unexpectedly"
- **日志已保存到 plan_output 字段**

### 3. 测试 Agent 断开连接时的日志保存

1. 创建一个新任务
2. 等待任务开始执行（status = running）
3. 强制终止 agent（kill pod 或 ctrl+c）
4. 检查任务状态：
   - 应该被标记为 failed
   - `plan_output` 或 `apply_output` 应该包含已接收的日志
   - 错误信息显示 agent 断开连接

### 4. 测试 apply_pending 安全性

1. 创建 plan_and_apply 任务
2. 等待 plan 完成，任务进入 `apply_pending` 状态
3. 重启后端服务
4. 确认任务仍是 `apply_pending` 状态，没有自动执行 apply

## 部署步骤

1. **应用数据库迁移**：
   ```bash
   docker exec -i iac-platform-postgres psql -U postgres -d iac_platform < scripts/alter_workspace_tasks_agent_id_to_varchar.sql
   ```

2. **编译后端**：
   ```bash
   cd backend
   go build -o iac-platform main.go
   ```

3. **重启后端服务**：
   ```bash
   # 停止当前服务
   # 启动新服务
   ./iac-platform
   ```

4. **验证**：
   - 检查任务 511 是否被调度
   - 检查任务 514 是否被标记为 failed 且日志已保存
   - 检查所有 apply_pending 任务保持不变

## 相关文档

- `docs/task-511-pending-bug-fix.md` - 详细的 bug 分析和修复说明
- `docs/task-497-server-restart-apply-pending-fix.md` - Apply pending 重启安全性
- `docs/task-500-apply-pending-auto-execute-bug-fix.md` - Apply pending 自动执行 bug
- `docs/task-510-apply-pending-restart-fix.md` - Apply pending 重启修复
