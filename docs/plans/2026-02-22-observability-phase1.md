# Observability Phase 1: Metrics + Health Checks

**Goal:** 替换自实现 AI 指标为 `prometheus/client_golang`，新增 HTTP/DB 指标中间件，实现 K8s 三级健康检查 — 不破坏任何现有功能。

**架构:** 新建 `internal/observability/` 包，提供 metrics registry、Gin 中间件、GORM Callback、健康检查 handler。所有新中间件内置 panic recovery。现有 `ai_metrics.go` 12 个导出函数签名不变，仅替换内部实现。`/health` 端点保持原样。

**约束:** 见 `docs/observability/README.md` 第 6 章全部安全红线。

---

## Task 1: 添加 Prometheus 依赖

**改:** `backend/go.mod`, `backend/go.sum`

- `go get github.com/prometheus/client_golang@latest && go mod tidy`
- 验证: `go build ./...` 通过

---

## Task 2: Metrics Registry + gin.Recovery

**建:** `backend/internal/observability/metrics/registry.go`, `registry_test.go`
**改:** `backend/internal/router/router.go`

- 实现 `InitRegistry()` — 创建 `prometheus.NewRegistry()`，注册 GoCollector + ProcessCollector
- 测试: `InitRegistry()` 返回非 nil，`Gather()` 包含 `go_goroutines`
- `router.go` 中间件链首位添加 `gin.Recovery()` [约束 6.1]

中间件链变为:
```
gin.Recovery() → CORS → Logger → ErrorHandler → JWTAuth → ...
```

---

## Task 3: HTTP 指标中间件

**建:** `backend/internal/observability/metrics/http.go`, `http_test.go`
**改:** `backend/internal/router/router.go`

- 实现 `HTTPMetricsMiddleware(reg)` — 记录 `iac_http_requests_total` (Counter), `iac_http_request_duration_seconds` (Histogram), `iac_http_requests_in_flight` (Gauge)
- 内置 `defer recover()` [约束 6.1]
- `route` 标签使用 `c.FullPath()` 路由模板，防基数爆炸 [约束 6.6]
- 测试: 请求后 registry 包含 `iac_http_requests_total`；`/api/v1/workspaces/ws-123` 的 route 标签为 `/api/v1/workspaces/:id`；不 panic
- `router.go` 在 `gin.Recovery()` 之后、`CORS()` 之前注册

---

## Task 4: DB 指标 (GORM Callback)

**建:** `backend/internal/observability/metrics/database.go`, `database_test.go`
**改:** `backend/internal/database/database.go`

- 实现 `RegisterDBMetrics(reg)` — 注册 `iac_db_queries_total`, `iac_db_query_duration_seconds`, `iac_db_connections_open/max/waiting`
- 实现 `RegisterGORMCallbacks(db)` — Before/After Create/Query/Update/Delete，通过 `db.Set()/Get()` 记录耗时
- Callback 只读 `gorm.Statement` 元信息，不执行任何 SQL [约束 6.4]
- 所有 Callback 内置 `defer recover()` [约束 6.4]
- 实现 `StartDBStatsCollector(sqlDB, 15s)` — goroutine 定时采集 `sql.DBStats`
- 在 `database.go` 的 `gorm.Open()` 之后注册 Callback + 启动 StatsCollector
- 测试: 注册不 panic；`recordDBMetric()` 传 nil 不 panic

---

## Task 5: AI 指标迁移

**改:** `backend/services/ai_metrics.go`
**建:** `backend/services/ai_metrics_test.go`

- 重写 `ai_metrics.go` 内部：自实现 Histogram/Counter/Gauge/Registry → `prometheus/client_golang` 对应类型
- **所有 12 个导出函数签名不变** [约束 6.3]
- `defaultBuckets` 保持 `{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}`
- `Timer` / `ElapsedMs()` 保持兼容
- 新增 `GetAIMetricsRegistry()` 返回内部 registry，供 Task 6 合并使用
- 测试:
  - 函数签名编译测试 — 调用全部 12 个函数，能编译 = 通过
  - `/metrics` 输出包含 `iac_ai_call_duration_ms`, `iac_ai_call_total`
  - Bucket 边界: 输出包含 `le="10"`, `le="250"`, `le="60000"`
- 验证: `go build ./...` 通过（111 个调用点无需修改）

---

## Task 6: 合并 /metrics 端点

**改:** `backend/internal/router/router.go`

- 将 `/metrics` 从 `gin.WrapF(services.MetricsHandler())` 改为 `promhttp.HandlerFor(prometheus.Gatherers{metricsRegistry, services.GetAIMetricsRegistry()}, ...)`
- 验证: `curl /metrics` 同时包含 `iac_http_*`, `iac_db_*`, `iac_ai_*`, `go_*` 指标

---

## Task 7: 三级健康检查

**建:** `backend/internal/observability/health/handler.go`, `checkers.go`, `handler_test.go`
**改:** `backend/internal/router/router.go`, `backend/main.go`

- `handler.go`: 实现 `RegisterRoutes(r, db)` — 注册 `/health`、`/health/live`、`/health/ready`、`/health/startup`
- **`GET /health` 返回 `{"status":"ok"}` 不变** [约束 6.2]
- `/health/live` — 仅验证 HTTP 响应，200 固定返回
- `/health/ready` — 检查 DB Ping，fail → 503
- `/health/startup` — 检查 DB + `startupReady` 原子标记
- `checkers.go`: 实现 `CheckDatabase(db, timeout)` — 带 context timeout 的 Ping
- `main.go`: 所有服务初始化完成后调用 `health.MarkStartupReady()`
- `router.go`: 移除原 `/health` handler，改为 `health.RegisterRoutes(r, db)`
- 测试:
  - `/health/live` → 200, `{"status":"healthy"}`
  - `/health` → 200, `{"status":"ok"}` [约束 6.2 回归]
  - `/health/ready` nil DB → 503, `{"status":"unhealthy"}`
  - `/health/startup` nil DB → 503

---

## Task 8: SKIP LOCKED 回归测试

**建:** `backend/services/task_lock_service_test.go`

- 连接本地 PG（不可用则 `t.Skip`）
- 注册 GORM Callback 后调用 `AcquireTask()`
- 验证: 不因 Callback panic 或报错（允许 "no available tasks"）[约束 6.4]

---

## Task 9: 集成验证

**无新文件** — 纯验证。

- `go build ./...` 通过
- `go test ./... -count=1` 全部通过
- 手动验证（如有本地环境）:
  - `curl /metrics` — Prometheus 文本格式，包含 `iac_http_*`, `iac_db_*`, `iac_ai_*`, `go_*`
  - `curl /health` — `{"status":"ok"}`
  - `curl /health/live` — `{"status":"healthy"}`
  - `curl /health/ready` — 包含 database check 结果

---

## 预期 Commit 历史

```
deps: add prometheus/client_golang
feat(observability): add metrics registry + gin.Recovery panic guard
feat(observability): add HTTP metrics middleware with panic recovery
feat(observability): add DB metrics via GORM callbacks (read-only, panic-safe)
refactor(metrics): migrate AI metrics to prometheus/client_golang (signatures unchanged)
feat(observability): combine AI + system metrics on /metrics endpoint
feat(observability): add three-tier health checks, preserve original /health
test(observability): add SKIP LOCKED regression test for GORM callbacks
```
