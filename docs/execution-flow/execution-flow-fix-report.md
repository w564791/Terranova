# 执行流程问题修复报告

**分支**: `fix/execution-flow-issues`
**基于**: `docs/execution-flow-issues.md` 系统性审查
**修复范围**: 全部 6 个 CRITICAL + 关键 HIGH 问题（H1-H4, H7）

---

## 修复总览

| ID | 严重度 | 状态 | 简述 |
|----|--------|------|------|
| C1 | CRITICAL | ✅ 已修复 | applying 临界区改用 defer |
| C2 | CRITICAL | ✅ 已修复 | Apply 取消时解锁 workspace、清理目录、保存 partial state |
| C3 | CRITICAL | ✅ 已修复 | RunTask Webhook 失败时检查 mandatory enforcement |
| C4 | CRITICAL | ✅ 已修复 | Callback/数据端点添加 Bearer Token 验证 |
| C5 | CRITICAL | ✅ 已修复 | RunTaskResult 创建后生成 JWT access_token |
| C6 | CRITICAL | ✅ 已修复 | 补齐 pre_plan / pre_apply / post_apply 三个 RunTask 阶段 |
| H1 | HIGH | ✅ 已修复 | Plan 早期错误（GetWorkspace / PrepareWorkspace）调用 saveTaskFailure |
| H2 | HIGH | ✅ 已修复 | Plan 取消时清理工作目录（合入 C2 saveTaskCancellation 增强） |
| H3 | HIGH | ✅ 已修复 | Mandatory RunTask 失败改用 saveTaskFailure 保存完整日志 |
| H4 | HIGH | ✅ 已修复 | Apply stdout/stderr goroutine 添加 panic recovery |
| H7 | HIGH | ✅ 已修复 | Workspace 锁定失败作为错误处理，阻断任务 |

### 未修复（留待后续迭代）

| ID | 严重度 | 简述 | 原因 |
|----|--------|------|------|
| H5 | HIGH | 后续操作 fire-and-forget 无重试 | 需引入 outbox 模式，架构改动较大 |
| H6 | HIGH | CMDB 同步状态可永久卡死 | 需增加 timeout 字段和定期检查 worker |
| M1-M10 | MEDIUM | 各项 medium 问题 | 不影响核心正确性，优先级排后 |

---

## 涉及文件

| 文件 | 改动摘要 |
|------|---------|
| `backend/services/terraform_executor.go` | C1 defer 临界区; C2/H2/M5 saveTaskCancellation 增强; H1 早期错误; C6 三个新 RunTask stage; H3 mandatory 日志; H4 panic recovery; H7 锁定失败 |
| `backend/services/run_task_executor.go` | C5 token 生成; C3 webhook 失败 mandatory 检查; token 清理; 暴露 GetTokenService() |
| `backend/services/run_task_timeout_checker.go` | 超时时清理 access_token |
| `backend/internal/handlers/run_task_callback_handler.go` | C4 Bearer token 验证; 新增 GetPlanJSON / GetResourceChanges handler |
| `backend/internal/router/router_run_task.go` | 新增公开 GET 路由 plan-json / resource-changes |

---

## 详细修复说明

### C1: applying 临界区改用 defer

**问题**: `EnterCriticalSection("applying")` 后，仅在成功路径手动调用 `ExitCriticalSection`。所有错误/取消 return 路径都不退出临界区。

**修复**: 在 `EnterCriticalSection` 后紧跟 `defer ExitCriticalSection`，删除原有手动调用。

```go
// 修复后
s.signalManager.EnterCriticalSection("applying")
defer s.signalManager.ExitCriticalSection("applying")
```

**影响范围**: `ExecuteApply()` 函数内所有提前 return 的路径（约 8 处）。

---

### C2 + H2 + M5: saveTaskCancellation 全面增强

