# RunTask 优先级与审批流设计方案

> **状态**: 未实现
>
> **关联**: [execution-flow-issues.md](./execution-flow-issues.md) | [execution-flow-fix-report.md](./execution-flow-fix-report.md)

## 1. 背景

当前 RunTask 同一 stage 内的所有任务**全部并行**执行，没有顺序控制。这导致审批类 RunTask 无法看到其他检查类 RunTask 的结果就被触发，失去了"基于检查结果做审批决策"的语义。

### 1.1 当前流程

```
post_plan stage:
  ┌─ Security Scan (advisory)   ──→ webhook ──→ callback
  ├─ Compliance Check (advisory) ──→ webhook ──→ callback
  └─ Approval Gate (mandatory)  ──→ webhook ──→ callback
  全部并行发出，审批方收到 webhook 时 Security Scan 和 Compliance Check 可能还未完成
```

### 1.2 期望流程

```
post_plan stage:
  Batch 0 (priority=0): 并行
    ├─ Security Scan (advisory)    ──→ webhook ──→ callback (passed)
    └─ Compliance Check (advisory) ──→ webhook ──→ callback (failed)

  等待 Batch 0 全部完成 ✓

  Batch 1 (priority=1): 并行
    └─ Approval Gate (mandatory)   ──→ webhook (携带前序结果摘要) ──→ callback
```

审批方收到 webhook 时，能看到 Security Scan passed、Compliance Check failed，基于完整上下文做审批决策。

---

## 2. Model 变更

### 2.1 WorkspaceRunTask / RunTask 新增 priority 字段

| Model | 字段 | 类型 | 默认值 | 说明 |
|-------|------|------|--------|------|
| `WorkspaceRunTask` | `Priority` | `int` | `0` | Workspace 级别执行优先级 |
| `RunTask` | `GlobalPriority` | `int` | `0` | 全局 RunTask 默认优先级 |

- 0 为最高优先级，数值越大优先级越低
- 相同优先级的 RunTask 在同一批次内并行执行
- 虚拟 WorkspaceRunTask（全局 RunTask 转换）继承 `GlobalPriority`

### 2.2 RunTask 关联 IAM Application

| Model | 字段 | 类型 | 说明 |
|-------|------|------|------|
| `RunTask` | `ApplicationID` | `*uint` | 关联的 IAM Application ID（可选） |

当 `ApplicationID` 非空时，该 RunTask 的 token 基于 Application 身份签发，权限由 Application 绑定的 IAM Role 控制。

### 2.3 RunTaskResult 新增 priority 记录

| Model | 字段 | 类型 | 说明 |
|-------|------|------|------|
| `RunTaskResult` | `Priority` | `int` | 记录执行时的优先级，用于 prior-results 查询 |

### 2.4 RunTaskTokenClaims 扩展

| 字段 | 类型 | 说明 |
|------|------|------|
| `result_id` | `string` | 现有 |
| `task_id` | `uint` | 现有 |
| `workspace_id` | `string` | 现有 |
| `stage` | `string` | 现有 |
| `app_id` | `*uint` | **新增**，关联的 Application ID（无关联时为空） |
| `scope` | `[]string` | **新增**，权限范围列表，由 IAM Role 决定 |

### 2.5 DB Migration

```sql
-- Priority
ALTER TABLE workspace_run_tasks ADD COLUMN priority int NOT NULL DEFAULT 0;
ALTER TABLE run_tasks ADD COLUMN global_priority int NOT NULL DEFAULT 0;
ALTER TABLE run_task_results ADD COLUMN priority int NOT NULL DEFAULT 0;

-- IAM Application 关联
ALTER TABLE run_tasks ADD COLUMN application_id int REFERENCES applications(id);
CREATE INDEX idx_run_tasks_application_id ON run_tasks(application_id);
```

现有数据 `priority=0`，`application_id=NULL`，行为不变，**向后兼容**。

---

## 3. 执行引擎变更

### 3.1 按优先级分批执行

