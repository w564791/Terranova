# IAC Platform 执行流程 — 逻辑问题与优化项

基于对核心执行代码的系统性审查，以下按严重程度分类列出所有发现。

---

## CRITICAL — 必须修复

### C1. `applying` 临界区在错误/取消路径未退出

**文件**: `services/terraform_executor.go`
**行号**: 2121 (EnterCriticalSection), 2131/2144/2167/2179/2189/2198/2243 (错误返回)

`applying` 临界区在 line 2121 进入，但**没有使用 `defer`**。仅在成功路径（line 2262）退出。所有错误/取消路径调用 `saveTaskFailure` 或 `saveTaskCancellation` 后直接 `return`，临界区永远不会退出。

```go
// line 2121
s.signalManager.EnterCriticalSection("applying")
// ... 多个错误路径直接 return，没有 ExitCriticalSection
// line 2262 (仅成功路径)
s.signalManager.ExitCriticalSection("applying")
```

对比 `saving_state`（line 2282-2283）和 `saving_plan`（line 3670-3671）都正确使用了 `defer`。

**修复**: 改为 `defer s.signalManager.ExitCriticalSection("applying")`，将 line 2262 的手动调用删除。

---

### C2. Apply 取消时 Workspace 不解锁、工作目录不清理

**文件**: `services/terraform_executor.go`
**行号**: 2189, 2243 (取消路径) → 1542-1570 (saveTaskCancellation)

`saveTaskCancellation` 仅更新 task 状态，**不解锁 workspace、不清理工作目录**。对比 `saveTaskFailure`：
- Apply 失败: 解锁 workspace (line 1610)、清理目录 (line 1634)
- Apply 取消: **两者都不做**

**影响**: 用户取消 Apply 后，workspace 永久锁定，工作目录永久残留。

**修复**: 在 `saveTaskCancellation` 中增加 apply 类型的解锁和清理逻辑，与 `saveTaskFailure` 对齐。

---

### C3. RunTask Webhook 全部失败时误判为通过

**文件**: `services/run_task_executor.go`
**行号**: 145-175

当所有 RunTask 的 Webhook 发送均失败时（例如端点不可达），错误进入 `execErrors` channel，`pendingResultIDs` 为空，line 172-174 直接返回 `(true, nil)` — 即"全部通过"。

```go
if len(pendingResultIDs) == 0 {
    log.Printf("[RunTask] No run tasks were successfully triggered")
    return true, nil  // BUG: 应该检查是否有 mandatory 任务失败
}
```

**影响**: Mandatory RunTask 的 Webhook 端点宕机时，Plan/Apply 不会被阻断，安全检查被绕过。

**修复**: 收集 `execErrors` 时检查对应 RunTask 的 enforcement level，如有 mandatory 任务的 webhook 失败，应返回 `(false, err)`。

---

### C4. RunTask Callback 端点无认证

**文件**: `internal/handlers/run_task_callback_handler.go`
**行号**: 37-78

Callback 端点 `PATCH /api/v1/run-task-results/{result_id}/callback` 完全无认证：
- 无 HMAC 签名验证
- 无 Bearer Token 验证
- `result_id` 格式为 `rtr-{16字符随机}`，可被枚举

**影响**: 任何人知道 `result_id` 格式后可伪造回调，将 mandatory RunTask 标记为 passed，绕过安全检查。

**修复**: 至少添加以下一种认证：
1. 在 Webhook payload 中下发一次性 token，callback 时携带验证
2. 验证 HMAC 签名（与 Webhook 发送时使用相同 key）
3. 限制 callback 来源 IP

---

### C5. RunTask AccessToken 未生成

**文件**: `services/run_task_executor.go`
**行号**: 311-323, 419

`RunTaskResult` 创建时未生成 `AccessToken`（字段为空字符串），但 Webhook payload 中将空 token 发送给外部服务：

```go
"access_token": result.AccessToken,  // 空字符串
```

`run_task_token_service.go` 中定义了 token 生成逻辑，但从未在 executor 中调用。

**影响**: 外部 RunTask 服务无法通过 token 访问平台 API 获取 plan 数据，RunTask 功能不完整。

**修复**: 在创建 `RunTaskResult` 后调用 token service 生成 JWT token 并填入。

---

### C6. `pre_plan` 和 `pre_apply` RunTask 未实装

**文件**: `services/terraform_executor.go`

模型层定义了 4 个 RunTask 阶段：

