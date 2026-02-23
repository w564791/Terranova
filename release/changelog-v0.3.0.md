## v0.3.0

可观测性增强专项：引入 Prometheus 标准指标体系、OpenTelemetry 分布式追踪、K8s 三级健康检查，覆盖 HTTP/DB/业务/AI 全链路监控。

### Features — 指标监控

- **Prometheus 标准指标体系** — 引入 `prometheus/client_golang` 替换自实现指标收集，`/metrics` 端点切换为 `promhttp.Handler()`，同时暴露系统指标（`go_goroutines`、`go_gc_duration_seconds` 等）和业务指标 (`registry.go`, `router.go`)
- **HTTP 请求指标中间件** — 新增 `iac_http_requests_total` (Counter)、`iac_http_request_duration_seconds` (Histogram)、`iac_http_requests_in_flight` (Gauge)；`route` 标签使用 `c.FullPath()` 路由模板防基数爆炸，内置 `defer recover()` (`http.go`)
- **DB 查询指标** — GORM Before/After Callback 记录 `iac_db_queries_total`、`iac_db_query_duration_seconds`；goroutine 每 15s 采集 `sql.DBStats` 连接池指标（open/max/waiting）；Callback 只读 Statement 元信息，不执行 SQL (`database.go`)
- **AI 指标迁移** — `ai_metrics.go` 内部从自实现 Histogram/Counter/Gauge 迁移到 `prometheus/client_golang`，全部 12 个导出函数签名不变，111 个调用点零修改 (`ai_metrics.go`)
- **业务指标框架** — 统一注册 Workspace 任务（`iac_workspace_tasks_total`、`iac_workspace_task_duration_seconds`）、Agent 连接/分发（`iac_agent_connections`、`iac_agent_tasks_dispatched_total`）、认证（`iac_auth_logins_total`、`iac_auth_tokens_issued_total`）、AI Token 消耗（`iac_ai_tokens_total`）、漂移检测（`iac_workspace_drift_detected_total`）等指标，所有记录函数内置 nil guard + panic recovery (`business.go`)

### Features — 业务指标埋点

- **Workspace 任务生命周期** — 任务完成（success/failed/cancelled）时记录类型、状态、耗时 (`task_queue_manager.go`)
- **Agent 连接与分发** — WebSocket 连接/断开时增减 `iac_agent_connections`，任务分发/完成时按 `pool_type` 记录；pool_type 从数据库查询而非硬编码 (`agent_cc_handler.go`)
- **认证登录与 Token** — 本地登录/SSO 回调的成功/失败计数，JWT/MFA Token 签发计数 (`auth.go`, `sso_handler.go`)
- **AI Token 消耗** — Bedrock (Claude) 和 OpenAI 兼容 API 响应中解析 `usage` 字段，记录 prompt/completion token 数；覆盖错误分析、表单生成、模块 Skill 生成、OpenAI/Titan Embedding 调用 (`ai_analysis_service.go`, `ai_form_service.go`, `module_skill_ai_service.go`, `embedding_service.go`)
- **漂移检测** — drift check 结果处理时记录是否检出漂移 (`drift_check_service.go`)

### Features — 分布式追踪

- **OpenTelemetry TracerProvider** — OTLP gRPC 导出 + `TraceIDRatioBased` 采样；`OTEL_EXPORTER_OTLP_ENDPOINT` 未设置时自动降级为 noop（零开销），不阻塞服务启动 (`tracing/provider.go`, `main.go`)
- **HTTP 追踪中间件** — `otelgin.Middleware` 自动从请求 Header 提取 W3C `traceparent` 或创建新 Trace，创建 Root Span 并注入 `c.Request.Context()` (`router.go`)
- **DB 追踪 Callback** — GORM Before/After Callback（`trace:` 前缀）从父 Context 创建子 Span，记录 `db.system`、`db.operation` 属性；与 metrics Callback（`obs:` 前缀）独立注册互不冲突 (`tracing/gorm.go`, `database.go`)

