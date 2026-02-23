# Observability Phase 2: 分布式追踪

**Goal:** 引入 OpenTelemetry SDK，实现 HTTP 请求自动生成 Trace + DB 查询自动创建子 Span — 不改造现有 Service 层函数签名。

**架构:** 新建 `internal/observability/tracing/` 包，提供 TracerProvider 初始化和 Shutdown。HTTP 层使用 `otelgin` 官方中间件自动注入 Root Span。DB 层复用现有 GORM Before/After Callback 结构，从 `tx.Statement.Context` 提取父 Span 创建子 Span。Collector 地址通过 `OTEL_EXPORTER_OTLP_ENDPOINT` 环境变量配置，未配置时追踪功能整体禁用（noop）。

**约束:** 见 `docs/observability/README.md` 第 6 章。本期重点约束：
- [6.1] 中间件 panic-safe
- [6.4] GORM Callback 只读、不干扰事务
- [6.5] 不改造 AI Service Context，不改造异步 goroutine

**前置:** Phase 1 已完成（`feat/observability-phase1` 分支）。

---

## Task 1: 添加 OpenTelemetry 依赖

**改:** `backend/go.mod`, `backend/go.sum`

- 引入 `go.opentelemetry.io/otel`、`go.opentelemetry.io/otel/sdk`、`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`、`go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin`
- 验证: `go build ./...` 通过

---

## Task 2: TracerProvider 初始化

**建:** `backend/internal/observability/tracing/provider.go`, `provider_test.go`

- 实现 `InitTracer(ctx) (shutdown func(context.Context) error, err error)`
- 读取 `OTEL_EXPORTER_OTLP_ENDPOINT` 环境变量，未设置时返回 noop（不创建 exporter，不报错）
- 已设置时创建 OTLP gRPC exporter + BatchSpanProcessor
- Resource 设置: `service.name` = `OTEL_SERVICE_NAME` (默认 `iac-backend`)，`service.version` 从构建信息或环境变量读取
- Sampler: `TraceIDRatioBased`，采样率从 `OTEL_TRACES_SAMPLER_ARG` 读取（默认 `1.0`）
- 调用 `otel.SetTracerProvider(tp)` 设置全局 provider
- 调用 `otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))` 设置 W3C propagator
- 返回的 `shutdown` 函数用于 `main.go` 中 `defer shutdown(ctx)` 优雅刷新
- 测试: `OTEL_EXPORTER_OTLP_ENDPOINT` 为空时 `InitTracer` 不报错，全局 provider 为 noop

---

## Task 3: main.go 集成 TracerProvider

**改:** `backend/main.go`

- 在 `database.Initialize()` 之后、`router.Setup()` 之前调用 `tracing.InitTracer(shutdownCtx)`
- `defer shutdown(ctx)` 确保进程退出时 flush 所有 Span
- 初始化失败只 log warning，不 Fatal（追踪不可用不应阻止服务启动）

---

## Task 4: HTTP Tracing 中间件

**改:** `backend/internal/router/router.go`

- 在中间件链中 `gin.Recovery()` 之后、`HTTPMetricsMiddleware` 之前插入 `otelgin.Middleware("iac-backend")`
- `otelgin` 自动: 从请求 Header 提取 `traceparent`（或创建新 Trace）→ 创建 Root Span → 注入 `c.Request.Context()`
- 中间件链变为:
  ```
  Recovery → otelgin → HTTPMetrics → CORS → Logger → ErrorHandler → ...
  ```
- `otelgin` 在 noop provider 下开销为零（不生成 Span、不分配内存）
- 测试: 发送请求后 `c.Request.Context()` 包含有效 SpanContext（通过 `trace.SpanFromContext` 验证）

---

## Task 5: DB Tracing (GORM Callback Span)

**建:** `backend/internal/observability/tracing/gorm.go`, `gorm_test.go`

- 实现 `RegisterGORMTracing(db)` — 注册 Before/After Callback（`trace:before_*` / `trace:after_*`）
- Before callback: 从 `tx.Statement.Context` 提取父 Span，创建子 Span（名称 `db.{operation}`），将新 ctx 写回 `tx.Statement.Context`
- After callback: 结束 Span，设置 `db.system=postgresql`、`db.operation` 属性；如有 error 记录 Span status
- 所有 Callback 内置 `defer recover()` [约束 6.1, 6.4]
- Callback 只读 Statement 元信息，不执行 SQL [约束 6.4]
- noop provider 下 `tracer.Start()` 返回 noop Span，无开销
- 在 `database.go` 的 `metrics.RegisterGORMCallbacks(db)` 之后调用 `tracing.RegisterGORMTracing(db)`
- 测试: 注册不 panic；nil db 不 panic；callback 名不与现有 `obs:*` 冲突

---

## Task 6: SKIP LOCKED 回归验证

**改:** `backend/services/task_lock_service_test.go`

- 在现有 `TestAcquireTask_WithGORMCallbacks` 中，同时注册 metrics Callback 和 tracing Callback
- 验证: SKIP LOCKED 查询不因 tracing Callback panic 或报错 [约束 6.4]

---

## Task 7: 集成验证

**无新文件** — 纯验证。

- `go build ./...` 通过
- `go test ./... -count=1` 全部通过
- `OTEL_EXPORTER_OTLP_ENDPOINT` 未设置时启动无报错，无 Span 输出（noop 模式）
- `OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317` 时启动无 panic（即使 collector 不存在，gRPC 连接异步建立不阻塞）

---

## 不在本期范围

| 项目 | 原因 | 排期 |
|------|------|------|
| AI Service 添加 Context 参数 | 111 个调用点，需逐个迁移测试 [约束 6.5] | Phase 3 或独立任务 |
| 异步 goroutine Span 传播 | 42 处 `context.Background()`，改动面大 [约束 6.5] | Phase 3 |
| `db.Begin()` → `db.WithContext(ctx).Begin()` | 7 处事务需逐一验证 | Phase 3 |
| 追踪后端部署 (Tempo/Jaeger) | 基础设施侧工作 | 独立 |
| Grafana Trace 面板配置 | 可视化侧工作 | 独立 |

---

## 环境变量

| 变量 | 必需 | 默认值 | 说明 |
|------|------|--------|------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 否 | 空 (禁用追踪) | Collector gRPC 地址，如 `tempo:4317` |
| `OTEL_SERVICE_NAME` | 否 | `iac-backend` | 服务名，写入 Trace Resource |
| `OTEL_TRACES_SAMPLER_ARG` | 否 | `1.0` | 采样率，生产建议 `0.1` |

---

## 预期 Commit 历史

```
deps: add opentelemetry SDK + otelgin
feat(tracing): add TracerProvider init with OTLP exporter (noop when unconfigured)
feat(tracing): integrate TracerProvider in main.go
feat(tracing): add otelgin HTTP tracing middleware
feat(tracing): add DB tracing via GORM callbacks (read-only, panic-safe)
test(tracing): extend SKIP LOCKED regression test with tracing callbacks
```