**问题**: `saveTaskCancellation` 仅更新 task 状态和保存日志。对比 `saveTaskFailure`，缺少：
- Workspace 解锁（Apply 取消时）
- Partial state 保存（Apply 取消时）
- 工作目录清理（Plan 和 Apply 取消时）
- 取消通知发送

**修复**: 对齐 `saveTaskFailure` 的完整清理逻辑：

| 操作 | Plan 取消 | Apply 取消 |
|------|----------|-----------|
| 更新 task 状态为 cancelled | ✅ | ✅ |
| 保存输出到 task_logs | ✅ | ✅ |
| 清理工作目录 | ✅ **新增** | ✅ **新增** |
| 解锁 workspace | — | ✅ **新增** |
| 保存 partial state | — | ✅ **新增** |
| 发送 task_cancelled 通知 | ✅ **新增** | ✅ **新增** |

---

### C3: RunTask Webhook 全部失败时检查 mandatory enforcement

**问题**: `ExecuteRunTasksForStage` 中，webhook 发送失败的结果进入 `execErrors` channel 但丢失了 enforcement 信息。当所有 webhook 均失败时，`pendingResultIDs` 为空，直接返回 `(true, nil)` — mandatory RunTask 的安全检查被完全绕过。

**修复**: 重构为单一 channel 传递完整的 `runTaskExecResult`（含 enforcement 字段）。遍历结果时，对失败的 mandatory RunTask 立即返回 `(false, error)` 阻断执行。

```go
// 修复后：单一 channel 保留完整结果
allResults := make(chan *runTaskExecResult, len(workspaceRunTasks))

for result := range allResults {
    if result.err != nil {
        if result.enforcement == models.RunTaskEnforcementMandatory {
            return false, fmt.Errorf("mandatory run task %s webhook failed: %v", ...)
        }
    } else {
        pendingResultIDs = append(pendingResultIDs, result.resultID)
    }
}
```

---

### C4 + C5: RunTask Token 认证体系

**修复前状态**: Callback 端点完全无认证；`AccessToken` 字段存在但从未生成（webhook payload 中发送空字符串）；数据端点（plan-json, resource-changes）指向 JWT 保护的内部路由，外部 RunTask 服务无法访问。

**修复后认证架构**:

```
平台 → 外部 RunTask 服务:
  1. 出站认证: HMAC-SHA512 签名 (X-TFC-Task-Signature header)
  2. Webhook payload 携带 JWT access_token

外部 RunTask 服务 → 平台:
  3. Callback 认证: Bearer Token (JWT)
  4. 数据端点认证: Bearer Token (JWT)
```

#### Token 生命周期

```
┌──────────────────────────────────────────────────────────────────────┐
│                      Token 生命周期                                   │
│                                                                      │
│  创建 RunTaskResult                                                   │
│       │                                                              │
│       ▼                                                              │
│  生成 JWT access_token ──────────────────────────────────────────┐   │
│  (有效期 = min(TimeoutSeconds, 24h))                             │   │
│       │                                                          │   │
│       ▼                                                          │   │
│  保存到 RunTaskResult.AccessToken                                 │   │
│       │                                                          │   │
│       ▼                                                          │   │
│  发送 Webhook (payload 含 access_token)                           │   │
│       │                                                          │   │
│       ├─── 外部服务用 token 调用:                                  │   │
│       │      PATCH callback (Bearer token)                       │   │
│       │      GET  plan-json (Bearer token)                       │   │
│       │      GET  resource-changes (Bearer token)                │   │
│       │                                                          │   │
│       ▼                                                          │   │
│  回调完成 (passed/failed)                                         │   │
│       │                                                          │   │
│       ▼                                                          │   │
│  清理 token: AccessToken="" , AccessTokenUsed=true               │   │
│                                                                  │   │
│  ── 或 ──                                                        │   │
│                                                                  │   │
│  超时 (RunTaskTimeoutChecker)                                     │   │
│       │                                                          │   │
│       ▼                                                          │   │
│  清理 token: AccessToken="" , AccessTokenUsed=true               │   │
└──────────────────────────────────────────────────────────────────────┘
```

