# Observability Phase 3: 业务指标补全 + K8s 探针切换

**Goal:** 补全 Workspace 任务、Agent 连接/分发、认证、AI Token、Drift 检测等业务指标，并将 K8s 探针从 `/health` 安全切换到三级端点。

**架构:** 新建 `internal/observability/metrics/business.go` 统一注册业务指标。各业务模块（task_queue_manager、agent_cc_handler、auth handler、drift service）在关键事件点调用记录函数。指标注册到 Phase 1 已有的 `metricsRegistry`，通过 `/metrics` 端点暴露。

**约束:** 见 `docs/observability/README.md` 第 6 章。本期重点约束：
- [6.6] 所有标签值必须为预定义枚举，不接受用户输入
- [6.1] 业务指标记录失败不得影响业务逻辑（panic-safe）
- [6.2] K8s 探针分步切换，先验证新端点稳定性

**前置:** Phase 1 + Phase 2 已完成（`feat/observability-phase1` 分支）。

---

## Task 1: 业务指标注册框架

**建:** `backend/internal/observability/metrics/business.go`, `business_test.go`

- 实现 `RegisterBusinessMetrics(reg *prometheus.Registry)` — 注册下列所有业务指标
- 实现各指标的记录函数（`RecordTaskStarted`、`RecordTaskCompleted`、`IncAgentConnected` 等）
- 所有记录函数内置 nil guard + `defer recover()`，指标未注册或 panic 时静默跳过
- 在 `router.go` 的 `metrics.RegisterDBMetrics(metricsReg)` 之后调用 `metrics.RegisterBusinessMetrics(metricsReg)`

### 1.1 Workspace 任务指标

| 指标名称 | 类型 | 标签 | 记录点 |
|----------|------|------|--------|
| `iac_workspace_tasks_total` | Counter | `type`, `status` | 任务完成时 |
| `iac_workspace_task_duration_seconds` | Histogram | `type` | 任务完成时 (CompletedAt - StartedAt) |
| `iac_workspace_drift_detected_total` | Counter | `has_drift` | drift check 结果处理时 |

- `type` 枚举: `plan`, `apply`, `plan_and_apply`, `drift_check`
- `status` 枚举: `success`, `failed`, `cancelled`, `applied`, `planned_and_finished`, `apply_pending`
- `has_drift` 枚举: `true`, `false`

### 1.2 Agent 指标

| 指标名称 | 类型 | 标签 | 记录点 |
|----------|------|------|--------|
| `iac_agent_connections` | Gauge | — | 连接/断开时 Inc/Dec |
| `iac_agent_tasks_dispatched_total` | Counter | `pool_type` | SendTaskToAgent 时 |
| `iac_agent_tasks_completed_total` | Counter | `pool_type`, `status` | 任务完成回调时 |

- `pool_type` 枚举: `local`, `agent`, `k8s`
- `status` 枚举: `success`, `failed`

### 1.3 认证指标

| 指标名称 | 类型 | 标签 | 记录点 |
|----------|------|------|--------|
| `iac_auth_logins_total` | Counter | `method`, `status` | Login/SSO Callback 完成时 |
| `iac_auth_tokens_issued_total` | Counter | `type` | JWT 签发时 |

- `method` 枚举: `local`, `sso`
- `status` 枚举: `success`, `failure`
- `type` 枚举: `access`, `mfa`

### 1.4 AI Token 指标

| 指标名称 | 类型 | 标签 | 记录点 |
|----------|------|------|--------|
| `iac_ai_tokens_total` | Counter | `provider`, `type` | AI API 响应处理时 |

- `provider` 枚举: `openai`, `anthropic`, `azure` (按实际支持)
- `type` 枚举: `prompt`, `completion`
- **注意:** 需确认 AI API 响应中是否携带 token 用量数据。如当前不可用，该指标先注册但标注为 Phase 3+ 埋点

验证: `go build ./...` 通过; 注册不 panic; nil registry 不 panic

---

## Task 2: Workspace 任务指标埋点

**改:** `backend/services/task_queue_manager.go`

- 在 `executeTask()` 函数任务完成时（success/failed 分支）调用 `metrics.RecordTaskCompleted(taskType, status, duration)`
- 在 `pushTaskToAgent()` 中 agent 返回结果时调用同一记录函数
- 在 `ProcessDriftCheckResult()` 中调用 `metrics.RecordDriftDetected(hasDrift)`
- `import "iac-platform/internal/observability/metrics"` 加入 task_queue_manager.go
- 只在现有分支（success/failed/cancelled）中插入一行调用，不改变控制流