`ExecuteRunTasksForStage` 重构为分批循环：

```
ExecuteRunTasksForStage(ctx, task, stage) (bool, error):
    workspaceRunTasks = 查询并按 priority ASC 排序
    batches = groupByPriority(workspaceRunTasks)
    priorResults = []  // 前序批次结果累积

    for each batch in batches (按 priority 从小到大):
        passed, batchResults, err = executeBatch(ctx, task, stage, batch, priorResults)
        if err != nil:
            return false, err
        if !passed:
            return false, nil
        priorResults = append(priorResults, batchResults...)
    return true, nil
```

涉及函数：
- `ExecuteRunTasksForStage` — 重构为分批循环
- `groupByPriority(wrts []WorkspaceRunTask) [][]WorkspaceRunTask` — 新增
- `executeBatch(ctx, task, stage, batch, priorResults) (bool, []RunTaskResult, error)` — 新增，当前并行逻辑下沉到此函数

### 3.2 批次内逻辑不变

每个批次内部与当前 `ExecuteRunTasksForStage` 一致：
1. 并行发送 webhook（携带 `priorResults` 摘要）
2. 收集 webhook 发送结果
3. `waitForCallbacks` 等待所有 callback
4. mandatory 失败 → 阻断后续批次

### 3.3 前序结果注入 Webhook Payload

当 `priorResults` 非空时（即 priority > 0 的批次），webhook payload 新增：

| 字段 | 类型 | 说明 |
|------|------|------|
| `prior_results` | `[]object` | 前序批次结果摘要（内联） |
| `prior_results_api_url` | `string` | 完整前序结果查询 URL（token 认证） |

`prior_results` 摘要结构：

```json
[
  {
    "run_task_name": "Security Scan",
    "result_id": "rtr-xxx",
    "enforcement_level": "advisory",
    "status": "passed",
    "message": "All checks passed",
    "outcomes_summary": { "total": 12, "passed": 12, "failed": 0 }
  }
]
```

涉及函数：
- `buildWebhookPayload` — 增加 `priorResults []RunTaskResult` 参数
- `buildPriorResultsSummary(results []RunTaskResult) []map[string]interface{}` — 新增

---

## 4. Token 与 IAM 集成

### 4.1 现有 IAM 体系概述

平台已有完整的 IAM 体系，Application 是一等主体：

- `Application` — 应用实体（`AppKey` + `AppSecret`），归属组织
- `PrincipalType` — `USER` / `TEAM` / `APPLICATION`，Application 可在组织级别被授予角色
- `Role` → `RolePolicy` → `ResourceType` + `PermissionLevel` + `ScopeType`
- `UserRole` — 角色绑定（虽然叫 UserRole，但 `UserID` 字段可存 Application 的标识）

### 4.2 RunTask Token 签发策略

#### 4.2.1 无 Application 关联（`RunTask.ApplicationID = NULL`）

保持现有机制不变：
- 签名密钥：全局 `JWT_SECRET`
- Claims：`result_id`, `task_id`, `workspace_id`, `stage`
- 权限范围：硬编码，仅能访问自身 result 相关端点
- 适用场景：简单的检查类 RunTask，不需要额外权限

#### 4.2.2 有 Application 关联（`RunTask.ApplicationID != NULL`）

基于 Application 身份签发：
- 签名密钥：Application 独立的 `AppSecret`（从 `applications` 表读取，已加密存储）
- Claims 扩展：新增 `app_id` 和 `scope`
- 权限范围：由 Application 绑定的 IAM Role 决定，通过 `scope` claim 声明
- 适用场景：审批类 RunTask，需要读取前序结果、workspace 变量等

Token 签发流程：