#### JWT Token Claims 结构

```go
type RunTaskTokenClaims struct {
    ResultID    string   // 绑定到具体 RunTaskResult
    TaskID      uint     // 关联的 WorkspaceTask
    WorkspaceID string   // 关联的 Workspace
    Stage       string   // RunTask 阶段
    jwt.RegisteredClaims // exp, iat, nbf, iss, sub, jti
}
```

- **签名算法**: HMAC-SHA256
- **密钥来源**: 复用平台统一的 `JWT_SECRET` 环境变量（通过 `config.GetJWTSecret()`）
- **存储方式**: AES-256-GCM 加密存储在 `run_task_results.access_token` 字段（复用 `crypto.EncryptValue`，密钥派生自 `JWT_SECRET`）。发送 webhook 前解密（`crypto.DecryptValue`），回调完成或超时后清空
- **验证逻辑**: 校验签名 → 校验过期 → 校验 claims.ResultID 与 URL path 参数匹配

#### 端点认证对照

| 端点 | 修复前 | 修复后 |
|------|--------|--------|
| `PATCH /api/v1/run-task-results/:id/callback` | 无认证 | Bearer Token (JWT) |
| `POST /api/v1/run-task-results/:id/callback` | 无认证 | Bearer Token (JWT) |
| `GET /api/v1/run-task-results/:id` | 无认证 | 无认证（只读，无敏感数据） |
| `GET /api/v1/run-task-results/:id/plan-json` | **不存在** | **新增**, Bearer Token (JWT) |
| `GET /api/v1/run-task-results/:id/resource-changes` | **不存在** | **新增**, Bearer Token (JWT) |

#### Webhook Payload URL 变更

修复前 payload 中的数据 URL 指向 JWT 保护的内部路由，外部服务无法访问：

```json
// 修复前（不可用）
"plan_json_api_url": "{base}/api/v1/workspaces/{ws_id}/tasks/{task_id}/plan-json"
"resource_changes_api_url": "{base}/api/v1/workspaces/{ws_id}/tasks/{task_id}/resource-changes"

// 修复后（Token 认证的公开路由）
"plan_json_api_url": "{base}/api/v1/run-task-results/{result_id}/plan-json"
"resource_changes_api_url": "{base}/api/v1/run-task-results/{result_id}/resource-changes"
```

#### Token 清理时机

| 事件 | 清理动作 |
|------|---------|
| Callback 状态为 passed/failed | `AccessToken=""`, `AccessTokenUsed=true` |
| RunTaskTimeoutChecker 标记为 timeout | `AccessToken=""`, `AccessTokenUsed=true` |
| JWT 自然过期 | Token 校验失败，等效于无效 |

---

### C6: 补齐 pre_plan / pre_apply / post_apply RunTask 阶段

**问题**: 模型层定义了 4 个阶段，但仅 `post_plan` 有实际调用。

**修复后完整 RunTask 阶段**:

```
ExecutePlan:
  fetching → init → [pre_plan RunTasks] → planning → saving_plan → [post_plan RunTasks] → 状态判定

ExecuteApply:
  fetching → init → restoring_plan → [pre_apply RunTasks] → applying → saving_state → 后续处理 → [post_apply RunTasks] → 通知
```

| 阶段 | 位置 | mandatory 失败行为 |
|------|------|-------------------|
| `pre_plan` | ExecutePlan: init 完成后、planning 开始前 | 调用 saveTaskFailure，阻断 plan |
| `post_plan` | ExecutePlan: saving_plan 完成后 | 调用 saveTaskFailure，阻断 apply |
| `pre_apply` | ExecuteApply: restoring_plan 完成后、applying 开始前 | 调用 saveTaskFailure，阻断 apply |
| `post_apply` | ExecuteApply: task 标记为 applied 后、通知发送前 | **仅记录警告，不回滚 apply** |