| 阶段 | 定义 | 实际调用 |
|------|------|---------|
| `pre_plan` | `models.RunTaskStagePrePlan` | **未调用** |
| `post_plan` | `models.RunTaskStagePostPlan` | ExecutePlan line 1133 |
| `pre_apply` | `models.RunTaskStagePreApply` | **未调用** |
| `post_apply` | `models.RunTaskStagePostApply` | **未调用**（TaskQueueManager 中也未找到） |

用户如果配置了 `pre_plan`、`pre_apply`、`post_apply` 阶段的 RunTask，它们会被静默忽略。

**修复**: 按需在以下位置添加 `executeRunTasksForStage` 调用：
- `pre_plan`: ExecutePlan 中 `planning` stage 之前
- `pre_apply`: ExecuteApply 中 `applying` stage 之前
- `post_apply`: ExecuteApply 中 task 标记为 applied 之后，或在 TaskQueueManager.OnTaskCompleted 中

---

## HIGH — 应尽快修复

### H1. Plan 早期错误未调用 saveTaskFailure

**文件**: `services/terraform_executor.go`
**行号**: 673, 742

Line 673（GetWorkspace 失败）直接 `return err`，不调用 `saveTaskFailure`：
- Task 状态不会更新为 Failed
- 错误信息不保存
- Task 卡在非终态（running）

Line 742（PrepareWorkspace 失败）同样直接 `return err`。

**对比**: line 727（EnsureTerraformBinary 失败）正确调用了 `saveTaskFailure`。

**修复**: 所有返回 error 的路径应先调用 `saveTaskFailure`。

---

### H2. Plan 取消时工作目录不清理

**文件**: `services/terraform_executor.go`
**行号**: 985-988, 1029-1032

Plan 执行期间用户取消，调用 `saveTaskCancellation`，但该函数不清理工作目录。
`saveTaskFailure` 对 plan 类型会清理（line 1597-1604），但 `saveTaskCancellation` 不会。

**影响**: 磁盘空间泄漏。

**修复**: 在 `saveTaskCancellation` 中增加工作目录清理逻辑。

---

### H3. Mandatory RunTask 失败时未保存完整日志

**文件**: `services/terraform_executor.go`
**行号**: 1141-1150

当 mandatory RunTask 失败（`!runTasksPassed`），直接设置 task 状态并 `UpdateTask`，**不调用 `saveTaskFailure`**：
- `task_logs` 表不会写入完整输出
- 失败通知不会发送

**对比**: line 1134-1138（RunTask 执行错误）正确调用了 `saveTaskFailure`。

**修复**: 统一使用 `saveTaskFailure` 处理失败，或在 line 1149 之后补充 `saveTaskLog` 和通知发送。

---

### H4. Apply 输出解析 Goroutine 无 Panic Recovery

**文件**: `services/terraform_executor.go`
**行号**: 2209-2231

两个 goroutine（stdout/stderr reader）调用 `applyParser.ParseLine(line)` 无 panic recovery。如果 ParseLine panic：
- Goroutine 静默终止
- 后续输出行丢失
- `wg.Wait()` (line 2235) 仍能完成（`defer wg.Done()` 会执行）但数据不完整

**修复**: 添加 `defer func() { if r := recover(); r != nil { log.Printf(...) } }()`。

---

### H5. TaskQueueManager 后续操作全部 fire-and-forget

**文件**: `services/task_queue_manager.go`
**行号**: ~913-946

Task 完成后的所有后续操作都通过 `go` 异步执行，无错误传播：

```go
go m.executeRunTriggers(task)         // 失败静默
go m.SyncCMDBAfterApply(task)         // 失败静默
go m.clearDriftIfApplicable(task)     // 失败静默
go m.TryExecuteNextTask(...)          // 失败静默
```

**问题**:
1. Run Trigger 失败 — 下游 workspace 不会被触发，但任务已标记为 applied
2. CMDB 同步失败 — CMDB 数据不一致
3. 无重试机制 — 网络抖动导致的失败永久丢失
4. 无顺序保障 — `TryExecuteNextTask` 可能在 CMDB 同步完成前执行下一个任务

**修复**: 至少为 CMDB 同步和 Run Triggers 增加重试机制；考虑将关键操作改为同步或有序异步。

---

### H6. CMDB 同步状态可永久卡在 "syncing"

**文件**: `services/task_queue_manager.go`
**行号**: ~1364-1384

使用 CAS 将 `cmdb_sync_status` 设为 `syncing`，但如果执行同步的 Pod crash：
- 状态永久停留在 `syncing`
- 后续 Apply 的 CMDB 同步全部跳过（`RowsAffected == 0`）
- 无超时、无心跳、无自动恢复

