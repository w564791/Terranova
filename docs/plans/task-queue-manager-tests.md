# TaskQueueManager 核心执行流程测试计划

> 状态：待执行
> 创建时间：2026-02-17
> 目标文件：`backend/services/task_queue_manager_test.go`
> 被测文件：`backend/services/task_queue_manager.go`

## 背景

`TaskQueueManager` 是任务调度的核心，负责任务优先级排序、workspace 串行锁、Agent 分发、多副本重试。
当前测试覆盖为 **0**，是关键质量风险。

## 测试策略

### Mock 方案

| 依赖 | 方案 | 说明 |
|------|------|------|
| `*gorm.DB` | SQLite in-memory | `gorm.io/driver/sqlite`，真实 SQL 语义 |
| `AgentCCHandler` | Interface mock | 已有接口定义，直接实现 mock struct |
| `*pglock.Locker` | 注入 mock DB 的真实 Locker | Locker 依赖 `*gorm.DB`，用 `pglock.New(mockDB)` |
| `*pgpubsub.PubSub` | 设为 nil | 跨副本投递测试用真实 PubSub 太重，设 nil 走本地路径 |
| `*K8sDeploymentService` | 设为 nil | 非 K8s 测试场景 |
| `*TerraformExecutor` | 设为 nil | Local 模式 executeTask 是 goroutine，不阻塞测试 |

### 测试基础设施（Task 0）

```go
// mockAgentCCHandler 实现 AgentCCHandler 接口
type mockAgentCCHandler struct {
    connectedAgents []string
    availableMap    map[string]bool      // agentID -> available
    sentTasks       []sentTaskRecord     // 记录发送的任务
    sendError       error                // 模拟发送失败
}

type sentTaskRecord struct {
    AgentID     string
    TaskID      uint
    WorkspaceID string
    Action      string
}

// setupTestDB 创建 SQLite in-memory DB + AutoMigrate
func setupTestDB(t *testing.T) *gorm.DB

// createTestWorkspace 创建测试 workspace
func createTestWorkspace(db *gorm.DB, wsID string, locked bool, execMode models.ExecutionMode, poolID *string) *models.Workspace

// createTestTask 创建测试任务
func createTestTask(db *gorm.DB, wsID string, taskType models.TaskType, status models.TaskStatus) *models.WorkspaceTask

// createTestAgent 创建测试 agent
func createTestAgent(db *gorm.DB, agentID string, poolID string) *models.Agent

// newTestManager 创建可测试的 TaskQueueManager（注入 mock 依赖）
func newTestManager(db *gorm.DB, mock *mockAgentCCHandler) *TaskQueueManager
```

---

## 任务清单

### Task 0: 测试基础设施
- [ ] 完成
- 文件：`backend/services/task_queue_manager_test.go`
- 内容：
  1. `mockAgentCCHandler` struct 实现 `AgentCCHandler` 接口
  2. `setupTestDB()` — SQLite in-memory + AutoMigrate(`Workspace`, `WorkspaceTask`, `Agent`, `AgentPool`)
  3. `createTestWorkspace()` 工厂函数
  4. `createTestTask()` 工厂函数（支持设置 `ApplyConfirmedBy`、`Description` 等）
  5. `createTestAgent()` 工厂函数
  6. `newTestManager()` — 构造 `TaskQueueManager`，注入 mock DB 和 mock handler
- 验证：`go test -run TestSetup -v ./services/`

### Task 1: GetNextExecutableTask 测试 (P0)
- [ ] 完成
- 依赖：Task 0
- 测试用例：

