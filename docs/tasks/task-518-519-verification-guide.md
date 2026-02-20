# Task 518/519 - Agent取消任务功能验证指南

## 验证目的

确认Task 518的修复已经生效，agent能够正确接收并处理取消信号。

## 前提条件

 Agent和Server都已重启
 代码已更新到commit fe5f710或更新版本

## 验证步骤

### 1. 检查Server日志

重启Server后，应该看到以下日志：

```
[TaskQueue] Agent C&C handler configured (using Raw WebSocket handler)
```

这表示agentCCHandler已经正确注入。

### 2. 检查Agent连接

Agent启动后应该看到：

```
[Connect] Successfully connected on attempt #1
[Connect] Handlers started, Connect() returning
```

### 3. 创建测试任务

1. 在UI中创建一个Plan任务
2. 等待任务开始执行（状态变为`running`）
3. 在Server日志中应该看到：

```
[TaskQueue] Assigning task XXX to agent YYY
[Raw] Sending run_task command to agent YYY for task XXX
```

### 4. 取消任务

点击"取消任务"按钮，观察日志：

**Server端日志（关键）：**
```
[CancelTask] Sent cancel signal to agent agent-xxx for task 519
[Raw] Sending cancel_task command to agent agent-xxx for task 519
[Raw] Successfully sent message to agent agent-xxx
```

**Agent端日志（关键）：**
```
[Server->Agent] Received cancel command for task 519
[Agent] Cancelled task 519 execution
```

### 5. 验证结果

 **成功标志：**
- Server日志显示"Sent cancel signal"
- Agent日志显示"Received cancel command"和"Cancelled task execution"
- 任务状态变为`cancelled`
- Agent停止执行Terraform命令
- Agent的current_tasks列表中移除了该任务

❌ **失败标志：**
- Server日志显示"agentCCHandler is nil"
- Agent没有收到取消信号
- Agent继续执行任务直到完成

## 如果验证失败

### 问题1：Server日志显示"agentCCHandler is nil"

**原因：** agentCCHandler没有正确注入

**解决方案：**
1. 确认代码已更新到最新版本
2. 检查`backend/main.go`中是否有：
   ```go
   r := router.Setup(db, streamManager, wsHub, nil, queueManager, rawCCHandler)
   ```
3. 重新编译并重启Server

### 问题2：Agent没有收到取消信号

**原因：** WebSocket连接可能有问题

**解决方案：**
1. 检查Agent日志，确认WebSocket连接正常
2. 检查Server的C&C端口（默认8090）是否可访问
3. 重启Agent

### 问题3：Agent收到信号但任务继续执行

**原因：** Agent端的handleCancelTask可能有问题

**解决方案：**
1. 检查Agent代码中的taskContexts map是否正常工作
2. 确认context.WithCancel正确使用
3. 重新编译并重启Agent

## 调试命令

### 查看Server日志（最近100行）
```bash
tail -100 backend/logs/server.log | grep -E "CancelTask|cancel_task|Sent cancel"
```

### 查看Agent日志
```bash
# Agent日志位置取决于你的配置
tail -100 /path/to/agent.log | grep -E "cancel|Cancelled"
```

### 检查任务状态
```sql
SELECT id, status, agent_id, created_at, completed_at 
FROM workspace_tasks 
WHERE id = 519;
```

## 预期的完整日志流程

```
# 1. 用户点击取消
[UI] Cancel button clicked for task 519

# 2. Server接收取消请求
[Server] POST /api/v1/workspaces/ws-xxx/tasks/519/cancel

# 3. Controller处理
[CancelTask] Attempting to send cancel signal to agent agent-xxx for task 519
[CancelTask] Sent cancel signal to agent agent-xxx for task 519

# 4. C&C Handler发送WebSocket消息
[Raw] Sending cancel_task command to agent agent-xxx for task 519
[Raw] Successfully sent message to agent agent-xxx

# 5. Agent接收并处理
[Server->Agent] Received cancel command for task 519
[Agent] Cancelled task 519 execution

# 6. 数据库更新
[Server] Task 519 status updated to cancelled
```

## 成功标准

-  Server能够发送取消信号
-  Agent能够接收取消信号
-  Agent能够停止任务执行
-  数据库状态正确更新为cancelled
-  Agent的current_tasks正确更新

## 注意事项

1. **任务必须是running状态** - 只有正在Agent上运行的任务才会发送取消信号
2. **Agent必须在线** - 如果Agent离线，信号无法送达（但数据库仍会标记为cancelled）
3. **WebSocket连接正常** - 确保Agent和Server的WebSocket连接正常

## 相关文档

- `docs/task-518-agent-cancel-incomplete-fix.md` - 问题分析
- `docs/task-518-agent-cancel-complete-fix.md` - 修复说明
- `docs/task-510-agent-cancel-task-bug-fix.md` - 原始设计文档