**修复**: 增加 `cmdb_sync_started_at` 字段，定期检查超时的 syncing 状态并重置。

---

### H7. Workspace 锁定失败仅 Warn

**文件**: `services/terraform_executor.go`
**行号**: 1253-1257

Plan 完成（有变更）时锁定 workspace 失败只记录 Warning，不阻断：

```go
if err := s.lockWorkspace(...); err != nil {
    logger.Warn("Failed to lock workspace: %v", err)  // 继续执行
}
```

Task 仍被设为 `apply_pending`，但 workspace 未锁定。用户可在 Plan 和 Apply 之间修改配置，导致 Apply 与实际配置不一致。

**修复**: 锁定失败应作为错误处理，或在 Apply 开始时验证锁状态。

---

## MEDIUM — 建议修复

### M1. Plan JSON 生成失败静默继续

**文件**: `services/terraform_executor.go`
**行号**: 1065-1070

`GeneratePlanJSON` 失败仅 Warn，后续：
- 资源变更计数为零（line 1074-1083）
- 异步 goroutine 跳过（line 1105）
- Task 仍标记为成功，但无资源变更数据

**建议**: 对 `plan_and_apply` 类型，Plan JSON 生成失败应视为错误（影响 Apply 审批决策）。

---

### M2. SavePlanDataWithLogging 无返回值

**文件**: `services/terraform_executor.go`
**行号**: 1089, ~3653

该函数无 error 返回值，内部错误仅日志记录。调用方无法感知 plan 数据保存失败。

```go
s.SavePlanDataWithLogging(task, planFile, planJSON, logger)  // 无法检查错误
```

**建议**: 增加 error 返回值，保存失败时终止任务。

---

### M3. 通知 Goroutine 无上限

**文件**: `services/notification_sender.go`
**行号**: 643, 669

每个匹配的通知配置都创建一个 goroutine 异步发送。在高并发场景（大量任务同时完成 + 大量通知配置），可导致 goroutine 爆炸。

**建议**: 使用 `semaphore` 或 worker pool 限制并发通知数量。

---

### M4. 通知无重试实现

**文件**: `services/notification_sender.go`
**行号**: ~207-281

`NotificationConfig` 模型有 `RetryCount`/`RetryIntervalSeconds` 字段，`NotificationLog` 有 `MaxRetryCount`/`NextRetryAt` 字段，但发送逻辑中无任何重试循环。单次失败即标记为 failed。

**建议**: 实现重试循环或异步重试队列。

---

### M5. 取消通知缺失

**文件**: `services/terraform_executor.go`

Plan/Apply 取消时（`saveTaskCancellation`），不发送任何通知。模型定义了 `NotificationEventTaskCancelled` 事件但未使用。

**对比**: 失败时 `saveTaskFailure` 发送 `task_failed` 通知 (line 1656-1665)。

**建议**: 在 `saveTaskCancellation` 中补充 `task_cancelled` 通知发送。

---

### M6. 异步资源解析 Goroutine 与后续 DB 更新竞态

**文件**: `services/terraform_executor.go`
**行号**: 1094-1119, 1302-1310

Plan 阶段异步 goroutine (line 1094) 执行 `ParseAndStorePlanChanges`（写入 DB），同时主线程在 line 1302 执行 `db.Model().Updates()`。两者可能并发写同一 task 记录。

主线程的 Updates 使用字段级更新（不覆盖 plan_data/plan_json），所以不会覆盖 goroutine 的数据，但 goroutine 的失败会被静默忽略。

**建议**: 使用 `sync.WaitGroup` 等待 goroutine 完成后再执行最终 DB 更新。

---

### M7. ExecuteConfirmedApply 无过期检查

**文件**: `services/task_queue_manager.go`
**行号**: ~1419-1424

仅检查 task 状态为 `apply_pending` 且 `ApplyConfirmedBy` 非空，不检查：
- Plan 是否过期（确认后可能已过很长时间）
- Workspace 是否仍锁定
- 幂等性（同一确认可能被执行多次）

**建议**: 增加过期时间检查和幂等保护。

---

### M8. Plan Hash 计算失败静默忽略

**文件**: `services/terraform_executor.go`
**行号**: 1055-1061

Hash 计算失败仅 Warn，`task.PlanHash` 保持空字符串。Apply 阶段的优化跳过逻辑（init/restoring_plan 跳过判断）依赖 PlanHash，空值导致永远无法跳过。