### Features — 健康检查

- **K8s 三级健康端点** — `/health/live`（进程存活）、`/health/ready`（DB 可用性检查，fail → 503）、`/health/startup`（DB + 原子标记，服务初始化完成后置位）；原 `GET /health` 返回 `{"status":"ok"}` 行为不变 (`health/handler.go`, `health/checkers.go`)
- **K8s 探针切换** — `deployment-backend.yaml` 新增 `startupProbe`，`readinessProbe` → `/health/ready`，`livenessProbe` → `/health/live` (`deployment-backend.yaml`)

### Refactoring

- **中间件链重构** — 首位添加 `gin.Recovery()` 兜底 panic recovery；中间件顺序调整为 `Recovery → otelgin → HTTPMetrics → CORS → Logger → ErrorHandler → ...` (`router.go`)

### Bug Fixes

- **Agent 模式 apply 失败后 partial state 丢失** — `saveTaskFailure` 中 partial state save 被 `if s.db != nil` 跳过，Agent 模式下 `s.db` 为 nil 导致失败后的 terraform state 不保存；改为 Local/Agent 双通路：Local 从 DB 加载 workspace，Agent 构造最小 workspace 通过 `dataAccessor` 保存 (`terraform_executor.go`)
- **Agent 模式 apply 后缺失 CMDB 同步** — `handleTaskCompleted` 和 `handleTaskFailed` 只发通知，未触发 CMDB 同步和 Run Triggers；新增 `postApplyCompletionTasks`（成功：Run Triggers + CMDB 同步）和 `postApplyFailureCleanup`（失败：CMDB 同步），与 Local 模式 `TaskQueueManager.executeTask` 对齐 (`agent_cc_handler_raw.go`, `task_queue_manager.go`)
- **in-flight 指标泄漏修复** — HTTP 中间件使用 `defer` 确保 Gauge Dec 在 panic 时也能执行 (`http.go`)
- **Tracer shutdown context 修复** — 使用独立 context 进行 tracer shutdown，避免复用已取消的 shutdownCtx (`main.go`)
- **Agent pool_type 修复** — 连接时从数据库查询 `agent_pools` 表获取实际 pool_type，替代硬编码 "static" (`agent_cc_handler.go`)
- **Run Task 失败时 UI 错误显示位置修正** — Post-plan / Pre-apply Run Task（mandatory）失败时，`task.Stage` 停留在 `planning`，前端误将错误显示在 Plan/Apply 卡片中；后端设置 `task.Stage` 为 `post_plan_run_tasks` / `pre_apply_run_tasks`，前端据此判断：Plan 卡片显示 passed，Apply 卡片不显示 (`terraform_executor.go`, `TaskTimeline.tsx`)

### Tests

- **SKIP LOCKED 回归测试** — 注册 metrics + tracing Callback 后验证 `AcquireTask()` 任务锁行为不受影响 (`task_lock_service_test.go`)
- **业务指标测试** — 覆盖全部 9 个指标的注册、记录函数、nil 安全、panic 安全、标签枚举验证 (`business_test.go`)
- **可观测性单元测试** — Registry 初始化、HTTP 中间件、DB Callback、健康检查端点、TracerProvider、GORM tracing 全覆盖 (`registry_test.go`, `http_test.go`, `database_test.go`, `handler_test.go`, `provider_test.go`, `gorm_test.go`)

### Dependencies

- `github.com/prometheus/client_golang` v1.23.2
- `go.opentelemetry.io/otel` v1.40.0
- `go.opentelemetry.io/otel/sdk` v1.40.0
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` v1.40.0
- `go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin` v0.65.0

### Configuration

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 空 (禁用追踪) | Collector gRPC 地址 |
| `OTEL_SERVICE_NAME` | `iac-backend` | Trace 中的服务名 |
| `OTEL_TRACES_SAMPLER_ARG` | `1.0` | 采样率 (生产建议 `0.1`) |

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.2.9...v0.3.0