```
GenerateAccessToken(resultID, taskID, workspaceID, stage, runTask):
    if runTask.ApplicationID == nil:
        // 走现有简单 JWT 路径
        return signWithJWTSecret(basicClaims)

    app = loadApplication(runTask.ApplicationID)
    if !app.IsActive || app.IsExpired():
        return error("application inactive or expired")

    // 从 IAM 查询 Application 的权限范围
    scope = resolveApplicationScope(app.ID, workspaceID)

    claims = RunTaskTokenClaims{
        ...basicClaims,
        AppID: app.ID,
        Scope: scope,  // e.g. ["TASK_DATA_ACCESS:READ", "RUN_TASK_RESULTS:READ"]
    }
    return signWithAppSecret(claims, app.AppSecret)
```

涉及函数：
- `RunTaskTokenService.GenerateAccessToken` — 判断是否关联 Application，选择签名方式
- `RunTaskTokenService.ValidateAccessToken` — 支持两种 token 格式验证
- `resolveApplicationScope(appID uint, workspaceID string) []string` — 新增，查询 Application 的 IAM 角色权限

### 4.3 Token 权限范围定义

新增 `ResourceType`：

| ResourceType | 说明 | Scope |
|-------------|------|-------|
| `RUN_TASK_RESULTS` | RunTask 结果访问 | ORGANIZATION |

权限级别对照：

| PermissionLevel | 能力 |
|----------------|------|
| `READ` | 读取自身 result 的 plan-json、resource-changes；读取同 task 同 stage 的前序 results + outcomes |
| `WRITE` | READ + callback 回写能力（当前所有 RunTask 都需要） |

预置权限组合示例：

| 角色 | RUN_TASK_RESULTS | TASK_DATA_ACCESS | WORKSPACE_VARIABLES | WORKSPACE_STATE |
|------|-----------------|------------------|--------------------|-----------------|
| `runtask-checker`（检查类） | WRITE | READ | - | - |
| `runtask-approver`（审批类） | WRITE | READ | READ | READ |

管理员为审批类 RunTask 的 Application 绑定 `runtask-approver` 角色即可获得读取前序结果和 workspace 数据的权限。

### 4.4 端点权限验证

所有 `/api/v1/run-task-results/` 下的端点统一通过 `validateRunTaskToken` 中间件验证：

```
validateRunTaskToken(c *gin.Context, resultID string):
    token = extractBearerToken(c)

    // 尝试 Application 签名验证
    claims, app = tryValidateWithAppSecret(token)
    if claims != nil:
        // Application token: 检查 scope claim 是否包含所需权限
        if !claims.Scope.Contains(requiredPermission):
            return 403
        return claims

    // 回退到全局 JWT_SECRET 验证（无 Application 关联的简单 token）
    claims = validateWithJWTSecret(token)
    if claims != nil:
        // 简单 token: 仅允许访问自身 result 相关端点
        if claims.ResultID != resultID:
            return 403
        return claims

    return 401
```

涉及函数：
- `RunTaskCallbackHandler.validateRunTaskToken` — 重构，支持双重验证
- `RunTaskTokenService.ValidateAccessToken` — 增加 Application secret 验证路径
- `RunTaskTokenService.tryValidateWithAppSecret(token) (*Claims, *Application)` — 新增

### 4.5 Token 不可变原则

- RunTask 平台**无法**自行请求更高权限的 token
- Token 的 `scope` 在签发时由管理员配置的 IAM Role 决定
- 修改 Application 的角色绑定后，**已签发的 token 不受影响**（token 是自包含的），新的 token 会携带更新后的 scope
- 即使 RunTask 平台被攻破，攻击者只能在 token scope 范围内操作

---

## 5. 新增 API 端点

### 5.1 GET /api/v1/run-task-results/:result_id/prior-results

返回同一 task、同一 stage 中，priority 小于当前 result 优先级的所有结果（含 outcomes）。

**认证**: Bearer token
**权限要求**:
- Application token: scope 包含 `RUN_TASK_RESULTS:READ`
- 简单 token: 允许（prior-results 是同 task 同 stage 的数据，不算越权）

**响应**:

