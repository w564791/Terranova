# Task 519 调试指南 - Agent取消任务仍在运行

## 问题现象

任务519已经取消（数据库状态为cancelled），但Agent仍在执行任务。

## 需要检查的关键点

### 1. 检查Server日志

**查找取消相关的日志：**
```bash
# 在Server日志中搜索任务519的取消操作
grep -E "CancelTask|cancel.*519|task.*519.*cancel" backend/logs/*.log
```

**期望看到的日志：**
```
[CancelTask] Sent cancel signal to agent agent-xxx for task 519
[Raw] Sending cancel_task command to agent agent-xxx for task 519
[Raw] Successfully sent message to agent agent-xxx
```

**如果看到这些日志：**
- ❌ `[CancelTask] agentCCHandler is nil` → agentCCHandler未正确注入
- ❌ `[CancelTask] Failed to send cancel signal` → WebSocket发送失败
- ❌ 没有任何CancelTask日志 → Controller的CancelTask函数可能没有被调用

### 2. 检查Agent日志

**查找取消相关的日志：**
```bash
# 在Agent日志中搜索取消消息
grep -E "CANCEL|cancel_task|Received cancel" /path/to/agent.log
```

**期望看到的日志（新增的详细日志）：**
```
========================================
[CANCEL] Received cancel_task message from server
[CANCEL] Payload: map[task_id:519]
========================================
[CANCEL]  Parsed task_id: 519
[CANCEL] Current taskContexts map has 1 entries
[CANCEL]   - Task 519 is in taskContexts
[CANCEL]  Task 519 FOUND in running tasks, proceeding to cancel
[CANCEL] Calling cancelFunc() for task 519...
[CANCEL]  Successfully cancelled task 519 execution 
[CANCEL] The Terraform process should now terminate
========================================
```

**如果没有看到这些日志：**
- Agent没有收到cancel_task消息
- 说明Server端没有成功发送，或WebSocket连接有问题

### 3. 检查数据库状态

```sql
-- 检查任务519的详细信息
SELECT 
    id,
    status,
    agent_id,
    execution_mode,
    started_at,
    completed_at,
    error_message
FROM workspace_tasks 
WHERE id = 519;

-- 检查Agent信息
SELECT 
    agent_id,
    status,
    last_ping_at
FROM agents
WHERE id = (SELECT agent_id FROM workspace_tasks WHERE id = 519);
```

### 4. 检查WebSocket连接

**Server端：**
```bash
# 检查是否有Agent连接到C&C
grep -E "Agent.*connected to C&C|RawAgentCCHandler" backend/logs/*.log | tail -20
```

**Agent端：**
```bash
# 检查WebSocket连接状态
grep -E "Connect.*Successfully|WebSocket connected" /path/to/agent.log | tail -10
```

## 可能的原因分析

### 原因1：Server端agentCCHandler为nil

**症状：** Server日志显示"agentCCHandler is nil"

**原因：** 
- 代码更新后Server没有重启
- 或者router.Setup没有正确传递rawCCHandler

**验证：**
```bash
# 检查Server启动日志
grep "Agent C&C handler configured" backend/logs/*.log
```

应该看到：
```
[TaskQueue] Agent C&C handler configured (using Raw WebSocket handler)
```

### 原因2：Agent没有收到消息

**症状：** Agent日志中没有任何[CANCEL]相关日志

**可能原因：**
1. WebSocket连接断开
2. Server发送失败
3. Agent的handleMessages没有正确处理

**验证：**
```bash
# 检查Agent的WebSocket连接
grep "handleMessages" /path/to/agent.log | tail -20
```

### 原因3：任务不在taskContexts中

**症状：** Agent日志显示"Task XXX NOT FOUND in running tasks"

**可能原因：**
1. 任务已经完成但数据库状态没更新
2. taskContexts map没有正确维护
3. 任务是在旧版本Agent上启动的（没有taskContexts支持）

## 立即排查步骤

### Step 1: 确认Server是否发送了取消信号

```bash
# 查看Server最近的日志
tail -50 backend/logs/server.log | grep -A 5 -B 5 "519"
```

### Step 2: 确认Agent是否收到了消息

```bash
# 查看Agent最近的日志
tail -100 /path/to/agent.log | grep -E "Server->Agent|CANCEL"
```

### Step 3: 检查Agent版本

```bash
# 确认Agent是否使用了最新编译的版本
ls -lh backend/cmd/agent/agent
# 检查编译时间是否是最近的
```

## 临时解决方案

如果取消功能仍然不工作，可以：

1. **手动停止Agent进程** - 这会导致任务失败，但至少停止了执行
2. **等待任务完成** - 如果任务快完成了，可以等待
3. **检查并修复代码** - 根据日志找出问题所在

## 下一步行动

请提供以下信息以便进一步诊断：

1. **Server日志** - 关于任务519取消的部分
2. **Agent日志** - 关于任务519的所有日志
3. **Agent版本** - 确认是否使用了最新编译的版本
4. **WebSocket连接状态** - Agent是否正常连接到Server

根据这些信息，我们可以确定问题的具体原因并进行修复。
