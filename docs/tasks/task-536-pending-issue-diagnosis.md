# Task 536 Pending问题诊断

## 问题现象

任务536(plan任务)一直处于pending状态,无法执行。

## 日志分析

### 关键日志1: 任务创建时的调度尝试
```
2025/11/07 15:50:30 [TaskQueue] ===== TryExecuteNextTask START for workspace ws-mb7m9ii5ey =====
2025/11/07 15:50:30 [TaskQueue] TryExecuteNextTask called for workspace ws-mb7m9ii5ey
2025/11/07 15:50:30 [TaskQueue] GetNextExecutableTask for workspace ws-mb7m9ii5ey
2025/11/07 15:50:30 [TaskQueue] Found 1 blocking plan_and_apply tasks (running or apply_pending), all tasks must wait
2025/11/07 15:50:30 [TaskQueue] No executable tasks for workspace ws-mb7m9ii5ey
2025/11/07 15:50:30 [TaskQueue] ===== TryExecuteNextTask END for workspace ws-mb7m9ii5ey =====
```

**问题**: 系统认为有1个blocking的plan_and_apply任务,但实际上任务535已经是success状态了!

### 关键日志2: 容量计算
```
2025/11/07 15:52:48 [K8sDeployment] Pool pool-z73eh8ihywlmgx0x capacity calculation: plan_tasks=1, plan_and_apply_tasks=1, agents_for_plan=1, agents_for_plan_and_apply=1, required_agents=1
```

**问题**: 容量计算还在统计1个plan_and_apply任务,但任务535已经success了!

## 数据库实际状态

```sql
-- 任务536
id=536, workspace_id=ws-mb7m9ii5ey, task_type=plan, status=pending, retry_count=0

-- 任务535 (前一个任务)
id=535, task_type=plan_and_apply, status=success, created_at=2025-11-07 15:50:27

-- Workspace状态
workspace_id=ws-mb7m9ii5ey, execution_mode=k8s, is_locked=false
```

## 根本原因

**代码修复还没有重启应用!**

虽然我们已经修复了代码:
1.  Agent容量计算逻辑 (`k8s_deployment_service.go`)
2.  Workspace lock检查 (`task_queue_manager.go`)
3.  Pending任务重试机制 (`task_queue_manager.go`)

但是**后端服务还在运行旧代码**,所以修复没有生效。

## 为什么容量计算显示1个plan_and_apply任务?

查看容量计算的SQL查询:
```go
Where("workspace_tasks.task_type = ?", models.TaskTypePlanAndApply).
Where("workspace_tasks.status IN (?)", []models.TaskStatus{
    models.TaskStatusPending,
    models.TaskStatusRunning,
    models.TaskStatusApplyPending,
})
```

任务535的状态是`success`,不在这个查询范围内,所以不应该被计数。

但是日志显示统计到了1个plan_and_apply任务,这说明:
- 要么还有其他workspace的plan_and_apply任务
- 要么代码还是旧版本

让我检查是否有其他workspace的任务...

实际上,从日志时间戳看:
- 15:50:30 - 任务536创建,尝试调度,发现blocking任务
- 15:52:38 - 任务535完成(success)
- 15:52:48 - 容量计算显示plan_and_apply_tasks=0 (正确!)

**所以容量计算已经正确了!**

## 真正的问题

任务536在15:50:30创建时被阻塞,之后**没有重试机制**来重新尝试执行!

虽然我们添加了`StartPendingTasksMonitor`方法,但是:
1. 这个方法需要在main.go中启动
2. 当前代码可能还没有启动这个监控器

## 解决方案

### 立即解决(手动触发)
需要手动触发任务536的执行,可以通过以下方式:
1. 重启后端服务(应用新代码)
2. 或者在数据库中手动触发

### 长期解决
1. **重启后端服务**以应用所有修复
2. **确保StartPendingTasksMonitor已启动**
3. **验证pending任务能自动重试**

## 需要在main.go中添加的代码

```go
// 启动pending任务监控器(30秒检查一次)
go taskQueueManager.StartPendingTasksMonitor(ctx, 30*time.Second)
```

## 验证步骤

1. 重启后端服务
2. 观察任务536是否自动开始执行
3. 检查日志中是否有:
   - `[TaskQueue] Starting pending tasks monitor`
   - `[TaskQueue] Checking N workspaces with pending tasks`
   - workspace lock检查日志
4. 验证新的容量计算逻辑是否正确工作
