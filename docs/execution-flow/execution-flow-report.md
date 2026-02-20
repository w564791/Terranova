# IAC Platform 核心执行流程报告

## 架构概览

平台基于 Terraform/OpenTofu 的 IaC 执行引擎，核心流程分为 **Plan 阶段** 和 **Apply 阶段**。Apply 不是独立流程，始终基于一个已完成的 Plan（通过 `plan_and_apply` 任务类型绑定，Apply 阶段从 Plan 任务中恢复 plan 数据和快照配置）。

**任务类型**:

| 类型 | 说明 |
|------|------|
| `plan_only` | 仅执行 Plan |
| `plan_and_apply` | Plan 完成后等待审批，再执行 Apply |
| `drift_check` | 特殊 Plan（`-refresh-only`），用于漂移检测 |

**执行模式**: Local（本地直接执行）、Agent（远程 Agent 代理）、K8s Agent

---

## 完整执行流水线

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                          PLAN 阶段 (ExecutePlan)                             │
│                                                                              │
│  ┌─────────┐   ┌──────┐   ┌──────────┐   ┌─────────────┐   ┌────────────┐  │
│  │ fetching │──>│ init │──>│ planning │──>│ saving_plan │──>│ post_plan  │  │
│  │          │   │      │   │          │   │             │   │ run_tasks  │  │
│  └─────────┘   └──────┘   └──────────┘   └─────────────┘   └────────────┘  │
│                                                                    │         │
│                                              ┌─────────────────────┘         │
│                                              v                               │
│                                   ┌──────────────────┐                       │
│                                   │  状态判定 & 通知   │                       │
│                                   └──────────────────┘                       │
│                                              │                               │
│                    ┌─────────────────────────-┤                               │
│                    v                          v                               │
│          无变更: planned_and_finished   有变更: apply_pending                  │
│                                         (自动锁定 workspace)                  │
│                                         [通知: approval_required]            │
└──────────────────────────────────────────────────────────────────────────────┘
                                              │
                                    [用户审批 / auto_apply]
                                              │
                                              v
┌──────────────────────────────────────────────────────────────────────────────┐
│                         APPLY 阶段 (ExecuteApply)                            │
│                                                                              │
│  ┌─────────┐   ┌──────┐   ┌───────────────┐   ┌──────────┐   ┌───────────┐ │
│  │ fetching │──>│ init │──>│restoring_plan │──>│ applying │──>│ saving_   │ │
│  │(可复用)  │   │(可跳)│   │   (可跳过)     │   │ [临界区]  │   │ state     │ │
│  └─────────┘   └──────┘   └───────────────┘   └──────────┘   │ [临界区]   │ │
│                                                                └───────────┘ │
│                                                                      │       │
│                                                                      v       │
│                                                           ┌──────────────┐   │
│                                                           │ 后续处理      │   │
│                                                           │ - 解锁 WS    │   │
│                                                           │ - 清理目录    │   │
│                                                           │ [通知]        │   │
│                                                           └──────────────┘   │
└──────────────────────────────────────────────────────────────────────────────┘
                                              │
                              [TaskQueueManager.OnTaskCompleted]
                                              │
                              ┌────────────────┼────────────────┐
                              v                v                v
                        CMDB 同步       Run Triggers      post_apply
                                      (触发下游 WS)      RunTasks