```
TestGetNextExecutableTask_LockedWorkspace
  → workspace.IsLocked=true → 返回 nil, nil

TestGetNextExecutableTask_NoPendingTasks
  → 空数据 → 返回 nil, nil

TestGetNextExecutableTask_PlanAndApply_NoBLocker
  → 1 个 plan_and_apply pending, 无阻塞 → 返回该任务

TestGetNextExecutableTask_PlanAndApply_BlockedByRunning
  → task1: plan_and_apply running (id=1)
  → task2: plan_and_apply pending (id=2)
  → 返回 nil（plan_and_apply 被阻塞，无 plan 可执行）

TestGetNextExecutableTask_PlanAndApply_BlockedByApplyPending
  → task1: plan_and_apply apply_pending (id=1)
  → task2: plan_and_apply pending (id=2)
  → 返回 nil

TestGetNextExecutableTask_PlanIndependent
  → task1: plan_and_apply running
  → task2: plan pending
  → 返回 task2（plan 完全独立，不受 plan_and_apply 阻塞）

TestGetNextExecutableTask_DriftCheckLowestPriority
  → task1: drift_check pending
  → 无其他任务 → 返回 task1

TestGetNextExecutableTask_Priority
  → task1: plan_and_apply pending (无阻塞)
  → task2: plan pending
  → task3: drift_check pending
  → 返回 task1（plan_and_apply 优先级最高）

TestGetNextExecutableTask_PlanAndApplyBlocked_FallbackToPlan
  → task1: plan_and_apply apply_pending (id=1, 阻塞后续)
  → task2: plan_and_apply pending (id=2, 被 task1 阻塞)
  → task3: plan pending
  → 返回 task3（plan_and_apply 被阻塞，回退到 plan）

TestGetNextExecutableTask_WorkspaceNotFound
  → 不存在的 workspace → 返回 error

TestGetNextExecutableTask_OnlyApplyPending_NotReturned
  → 1 个 plan_and_apply apply_pending（未确认）
  → 无 pending → 返回 nil（apply_pending 不被自动调度）
```

- 验证：`go test -run TestGetNextExecutableTask -v ./services/`

### Task 2: TryExecuteNextTask 测试 (P0)
- [ ] 完成
- 依赖：Task 0, Task 1
- 测试用例：

```
TestTryExecuteNextTask_NoTask
  → 无 pending 任务 → 返回 nil

TestTryExecuteNextTask_PlanTask_NoLock
  → plan pending + agent 模式 → 不加 advisory lock，调用 pushTaskToAgent
  → 验证 mock handler 收到正确参数

TestTryExecuteNextTask_PlanAndApply_AcquiresLock
  → plan_and_apply pending + local 模式 → 加 advisory lock → 执行
  → 注意：advisory lock 依赖真实 PG，SQLite 无法测试
  → 方案：跳过锁测试或 mock pgLocker

TestTryExecuteNextTask_AgentMode_CallsPushTaskToAgent
  → workspace.ExecutionMode = "agent"
  → 验证调用 pushTaskToAgent

TestTryExecuteNextTask_K8sMode_CallsPushTaskToAgent
  → workspace.ExecutionMode = "k8s"
  → 验证调用 pushTaskToAgent

TestTryExecuteNextTask_LocalMode_ExecutesLocally
  → workspace.ExecutionMode = "local"
  → 验证 task 被异步执行（status 变化）
```

- 注意：`pglock.Locker` 使用 `pg_try_advisory_lock` SQL 函数，SQLite 不支持。
  需要将 `pgLocker` 抽象为 interface 或在测试中跳过锁相关逻辑。
  **推荐方案**：为 `pgLocker` 引入 interface wrapper：

```go
// LockProvider 接口（新增到 task_queue_manager.go）
type LockProvider interface {
    TryLock(key int64) (bool, error)
    Unlock(key int64) (bool, error)
}

// pglock.Locker 已满足此接口，无需修改
// 测试中注入 mockLockProvider
```

- 验证：`go test -run TestTryExecuteNextTask -v ./services/`

### Task 3: pushTaskToAgent 测试 (P1)
- [ ] 完成
- 依赖：Task 0
- 测试用例：