验证: `go build ./...` 通过; 现有任务流程不受影响

---

## Task 3: Agent 连接/分发指标埋点

**改:** `backend/internal/handlers/agent_cc_handler.go`

- 在 `HandleCCConnection()` agent 认证成功后调用 `metrics.IncAgentConnected()`
- 在 `handleConnection()` defer cleanup 中调用 `metrics.DecAgentConnected()`
- 在 `SendTaskToAgent()` 成功发送后调用 `metrics.IncAgentTaskDispatched(poolType)`
- 在 `handleTaskCompleted()` / `handleTaskFailed()` 中调用 `metrics.IncAgentTaskCompleted(poolType, status)`
- `poolType` 从 workspace 的 `ExecutionMode` 字段获取

验证: `go build ./...` 通过; Agent WebSocket 连接不受影响

---

## Task 4: 认证指标埋点

**改:** `backend/internal/handlers/auth.go`, `backend/internal/handlers/sso_handler.go`

- `auth.go` Login 函数:
  - 登录成功时调用 `metrics.IncLoginTotal("local", "success")` + `metrics.IncTokenIssued("access")`
  - 登录失败时调用 `metrics.IncLoginTotal("local", "failure")`
  - MFA 验证成功签发 MFA token 时调用 `metrics.IncTokenIssued("mfa")`
- `sso_handler.go` Callback 函数:
  - SSO 回调成功时调用 `metrics.IncLoginTotal("sso", "success")` + `metrics.IncTokenIssued("access")`
  - SSO 回调失败时调用 `metrics.IncLoginTotal("sso", "failure")`

验证: `go build ./...` 通过; 登录流程不受影响

---

## Task 5: 测试

**建:** `backend/internal/observability/metrics/business_test.go`

- 注册测试: `RegisterBusinessMetrics` 正常注册所有指标
- nil 安全测试: registry 为 nil 时注册不 panic
- 记录函数测试: 各 `Record*`/`Inc*`/`Dec*` 函数调用后 registry 中指标值正确变化
- panic 安全测试: 指标未注册时记录函数不 panic
- 标签枚举测试: 验证标签值在预期范围内

验证: `go test ./internal/observability/metrics/... -v -count=1` 全部通过

---

## Task 6: K8s 探针切换准备

**改:** `manifests/base/deployment-backend.yaml`

当前 K8s 探针配置:
```yaml
readinessProbe:
  httpGet:
    path: /health          # → 改为 /health/ready
    port: https
    scheme: HTTPS
livenessProbe:
  httpGet:
    path: /health          # → 改为 /health/live
    port: https
    scheme: HTTPS
```

切换步骤 [约束 6.2]:
1. 新增 `startupProbe` 指向 `/health/startup`（`failureThreshold: 30`, `periodSeconds: 5`）
2. `readinessProbe` 从 `/health` 改为 `/health/ready`
3. `livenessProbe` 从 `/health` 改为 `/health/live`
4. 保留原 `GET /health` 端点不变（向后兼容）

验证: `kubectl apply --dry-run=client -f manifests/base/deployment-backend.yaml` 无 YAML 错误

---

## Task 7: 集成验证

**无新文件** — 纯验证。

- `go build ./...` 通过
- `go test ./... -count=1` 全部通过（已知 controllers 包的 nil DB panic 除外）
- `curl /metrics` 包含 `iac_workspace_tasks_total`、`iac_agent_connections`、`iac_auth_logins_total` 指标定义
- K8s manifest YAML 合法

---

## 不在本期范围

| 项目 | 原因 | 排期 |
|------|------|------|
| AI Service Context 改造 | 111 个调用点，需逐个迁移 [约束 6.5] | 独立任务 |
| 异步 goroutine Span 传播 | 42 处 `context.Background()` | 独立任务 |
| AI Token 用量实际埋点 | 需确认 API 响应结构是否携带 token 数 | 视确认结果 |
| Grafana 仪表盘 + 告警规则 | 可视化侧工作 | 独立 |
| 观测栈部署编排 | 基础设施侧工作 | 独立 |

---

## 预期 Commit 历史

```
feat(metrics): add business metrics registration framework (workspace/agent/auth/ai)
feat(metrics): instrument workspace task lifecycle metrics
feat(metrics): instrument agent connection and dispatch metrics
feat(metrics): instrument authentication login and token metrics
test(metrics): add business metrics tests
chore(k8s): switch probes to three-tier health endpoints
```