```

---

## Plan 阶段详细分解

> 源码位置: `backend/services/terraform_executor.go` — `ExecutePlan()` 函数

### Stage 1: `fetching`

| 子步骤 | 说明 |
|--------|------|
| 1.0 确保 IaC 引擎二进制 | 检测引擎类型（Terraform/OpenTofu），下载并校验指定版本二进制 |
| 1.1 准备工作目录 | 创建 `/tmp/iac-platform/workspaces/{workspaceID}/{taskID}` |
| 1.2 重新加载 Workspace 配置 | 刷新最新 workspace 配置（name, execution_mode, terraform_version） |
| 1.3 获取 Workspace Resources | 通过 DataAccessor 获取所有资源定义 |
| 1.4 获取 Workspace Variables | 获取 terraform 变量，区分普通/敏感变量 |
| 1.5 获取 Provider 配置 | 获取 Provider 配置（如 AWS region） |
| 1.6 获取最新 State Version | 获取最新状态版本，处理首次运行（无 state）场景 |
| 1.7 生成配置文件 | 生成 `main.tf.json`, `provider.tf.json`, `variables.tf.json`, `variables.tfvars`, `outputs.tf.json`, `remote_data.tf.json` |
| 1.8 准备 State 文件 | 恢复 state 到工作目录 |
| 1.9 恢复 Lock 文件 | 恢复 `.terraform.lock.hcl` 确保 provider 版本一致 |

- **RunTask**: 无
- **通知**: 无

### Stage 2: `init`

执行 `terraform init`，初始化 backend、下载 provider 和 module。

- **RunTask**: 无
- **通知**: 无

### Stage 3: `planning`

执行 `terraform plan -out=plan.out -no-color -var-file=variables.tfvars`

特殊处理:

- **Drift Check 模式**: 添加 `-refresh-only` flag
- 支持 `--target` 参数（从 task context 和 TF_CLI_ARGS 中提取）
- 实时捕获 stdout/stderr 输出并通过 WebSocket 推送
- 支持用户取消（通过 context cancellation）

- **RunTask**: 无
- **通知**: 无

### Stage 4: `saving_plan`

| 子步骤 | 说明 |
|--------|------|
| 计算 Plan Hash | 用于 Apply 阶段的优化跳过判断 |
| 生成 Plan JSON | `terraform show -json` 解析结构化计划 |
| 解析资源变更 | 提取 add/change/destroy 计数 |
| 保存 Plan 数据 | **临界区**: 保存 `plan_data`（二进制）和 `plan_json`（结构化），带 3 次重试 |
| 异步解析资源变更 | goroutine 异步调用 `PlanParserService.ParseAndStorePlanChanges()` |

- **RunTask**: 无
- **通知**: 无

### Stage 5: `post_plan_run_tasks`

**这是 Plan 阶段唯一执行 RunTask 的节点。**

```go
runTasksPassed, runTasksErr := s.executeRunTasksForStage(ctx, task, RunTaskStagePostPlan, logger)
```

| 结果 | 行为 |
|------|------|
| Mandatory 失败 | task 标记为 `Failed`，阻断后续 Apply |
| Advisory 失败 | 不阻断，用户可在 UI 手动 override |
| 执行错误 | task 标记为 `Failed` |

- **RunTask**: `post_plan`
- **通知**: 无（通知在后续状态判定后发送）

### 最终状态判定

| 条件 | 最终状态 | Stage 值 | 后续动作 |
|------|---------|----------|---------|
| `plan_and_apply` + 有变更 | `apply_pending` | `apply_pending` | 保留工作目录，自动锁定 workspace |
| `plan_and_apply` + 无变更 | `planned_and_finished` | `planned_and_finished` | 清理工作目录 |
| `plan_only` / `drift_check` | `success` | `completed` | 清理工作目录 |

变更判断逻辑:
- 统计 `add + change + destroy` 资源数量
- 额外检测 output 变更（通过 JSON deep comparison 判断实际值是否变化）

### Plan 阶段通知

Plan 完成后（仅 Local 模式），异步发送:

| 条件 | 通知事件 |
|------|---------|
| 状态为 `apply_pending`（有变更等待审批） | `approval_required` |
| 其他情况（plan_only 完成或无变更） | `task_planned` |

---

## Apply 阶段详细分解

> 源码位置: `backend/services/terraform_executor.go` — `ExecuteApply()` 函数
>
> Apply 始终基于已完成的 Plan 任务，从 `planTask` 恢复快照数据。

### Stage 1: `fetching`

与 Plan 的 fetching 不同，Apply 有两种路径:

| 场景 | 行为 |
|------|------|
| 同 Pod 执行（工作目录已存在） | 复用 Plan 阶段的工作目录，跳过配置生成 |
| 不同 Pod 执行（新建目录） | 从 planTask 快照恢复: 配置文件、state 文件、`.terraform.lock.hcl` |

关键点: 不同 Pod 场景下，所有配置从 Plan 任务的快照（snapshot）数据恢复，而非从当前 workspace 配置读取，确保 Apply 与 Plan 时的配置完全一致。

- **RunTask**: 无
- **通知**: 无

### Stage 2: `init` (可跳过)

跳过条件（全部满足时）:
1. Plan hash 存在且 Agent ID 已设置
2. 同一 agent（hostname 匹配）
3. Plan hash 本地校验通过

跳过时输出: `"Init stage skipped — same agent, plan hash verified"`

- **RunTask**: 无
- **通知**: 无

### Stage 3: `restoring_plan` (可跳过)

跳过条件:
1. init 已跳过（`canSkipInit = true`）
2. 本地 plan 文件存在
3. 本地 plan 文件 hash 与存储的 plan hash 一致

否则从 `planTask.PlanData` 恢复 `plan.out` 文件。

- **RunTask**: 无
- **通知**: 无

### Stage 4: `applying` [临界区]

执行 `terraform apply -no-color -auto-approve plan.out`

```
signalManager.EnterCriticalSection("applying")
  |
  |-- 构建环境变量
  |-- 启动 terraform apply 命令
  |-- 实时解析输出 (ApplyOutputParserWithAccessor):
  |     |-- 正则匹配: Creating/Modifying/Destroying/Complete
  |     |-- 实时更新 WorkspaceTaskResourceChange 表
  |     |-- WebSocket 广播资源状态变更
  |-- 等待命令完成
  |-- 提取 terraform outputs
  |