```
TestPushTaskToAgent_SecurityReject_UnconfirmedApplyPending
  → task.Status=apply_pending, ApplyConfirmedBy=nil
  → 返回 error "requires explicit user confirmation"

TestPushTaskToAgent_ConfirmedApplyPending_Succeeds
  → task.Status=apply_pending, ApplyConfirmedBy="admin"
  → action="apply", stage="applying"
  → mock handler 收到 action="apply"

TestPushTaskToAgent_PendingTask_Succeeds
  → task.Status=pending
  → action="plan", stage="planning"
  → task.Status 更新为 running
  → task.AgentID 设置为选中的 agent

TestPushTaskToAgent_NilHandler_Retries
  → agentCCHandler=nil → 不报错，调度重试

TestPushTaskToAgent_NoPool_Retries
  → workspace.CurrentPoolID=nil → 调度重试

TestPushTaskToAgent_NoConnectedAgents_Retries
  → mock handler 返回空 agents → 调度重试

TestPushTaskToAgent_AgentNotInPool_Skipped
  → agent.PoolID != workspace.CurrentPoolID → 跳过该 agent

TestPushTaskToAgent_AgentNotAvailable_Skipped
  → IsAgentAvailable 返回 false → 跳过该 agent

TestPushTaskToAgent_SendFailed_Rollback
  → SendTaskToAgent 返回 error（非 "not connected"）
  → task.Status 回滚为 pending（或 apply_pending）
  → task.AgentID 清空
  → RetryCount++

TestPushTaskToAgent_SendFailed_NotConnected_PGNotify
  → SendTaskToAgent 返回 "not connected" error
  → pubsub != nil → 走 PG NOTIFY 路径（设 pubsub=nil 时跳过）

TestPushTaskToAgent_Success_ResetsRetryCount
  → task.RetryCount=3 → 发送成功 → RetryCount 重置为 0
```

- 注意：`pushTaskToAgent` 是私有方法。测试选项：
  1. 通过 `TryExecuteNextTask` 间接测试（推荐）
  2. 将测试文件放在 `services` package 内直接访问
- 验证：`go test -run TestPushTaskToAgent -v ./services/`

### Task 4: ExecuteConfirmedApply 测试 (P1)
- [ ] 完成
- 依赖：Task 0
- 测试用例：

```
TestExecuteConfirmedApply_TaskNotFound
  → taskID 不存在 → 返回 "task not found" error

TestExecuteConfirmedApply_NotApplyPending
  → task.Status="running" → 返回 "not in apply_pending status" error

TestExecuteConfirmedApply_NotConfirmed
  → task.Status="apply_pending", ApplyConfirmedBy=nil
  → 返回 "has not been confirmed" error

TestExecuteConfirmedApply_AgentMode_CallsPush
  → workspace.ExecutionMode="agent"
  → 验证走 pushTaskToAgent 路径

TestExecuteConfirmedApply_LocalMode_ExecutesLocally
  → workspace.ExecutionMode="local"
  → 验证异步执行
```

- 验证：`go test -run TestExecuteConfirmedApply -v ./services/`

### Task 5: checkAndRetryPendingTasks 测试 (P2)
- [ ] 完成
- 依赖：Task 0
- 测试用例：

```
TestCheckAndRetryPendingTasks_NoPendingTasks
  → 无任何待处理任务 → 无操作（不 panic）

TestCheckAndRetryPendingTasks_PendingTasks
  → 2 个 workspace 各有 pending 任务
  → 验证对每个 workspace 调用 TryExecuteNextTask

TestCheckAndRetryPendingTasks_ConfirmedApplyPending
  → 1 个 apply_pending + ApplyConfirmedBy 非空
  → 验证调用 ExecuteConfirmedApply

TestCheckAndRetryPendingTasks_UnconfirmedApplyPending_Ignored
  → 1 个 apply_pending + ApplyConfirmedBy=nil
  → 不调用 ExecuteConfirmedApply
```

- 注意：`checkAndRetryPendingTasks` 是私有方法，测试文件在同 package 可直接调用
- 验证：`go test -run TestCheckAndRetryPendingTasks -v ./services/`

### Task 6: RecoverPendingTasks + CleanupOrphanTasks 测试 (P2)
- [ ] 完成
- 依赖：Task 0
- 测试用例：

