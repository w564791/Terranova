# Task 644 修复部署和测试指南

## 修改总结

本次修改了 `backend/services/task_queue_manager.go` 中的 `pushTaskToAgent` 函数，将 agent_id 的设置和保存移到 SendTaskToAgent **之前**，解决了竞态条件问题。

## 部署步骤

### 1. 重新编译 Backend

```bash
cd backend
go build -o iac-platform-server
```

### 2. 重启 Backend 服务

```bash
# 如果使用 systemd
sudo systemctl restart iac-platform

# 或者直接kill进程后重启
pkill iac-platform-server
./iac-platform-server
```

### 3. 重启 Agent（如果有）

```bash
# Agent 也需要重启以使用新的代码
pkill iac-platform-agent
./iac-platform-agent
```

## 测试步骤

### 1. 创建新的 plan_and_apply 任务

通过 UI 或 API 创建一个新的 plan_and_apply 任务

### 2. 观察 Plan 阶段日志

Plan 阶段应该正常执行，agent_id 会被保存到数据库

### 3. 观察 Apply 阶段日志

**关键检查点**：Apply 阶段的日志应该显示：

```
[INFO] Same agent detected, can skip init
[INFO]   - Plan agent: agent-pool-xxx
[INFO]   - Apply agent: agent-pool-xxx  ← 应该有值，不是 (none)
[INFO] Reusing working directory: /tmp/terraform-xxx
[INFO] Skipping terraform init (optimization)
```

### 4. 验证数据库

```bash
docker exec iac-platform-postgres psql -U postgres -d iac_platform -c \
  "SELECT id, task_type, status, agent_id, plan_task_id FROM workspace_tasks ORDER BY id DESC LIMIT 1;"
```

应该看到新任务的 agent_id 有值。

## 如果还是失败

如果新任务还是显示 `Apply agent: (none)`，请提供：

1. 新任务的 ID
2. Apply 阶段的完整日志
3. 数据库查询结果
4. Backend 服务的启动日志（确认新代码已加载）

## 注意事项

- 任务 644 是旧任务，不会受益于这次修改
- 必须创建新任务才能测试修复效果
- 确保 Backend 和 Agent 都已重启