signalManager.ExitCriticalSection("applying")
```

- **RunTask**: 无
- **通知**: 无

### Stage 5: `saving_state` [临界区]

```
signalManager.EnterCriticalSection("saving_state")
```

| 子步骤 | 说明 |
|--------|------|
| 5.1 保存 Apply Output | 持久化 apply 输出日志到 `task_logs` 表 |
| 5.2 保存 State Version | 读取 `terraform.tfstate` → 计算校验和 → 备份到文件系统 → 创建 `WorkspaceStateVersion` 记录 → 更新 `workspace.tf_state`（**5 次重试 + 指数退避**；全部失败则自动锁定 workspace 并记录原因） |
| 5.3 提取资源详情 | 从 state JSON 解析资源 ID 等详情，更新 `WorkspaceTaskResourceChange` 记录 |
| 5.4 更新 Pending 资源 | 将所有 `apply_status=pending` 的资源标记为 `completed` |

- **RunTask**: 无
- **通知**: 无

### Stage 6: 后续处理

| 步骤 | 说明 |
|------|------|
| 更新 Task 状态 | `status=applied`, `stage=applied`, 记录 `completed_at` 和 `duration` |
| 解锁 Workspace | 调用 `dataAccessor.UnlockWorkspace()` |
| 清理工作目录 | 删除 `/tmp/iac-platform/workspaces/{workspaceID}/{taskID}` |
| 发送通知 | 异步发送 `task_completed` 通知 |

- **RunTask**: 无（post_apply RunTask 由 TaskQueueManager 处理）
- **通知**: `task_completed`

### Apply 失败路径

任何阶段失败时调用 `saveTaskFailure()`:

1. 设置 `task.Status = Failed`
2. 提取真实错误信息 (`extractRealError`)
3. 尝试保存部分 state（如果 apply 已部分执行）
4. 解锁 workspace
5. 保存日志到 `task_logs` 表
6. 发送 `task_failed` 通知

---

## RunTask 系统

> 源码位置:
> - 模型: `backend/internal/models/run_task.go`
> - 执行器: `backend/services/run_task_executor.go`
> - 回调处理: `backend/internal/handlers/run_task_callback_handler.go`

### 支持的 4 个阶段

| 阶段 | 时机 | 实现状态 |
|------|------|---------|
| `pre_plan` | Plan 执行前 | 模型已定义，**ExecutePlan 中未调用** |
| `post_plan` | Plan 完成后、Apply 决策前 | **已实现** — ExecutePlan 中调用 |
| `pre_apply` | Apply 执行前 | 模型已定义，可通过配置启用 |
| `post_apply` | Apply 完成后 | 由 `TaskQueueManager.OnTaskCompleted` 处理 |

### 执行级别

| 级别 | 行为 |
|------|------|
| `advisory` | 失败不阻断执行，记录警告，用户可在 UI override |
| `mandatory` | 失败阻断后续执行，task 标记为 Failed |

### RunTask 配置层级

| 层级 | 说明 |
|------|------|
| 全局 RunTask | `is_global=true`，自动应用到所有 workspace，通过 `global_stages` 指定阶段 |
| Workspace RunTask | 关联到特定 workspace，可单独配置 stage 和 enforcement_level |

去重规则: 如果 workspace 已配置某个 RunTask，则跳过该 RunTask 的全局配置。

### 执行流程

```
1. 查找 RunTask 配置
   ├── Workspace 级: workspace_run_tasks WHERE workspace_id=? AND stage=? AND enabled=true
   └── 全局级: run_tasks WHERE is_global=true AND global_stages LIKE %stage%