```json
{
  "prior_results": [
    {
      "result_id": "rtr-xxx",
      "run_task_name": "Security Scan",
      "enforcement_level": "advisory",
      "status": "passed",
      "message": "All checks passed",
      "completed_at": "2026-02-20T10:30:00Z",
      "outcomes": [
        {
          "outcome_id": "SEC-001",
          "description": "No hardcoded secrets found",
          "tags": { "Severity": [{ "label": "Info", "level": "info" }] }
        }
      ]
    }
  ]
}
```

**查询逻辑**:
1. 从 token claims 获取 `task_id`
2. 从 `run_task_results` 查当前 result 的 `priority` 值 P
3. 查询同 `task_id`、同 `stage`、`priority < P` 的所有 result，Preload Outcomes 和 WorkspaceRunTask

涉及函数：
- `RunTaskCallbackHandler.GetPriorResults` — 新增 handler
- `router_run_task.go` — 新增路由

---

## 6. API 变更汇总

| 方法 | 端点 | 变更 |
|------|------|------|
| POST | `/api/v1/workspaces/:id/run-tasks` | 请求体新增 `priority` |
| PUT | `/api/v1/workspaces/:id/run-tasks/:wrt_id` | 请求体新增 `priority` |
| POST | `/api/v1/run-tasks` | 请求体新增 `global_priority`, `application_id` |
| PUT | `/api/v1/run-tasks/:id` | 请求体新增 `global_priority`, `application_id` |
| GET | `/api/v1/run-task-results/:result_id/prior-results` | **新增**，token 认证 |
| POST | (webhook payload) | 新增 `prior_results`, `prior_results_api_url` |

---

## 7. 前端变更

### 7.1 RunTask 配置页

- Workspace RunTask 绑定表单新增 `Priority` 数字输入（默认 0）
- 全局 RunTask 编辑表单新增 `Global Priority` 数字输入（默认 0）
- 全局 RunTask 编辑表单新增 `Application` 下拉选择（可选，从 IAM Applications 列表获取）
- RunTask 列表按 priority 排序展示

### 7.2 Task Detail 页 RunTask 面板

- 按 stage 分组（不变），stage 内部按 priority 分组
- 每个 priority 批次标注 "Batch 0" / "Batch 1" 等
- 不同批次之间展示分隔线和"等待前序完成"状态指示
- 审批类 RunTask 结果区域展示其引用的前序结果摘要

---

## 8. 向后兼容性

| 场景 | 影响 |
|------|------|
| 现有 WorkspaceRunTask 无 priority 字段 | 默认 `0`，全部并行，行为不变 |
| 现有 RunTask 无 application_id | 使用现有简单 JWT，行为不变 |
| 现有 webhook payload 无 prior_results | 第三方平台忽略未知字段即可 |
| 现有 token 无 app_id/scope | `validateRunTaskToken` 回退到 JWT_SECRET 验证路径，行为不变 |

---

## 9. 实现顺序

1. DB Migration：新增 priority、application_id 字段
2. Model 更新：`WorkspaceRunTask.Priority`, `RunTask.GlobalPriority`, `RunTask.ApplicationID`, `RunTaskResult.Priority`, `RunTaskTokenClaims` 扩展
3. API 更新：Create/Update 请求支持 priority 和 application_id
4. `resolveApplicationScope` — 新增，查询 Application 的 IAM 权限
5. `RunTaskTokenService` — 重构 `GenerateAccessToken` 和 `ValidateAccessToken`，支持 Application 签名和 scope claims
6. `ExecuteRunTasksForStage` — 重构为 `groupByPriority` + `executeBatch` 分批循环
7. `buildWebhookPayload` — 支持注入 `prior_results`
8. 新增 `GET /run-task-results/:result_id/prior-results` 端点
9. `validateRunTaskToken` — 重构，支持双重验证和 scope 检查
10. 新增 IAM 预置角色 `runtask-checker` 和 `runtask-approver`
11. 新增 `ResourceType: RUN_TASK_RESULTS`
12. 前端配置页和展示页适配