**影响**: 功能退化（性能优化失效），不影响正确性。

**建议**: 记录 metric 用于监控。

---

### M9. 通知发送使用 fmt.Printf 而非结构化日志

**文件**: `services/notification_sender.go`
**行号**: 646, 671

```go
fmt.Printf("Failed to send notification %s: %v\n", ...)
```

生产环境应使用结构化日志（`log.Printf` 或更好的 logging 框架），否则错误信息可能丢失。

---

### M10. RunTask 轮询超时后 Result 状态未更新

**文件**: `services/run_task_executor.go`
**行号**: 199-202

轮询超时返回 `(false, error)` 但不更新 `RunTaskResult` 记录状态。Result 保持 `running` 直到 `RunTaskTimeoutChecker` 定期扫描才处理。

**建议**: 超时时主动将对应 Result 标记为 `timeout`。

---

## 优化建议（非 Bug）

### O1. 统一 saveTaskFailure / saveTaskCancellation

两个函数逻辑高度相似但差异导致多个 Bug。建议合并为一个 `saveTaskTermination(task, logger, reason, taskType)` 函数，统一处理：
- 状态更新
- 日志保存
- 工作目录清理
- Workspace 解锁
- 通知发送
- 临界区退出

---

### O2. 使用 defer 统一临界区管理

`applying` 临界区应统一使用 `defer` 模式（与 `saving_state`/`saving_plan` 一致），消除手动管理导致的遗漏。

---

### O3. TaskQueueManager 后续操作增加 Outbox 模式

将 Run Triggers、CMDB 同步、通知等后续操作写入 outbox 表，由后台 worker 消费。好处：
- 持久化保证（crash 不丢）
- 自动重试
- 可审计
- 解耦顺序依赖

---

### O4. RunTask 执行结果增加 Webhook 失败状态

当前 `RunTaskResult.Status` 无法区分"外部服务检查失败"和"Webhook 发送失败"。建议增加 `webhook_error` 状态，让 UI 清晰展示根因。

---

### O5. Apply 成功但 State 保存失败的状态设计

当前将此场景标记为 `Failed`（line 2304），但实际 Terraform Apply 已成功完成。用户看到 Failed 会尝试重试 Apply，可能导致重复操作。

建议增加 `applied_state_error` 状态或在 error_message 中明确区分。

---

## 问题优先级总结

| ID | 严重度 | 分类 | 简述 |
|----|--------|------|------|
| C1 | CRITICAL | 资源泄漏 | applying 临界区错误路径不退出 |
| C2 | CRITICAL | 资源泄漏 | Apply 取消不解锁/不清理 |
| C3 | CRITICAL | 逻辑错误 | RunTask Webhook 失败误判为通过 |
| C4 | CRITICAL | 安全 | RunTask Callback 无认证 |
| C5 | CRITICAL | 功能缺失 | RunTask AccessToken 未生成 |
| C6 | CRITICAL | 功能缺失 | pre_plan/pre_apply/post_apply RunTask 未实装 |
| H1 | HIGH | 逻辑错误 | Plan 早期错误不更新 task 状态 |
| H2 | HIGH | 资源泄漏 | Plan 取消不清理工作目录 |
| H3 | HIGH | 数据丢失 | Mandatory RunTask 失败不保存日志 |
| H4 | HIGH | 健壮性 | Apply 输出 Goroutine 无 panic recovery |
| H5 | HIGH | 可靠性 | 后续操作全部 fire-and-forget 无重试 |
| H6 | HIGH | 可靠性 | CMDB 同步状态可永久卡死 |
| H7 | HIGH | 逻辑错误 | Workspace 锁定失败不阻断 |
| M1 | MEDIUM | 静默失败 | Plan JSON 生成失败继续执行 |
| M2 | MEDIUM | 静默失败 | SavePlanData 无错误返回 |
| M3 | MEDIUM | 资源 | 通知 Goroutine 无上限 |
| M4 | MEDIUM | 功能缺失 | 通知无重试实现 |
| M5 | MEDIUM | 功能缺失 | 取消通知缺失 |
| M6 | MEDIUM | 竞态 | 异步资源解析与 DB 更新竞态 |
| M7 | MEDIUM | 逻辑 | ConfirmedApply 无过期检查 |
| M8 | MEDIUM | 静默失败 | Plan Hash 计算失败不影响正确性但影响优化 |
| M9 | MEDIUM | 可观测性 | 通知错误用 fmt.Printf |
| M10 | MEDIUM | 数据一致性 | 轮询超时不更新 Result 状态 |