2. 创建 RunTaskResult 记录 (status=pending)
   ├── 生成唯一 result_id: "rtr-{16-char-random}"
   └── 生成一次性 JWT access_token (供外部服务访问平台数据)

3. 发送 Webhook
   ├── HTTP POST -> EndpointURL
   ├── Header: Content-Type: application/json
   ├── Header: X-TFC-Task-Signature: sha512={HMAC-SHA512 签名}
   └── Payload:
       ├── task: id, type, status, created_by, created_at
       ├── workspace: id, name, terraform_version, execution_mode
       ├── plan_data_url (post_plan/pre_apply/post_apply 阶段)
       ├── plan_changes: add/change/destroy 计数
       └── resource_changes_url

4. 轮询回调 (2s 间隔, 最长 10 分钟)
   ├── 外部服务通过 PATCH /api/v1/run-task-results/{result_id}/callback 报告结果
   ├── 支持 outcomes 详细检查结果 (TFE 兼容 JSON:API 格式)
   └── 状态: pending -> running -> passed/failed/error/timeout

5. 结果判定
   ├── 所有 mandatory 通过 -> passed=true
   ├── 任一 mandatory 失败 -> passed=false (阻断执行)
   └── advisory 失败 -> passed=true (不阻断, 可 override)
```

### 回调 Payload 格式 (TFE 兼容)

```json
{
  "data": {
    "type": "task-results",
    "attributes": {
      "status": "passed|failed|running",
      "message": "结果描述",
      "url": "外部服务详情链接"
    },
    "relationships": {
      "outcomes": {
        "data": [
          {
            "type": "task-result-outcomes",
            "attributes": {
              "outcome-id": "PRTNR-CC-TF-127",
              "description": "单行描述",
              "body": "Markdown 格式详情 (建议 < 1MB, 最大 5MB)",
              "tags": {
                "Status": [{"label": "Failed", "level": "error"}]
              }
            }
          }
        ]
      }
    }
  }
}
```

---

## 通知系统

> 源码位置:
> - 模型: `backend/internal/models/notification.go`
> - 发送器: `backend/services/notification_sender.go`

### 支持的事件

| 事件 | 触发时机 | 触发位置 |
|------|---------|---------|
| `task_created` | Task 创建时 | Controller |
| `task_planning` | Planning 开始 | — |
| `task_planned` | Plan 完成（无变更或 plan_only） | `ExecutePlan` 末尾 |
| `approval_required` | Plan 有变更，等待审批 | `ExecutePlan` 末尾 |
| `task_applying` | Apply 开始 | — |
| `task_completed` | Apply 成功完成 | `ExecuteApply` 末尾 |
| `task_failed` | 任务失败（任何阶段） | `saveTaskFailure()` |
| `task_cancelled` | 任务取消 | — |
| `approval_timeout` | 审批超时 | — |
| `drift_detected` | 检测到漂移 | — |

### 投递方式

| 类型 | 签名方式 | 特点 |
|------|---------|------|
| `webhook` | HMAC-SHA256 (`X-IaC-Signature: sha256=...`) | 通用 HTTP POST，支持自定义 Headers |
| `lark_robot` | HMAC-SHA256 (飞书格式) | 飞书机器人，交互卡片，颜色编码（绿=成功，红=失败，橙=警告） |

### 通知配置层级

| 层级 | 说明 |
|------|------|
| 全局通知 | `is_global=true`，自动应用到所有 Workspace |
| Workspace 通知 | 覆盖全局配置，可单独启停、配置事件过滤 |

去重规则: 与 RunTask 相同，workspace 已配置则跳过全局。

### Webhook Payload 格式

```json
{
  "event": "task_completed",
  "timestamp": "2024-01-01T12:00:00Z",
  "task": {
    "id": 123,
    "type": "plan_and_apply",
    "status": "applied",
    "description": "...",
    "created_by": "user123",
    "created_at": "...",
    "app_url": "https://platform.example.com/workspaces/ws123/tasks/123"
  },
  "workspace": {
    "id": "ws123",
    "name": "production",
    "terraform_version": "1.5.0",
    "app_url": "https://platform.example.com/workspaces/ws123"
  }
}
```

### 重试机制

- 默认重试次数: 3
- 重试间隔: 30s
- 请求超时: 30s
- 所有投递记录持久化到 `notification_logs` 表（含 request/response 详情）

---

## Run Triggers（跨 Workspace 联动）

> 源码位置:
> - 模型: `backend/internal/models/run_trigger.go`
> - 服务: `backend/services/run_trigger_service.go`

### 触发条件

**仅在源 Workspace 的 Apply 成功完成 (`TaskStatusApplied`) 后触发。**

### 执行流程

```
源 Workspace Apply 成功
       |
       v