`post_apply` 的特殊处理：apply 已经完成，基础设施已变更，mandatory 失败不应回滚已完成的 apply 状态。

---

### H1: Plan 早期错误调用 saveTaskFailure

**问题**: 两个早期错误路径直接 `return err`，task 卡在 running 状态。

| 位置 | 错误场景 | 修复前 | 修复后 |
|------|---------|--------|--------|
| ExecutePlan: `GetWorkspace()` 失败 | workspace 不存在或 DB 错误 | 直接 `return err`，task 卡在 running | 手动更新 task 为 failed（logger 尚未创建） |
| ExecutePlan: `PrepareWorkspace()` 失败 | 磁盘空间不足等 | 直接 `return err`，task 卡在 running | 调用 `saveTaskFailure()` |
| ExecuteApply: `PrepareWorkspace()` 失败 | 同上 | 直接 `return err` | 调用 `saveTaskFailure()` |

注意：`GetWorkspace()` 失败时 logger 尚未创建，无法调用 `saveTaskFailure()`，因此直接用 `dataAccessor.UpdateTask()` 更新状态。

---

### H3: Mandatory RunTask 失败保存完整日志

**问题**: `post_plan` 阶段 mandatory RunTask 失败时，直接调用 `UpdateTask` 设置 failed，不保存 `task_logs`、不发送通知。

**修复**: 改为调用 `saveTaskFailure()`，统一处理：
- 保存完整输出到 task_logs
- 发送 task_failed 通知
- Plan 类型自动清理工作目录

---

### H4: Apply 输出 Goroutine 添加 panic recovery

**问题**: stdout/stderr reader goroutine 中 `applyParser.ParseLine()` 如果 panic，goroutine 静默终止，后续输出丢失。

**修复**: 在两个 goroutine 中均添加 `defer recover()`。

---

### H7: Workspace 锁定失败作为错误处理

**问题**: Plan 完成（有变更）后锁定 workspace 失败仅记录 Warning，task 仍设为 `apply_pending`。用户可在 Plan-Apply 间隙修改配置，导致 Apply 与实际配置不一致。

**修复**: 锁定失败时调用 `saveTaskFailure()`，阻断任务。

---

## 验证结果

```
go build ./...          — 通过
go test ./services/     — 全部通过
go test ./controllers/  — 预存失败（与本次修改无关）
go test ./internal/...  — 无测试文件
```

---

## 手动验证清单

- [ ] Plan 取消 → workspace 解锁、工作目录清理、收到 task_cancelled 通知
- [ ] Apply 取消 → workspace 解锁、partial state 保存、工作目录清理、收到 task_cancelled 通知
- [ ] Apply 错误路径 → 临界区正确退出（日志中确认 ExitCriticalSection）
- [ ] RunTask webhook payload 包含有效 access_token（非空 JWT）
- [ ] RunTask callback 无 token → 401
- [ ] RunTask callback 有效 token → 200
- [ ] RunTask callback 错误 token → 401
- [ ] RunTask callback token 的 result_id 不匹配 → 403
- [ ] GET plan-json 有效 Bearer token → 200 + plan_json 数据
- [ ] GET resource-changes 有效 Bearer token → 200 + resource_changes 数据
- [ ] GET plan-json 无 token → 401
- [ ] Callback 完成后 token 被清理（AccessTokenUsed=true）
- [ ] 所有 webhook 失败 + mandatory → plan/apply 被阻断
- [ ] pre_plan RunTask → 在 init 后、planning 前执行
- [ ] pre_apply RunTask → 在 restoring_plan 后、applying 前执行
- [ ] post_apply RunTask → 在 apply 成功后执行，mandatory 失败不回滚 apply
- [ ] Plan 中 GetWorkspace 失败 → task 状态更新为 failed（不卡在 running）
- [ ] Plan 中 PrepareWorkspace 失败 → task 状态更新为 failed
- [ ] Workspace 锁定失败 → task 标记为 failed，不进入 apply_pending