```
TestCleanupOrphanTasks_NoOrphans
  → 无 running 任务 → 无操作

TestCleanupOrphanTasks_RunningTask_MarkedFailed
  → task.Status=running, stage="planning"
  → 清理后 Status=failed, ErrorMessage 包含 "server restart"

TestCleanupOrphanTasks_ApplyPendingStage_ResetToApplyPending
  → task.Status=running, stage="apply_pending"
  → 清理后 Status=apply_pending（不标记失败）

TestRecoverPendingTasks_CancelsRunTriggerTasks
  → task.Description="Triggered by workspace ws-xxx", status=pending
  → 恢复后 Status=cancelled

TestRecoverPendingTasks_RecoversNormalPending
  → task.Status=pending, description="Manual run"
  → 触发 TryExecuteNextTask

TestRecoverPendingTasks_SkipsApplyPending
  → task.Status=apply_pending
  → 不自动恢复执行
```

- 验证：`go test -run TestCleanupOrphanTasks -v ./services/ && go test -run TestRecoverPendingTasks -v ./services/`

### Task 7: 辅助方法测试
- [ ] 完成
- 依赖：Task 0
- 测试用例：

```
TestCalculateRetryDelay
  → RetryCount=0 → 5s
  → RetryCount=1 → 10s
  → RetryCount=4 → 60s
  → RetryCount=99 → 60s（上限）

TestCanExecuteNewTask_Locked
  → workspace locked → false

TestCanExecuteNewTask_HasBlockingTask
  → plan_and_apply running → false

TestCanExecuteNewTask_NoBLocker
  → 无阻塞 → true
```

- 验证：`go test -run TestCalculateRetryDelay -v ./services/ && go test -run TestCanExecuteNewTask -v ./services/`

---

## 前置准备（可能需要的代码修改）

### 1. 引入 LockProvider 接口（推荐）

当前 `TaskQueueManager.pgLocker` 类型为 `*pglock.Locker`（struct），SQLite 无法执行 `pg_try_advisory_lock`。

**修改方案**：在 `task_queue_manager.go` 中引入接口：

```go
// LockProvider abstracts advisory lock operations for testability.
type LockProvider interface {
    TryLock(key int64) (bool, error)
    Unlock(key int64) (bool, error)
}
```

将 `pgLocker *pglock.Locker` 改为 `pgLocker LockProvider`。
`pglock.Locker` 已满足此接口，无需修改任何调用方。

### 2. 添加 SQLite 依赖

```bash
cd backend && go get gorm.io/driver/sqlite
```

### 3. 导出或包内访问

测试文件 `task_queue_manager_test.go` 放在 `package services`，可直接访问私有方法和字段。

---

## 执行顺序

```
Task 0 (基础设施)
  ↓
Task 1 (GetNextExecutableTask) ←── P0 核心调度
  ↓
Task 2 (TryExecuteNextTask)    ←── P0 锁+分发
  ↓
Task 3 (pushTaskToAgent)       ←── P1 安全+Agent分发
  ↓
Task 4 (ExecuteConfirmedApply) ←── P1 确认申请
  ↓
Task 5 (checkAndRetryPendingTasks) ←── P2 监控器
  ↓
Task 6 (RecoverPendingTasks)   ←── P2 恢复
  ↓
Task 7 (辅助方法)              ←── P2 边缘
```

## 验证命令

```bash
# 全部测试
cd backend && go test -v -count=1 ./services/ -run "Test(GetNextExecutableTask|TryExecuteNextTask|PushTaskToAgent|ExecuteConfirmedApply|CheckAndRetryPendingTasks|CleanupOrphanTasks|RecoverPendingTasks|CalculateRetryDelay|CanExecuteNewTask)"

# 覆盖率
cd backend && go test -coverprofile=coverage.out ./services/ && go tool cover -func=coverage.out | grep task_queue_manager
```

## 完成标准

1. 所有 Task 0-7 的测试用例通过
2. `go vet ./services/` 无报错
3. `task_queue_manager.go` 核心方法覆盖率 > 70%
4. 不引入对真实 PostgreSQL 的依赖（纯 SQLite in-memory）