TaskQueueManager.OnTaskCompleted()
       |
       v
RunTriggerService.ExecuteTriggers()
       |
       |-- 查询 pending 的 TaskTriggerExecution 记录
       |-- 对每个 trigger:
       |     |-- 检查是否被用户临时禁用 -> skipped
       |     |-- 检查 trigger 是否仍启用 -> skipped if disabled
       |     |-- 创建下游 task:
       |     |     type: plan_and_apply
       |     |     status: pending
       |     |     description: "Triggered by workspace {source_id} (task #{id})"
       |     |-- 更新执行记录: status=triggered, target_task_id=新任务ID
       |     |-- 通知 QueueManager: TryExecuteNextTask(targetWorkspaceID)
       |
       v
下游 Workspace 开始执行 Plan -> Apply 流程 (递归)
```

### 循环检测

- 使用 BFS 算法检测触发链中的循环依赖
- `wouldCreateCycle()` 在创建 trigger 配置时调用
- 防止 A -> B -> C -> A 的无限触发链

### 用户控制

| 控制方式 | 说明 |
|---------|------|
| 全局启停 | RunTrigger 的 `Enabled` 字段 |
| 临时禁用 | `TaskTriggerExecution.TemporarilyDisabled`，Apply 前在 UI 可操作 |
| 预创建执行记录 | `PrepareTaskTriggerExecutions()` 在 task 创建时调用，用户可在 Apply 前查看和控制 |

---

## 完整时间线总结

```
1. Task Created (status: pending)
   |-- PrepareTaskTriggerExecutions() -- 创建待执行的 trigger 记录
   |-- [通知: task_created]

2. Plan 阶段 (ExecutePlan)
   |
   |-- [pre_plan RunTask]     <-- 模型已定义, 代码未调用
   |
   |-- Stage: fetching        -- 下载二进制、准备目录、获取配置、生成文件
   |-- Stage: init            -- terraform init
   |-- Stage: planning        -- terraform plan
   |-- Stage: saving_plan     -- 保存 plan 数据 [临界区]
   |
   |-- [post_plan RunTask]    <-- 已实现, mandatory 可阻断
   |
   |-- 状态判定:
   |     |-- 有变更 -> apply_pending (锁定 workspace)
   |     |-- 无变更 -> planned_and_finished
   |     |-- plan_only -> success
   |
   |-- [通知: approval_required 或 task_planned]

3. 等待审批 (如果 apply_pending)
   |-- 用户在 UI 确认 Apply
   |-- 或 auto_apply 自动通过

4. Apply 阶段 (ExecuteApply)
   |
   |-- [pre_apply RunTask]    <-- 可通过配置启用
   |
   |-- Stage: fetching        -- 复用或重建工作目录
   |-- Stage: init            -- terraform init (可跳过)
   |-- Stage: restoring_plan  -- 恢复 plan.out (可跳过)
   |-- Stage: applying        -- terraform apply [临界区]
   |-- Stage: saving_state    -- 保存 state [临界区]
   |
   |-- 解锁 workspace
   |-- 清理工作目录
   |-- [通知: task_completed]

5. 后续处理 (TaskQueueManager.OnTaskCompleted)
   |
   |-- [post_apply RunTask]   <-- 可通过配置启用
   |-- CMDB 同步              -- syncCMDBAfterApply()
   |-- Run Triggers           -- 触发下游 Workspace 的 plan_and_apply

6. 失败路径 (任何阶段)
   |-- saveTaskFailure()
   |-- 尝试保存部分 state (如果 apply 已部分执行)
   |-- 解锁 workspace
   |-- [通知: task_failed]
```

---

## 关键发现与待办

### 1. `pre_plan` RunTask 未实装

模型层已定义 `RunTaskStagePrePlan`，但 `ExecutePlan` 中没有调用 `executeRunTasksForStage(pre_plan)`。

**影响**: 用户配置了 `pre_plan` 阶段的 RunTask 不会被执行。

**建议**: 如需支持 Plan 前的安全扫描/策略检查，在 `planning` stage 之前添加:

```go
// 在 Stage 3 planning 之前
logger.StageBegin("pre_plan_run_tasks")
runTasksPassed, runTasksErr := s.executeRunTasksForStage(ctx, task, models.RunTaskStagePrePlan, logger)
// ... 处理结果
logger.StageEnd("pre_plan_run_tasks")
```

### 2. `pre_apply` / `post_apply` RunTask 调用点确认

- `pre_apply`: 需确认 `ExecuteApply` 中是否在 `applying` stage 之前调用
- `post_apply`: 当前由 `TaskQueueManager.OnTaskCompleted` 处理，与 ExecuteApply 解耦

### 3. 通知仅 Local 模式直接发送

Agent/K8s 模式下，通知依赖 Server 端 `OnTaskCompleted` 回调触发。需确认所有事件类型在 Agent 模式下覆盖完整。

### 4. CMDB 同步和 Run Triggers 统一在 Server 端

不在 `ExecuteApply` 内部处理，而是通过 `TaskQueueManager.OnTaskCompleted` 回调统一处理，确保 Local/Agent/K8s 三种执行模式行为一致。

---

## 源码文件索引

| 组件 | 文件路径 |
|------|---------|
| **核心执行器** | `backend/services/terraform_executor.go` |
| **任务队列管理** | `backend/services/task_queue_manager.go` |
| **RunTask 模型** | `backend/internal/models/run_task.go` |
| **RunTask 执行器** | `backend/services/run_task_executor.go` |
| **RunTask 回调处理** | `backend/internal/handlers/run_task_callback_handler.go` |
| **通知模型** | `backend/internal/models/notification.go` |
| **通知发送器** | `backend/services/notification_sender.go` |
| **Run Trigger 模型** | `backend/internal/models/run_trigger.go` |
| **Run Trigger 服务** | `backend/services/run_trigger_service.go` |
