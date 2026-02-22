# IaC Platform 可观测性增强方案

## 文档版本

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| 1.0 | 2026-02-22 | Platform Team | 初始版本 |
| 1.1 | 2026-02-22 | Platform Team | 精简方案：聚焦指标、追踪、健康检查 |

---

## 目录

1. [现状分析](#1-现状分析)
2. [整体架构](#2-整体架构)
3. [指标监控方案](#3-指标监控方案)
4. [分布式追踪方案](#4-分布式追踪方案)
5. [健康检查方案](#5-健康检查方案)
6. [实施约束（安全红线）](#6-实施约束安全红线)
7. [实施计划](#7-实施计划)

> **本期不含（后续独立推进）**: 结构化日志 (slog)、前端监控、Grafana 仪表盘 / 告警规则、观测栈部署编排、审计日志增强

---

## 1. 现状分析

### 1.1 当前可观测能力评估

| 维度 | 当前状态 | 成熟度 | 主要问题 |
|------|----------|--------|----------|
| **日志** | `log.Printf()` + Gin Logger + `TerraformLogger` | ★★☆☆☆ | 无结构化、无关联 ID；后续可独立演进 |
| **指标** | 自实现 Prometheus 文本格式 (`ai_metrics.go`) | ★★☆☆☆ | 仅覆盖 AI 调用，缺 HTTP/DB/业务指标，非标准客户端 |
| **追踪** | 无 | ☆☆☆☆☆ | 无法追踪请求链路，问题定位靠猜 |
| **健康检查** | `/health` → `{"status":"ok"}` | ★☆☆☆☆ | 不检查依赖、不支持 K8s 三级探针 |
| **审计** | 4 张表 + `AuditLogger` 中间件 | ★★★☆☆ | 覆盖面已较广，缺 trace_id 关联（后续增强） |
| **实时监控** | WebSocket Agent 指标推送 | ★★★☆☆ | 仅 Agent CPU/内存/任务，无全局视图 |

### 1.2 现有资产盘点

本方案在以下现有实现基础上增强，而非推倒重来：

| 资产 | 文件位置 | 说明 |
|------|----------|------|
| AI 指标收集 | `services/ai_metrics.go` | 自实现 Histogram/Counter/Gauge + Prometheus 文本格式导出，**需迁移到官方客户端** |
| 健康端点 | `internal/router/router.go:32-34` | `GET /health` → `{"status":"ok"}` |
| Agent 指标 Hub | `internal/websocket/agent_metrics_hub.go` | WebSocket 实时推送 Agent CPU/内存/任务状态 |
| Gin 中间件链 | `internal/middleware/` | CORS → Logger → ErrorHandler → JWTAuth → AuditLogger → RateLimit → IAMPermission |
| 审计系统 | `internal/middleware/audit_logger.go` | `access_logs` / `permission_audit_logs` / `agent_access_log` / `sso_login_log` 四张表 |

### 1.3 核心痛点（本期聚焦）

1. **故障排查困难**: 无 Trace ID 贯穿请求链路，跨组件问题定位依赖人工关联日志
2. **性能瓶颈不明**: 缺少 HTTP/DB 细粒度指标，无法定位慢查询/慢请求
3. **指标体系碎片化**: AI 指标自实现，与 Prometheus 生态不兼容，无法被标准工具抓取
4. **健康检查形同虚设**: 不检查 DB/外部依赖，K8s 无法据此做调度决策

---

## 2. 整体架构

### 2.1 技术选型

| 维度 | 技术 | 选型理由 |
|------|------|----------|
| 指标 | `prometheus/client_golang` | Go 生态事实标准，替换现有自实现 |
| 追踪 | OpenTelemetry SDK + OTLP 导出 | CNCF 标准，W3C Trace Context 兼容 |
| 追踪后端 | Grafana Tempo / Jaeger | 按需独立部署，本方案不涉及 |
| 健康检查 | 自实现 | 逻辑简单，不引入额外依赖 |

> **范围说明**: 本方案聚焦 **应用侧 instrumentation**。可视化（Grafana）、日志聚合（Loki）、部署编排等基础设施方案后续独立文档化。

### 2.2 数据流

```
用户请求
   │
   ▼
┌──────────────────────────────────────────────────────────────┐
│  Gin Middleware Chain                                         │
│  CORS → [Metrics] → [Tracing] → Logger → ErrorHandler       │
│       → JWTAuth → AuditLogger → RateLimit → IAMPermission   │
└──────────────────────────────────────────────────────────────┘
   │
   ├───────────────────┬──────────────────┬────────────────────┐
   ▼                   ▼                  ▼                    ▼
┌──────────┐    ┌──────────┐      ┌──────────┐        ┌──────────┐
│   指标   │    │   追踪   │      │   审计   │        │   健康   │
│(Prometheus│   │(OpenTelemetry)│  │  (现有)  │        │   检查   │
│ client)  │    │          │      │          │        │          │
└────┬─────┘    └────┬─────┘      └──────────┘        └──────────┘
     │               │
     ▼               ▼
  GET /metrics    OTLP Export
  (Prometheus     (Tempo/Jaeger
   Scrape)         接收)
```

### 2.3 新增目录结构

```
backend/internal/observability/
├── metrics/            # 指标监控
│   ├── registry.go     # 全局 Registry + 初始化
│   ├── http.go         # HTTP 中间件指标
│   ├── database.go     # GORM 回调指标
│   └── business.go     # 业务指标（含 AI 指标迁移）
├── tracing/            # 分布式追踪
│   ├── provider.go     # TracerProvider 初始化 + Shutdown
│   ├── middleware.go   # Gin 追踪中间件
│   └── propagator.go   # Context 传播工具函数
└── health/             # 健康检查
    ├── handler.go      # HTTP Handler（live/ready/startup）
    └── checkers.go     # 各依赖检查器实现
```

---

## 3. 指标监控方案

### 3.1 现有 AI 指标迁移

当前 `ai_metrics.go` 自实现了 Histogram/Counter/Gauge 和 Prometheus 文本格式导出。迁移策略：

| 步骤 | 说明 |
|------|------|
| 1. 引入 `prometheus/client_golang` | 替换自实现的 Registry、Histogram、Counter |
| 2. 重建现有指标 | 保持指标名称不变（`iac_ai_call_duration_ms` 等），切换到官方类型 |
| 3. 替换 `/metrics` 端点 | 使用 `promhttp.Handler()` 替换自实现的文本格式导出 |
| 4. 删除自实现代码 | 移除 `ai_metrics.go` 中的 Histogram/Counter/Gauge/Registry 自实现 |

> **兼容性**: 指标名称和标签保持不变，现有 Prometheus 抓取配置无需修改。

### 3.2 指标命名规范

```
iac_<domain>_<metric>_<unit>

示例:
- iac_http_requests_total          (Counter)
- iac_http_request_duration_seconds (Histogram)
- iac_db_query_duration_seconds     (Histogram)
- iac_agent_connections             (Gauge)
```

> **注意**: 现有 AI 指标使用 `_ms` 后缀（如 `iac_ai_call_duration_ms`），为保持兼容不做重命名。新增指标统一使用 `_seconds`。

### 3.3 HTTP 指标

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_http_requests_total` | Counter | method, route, status_code | 请求总数 |
| `iac_http_request_duration_seconds` | Histogram | method, route | 请求延迟分布 |
| `iac_http_requests_in_flight` | Gauge | — | 正在处理的请求数 |
| `iac_http_request_size_bytes` | Histogram | method, route | 请求体大小 |
| `iac_http_response_size_bytes` | Histogram | method, route | 响应体大小 |

**采集方式**: 新增 Gin 中间件，插入到中间件链 CORS 之后。

**高基数防护**: `route` 标签使用 Gin 路由模板（如 `/api/v1/workspaces/:id`）而非实际路径值。

### 3.4 数据库指标

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_db_queries_total` | Counter | operation, table | 查询总数 |
| `iac_db_query_duration_seconds` | Histogram | operation, table | 查询延迟 |
| `iac_db_connections_open` | Gauge | — | 当前连接数 |
| `iac_db_connections_max` | Gauge | — | 最大连接数 |
| `iac_db_connections_waiting` | Gauge | — | 等待连接的请求数 |

**采集方式**: GORM Callback（Before/After Create/Query/Update/Delete）+ `sql.DBStats` 定时采集（每 15s）。

### 3.5 业务指标

#### 3.5.1 Workspace 相关

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_workspace_tasks_total` | Counter | type, status | 任务执行总数 |
| `iac_workspace_task_duration_seconds` | Histogram | type | 任务执行延迟 |
| `iac_workspace_drift_detected_total` | Counter | — | 漂移检测次数 |

#### 3.5.2 Agent 相关

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_agent_connections` | Gauge | pool_id, status | Agent 连接数 |
| `iac_agent_tasks_dispatched_total` | Counter | pool_id | 任务分发总数 |
| `iac_agent_tasks_completed_total` | Counter | pool_id, status | 任务完成总数 |

#### 3.5.3 AI 相关（从 `ai_metrics.go` 迁移 + 新增）

| 指标名称 | 类型 | 标签 | 说明 | 状态 |
|----------|------|------|------|------|
| `iac_ai_call_duration_ms` | Histogram | capability, stage | AI 调用延迟 | 现有 → 迁移 |
| `iac_ai_call_total` | Counter | capability, stage | AI 调用总数 | 现有 → 迁移 |
| `iac_vector_search_duration_ms` | Histogram | — | 向量搜索延迟 | 现有 → 迁移 |
| `iac_vector_search_total` | Counter | — | 搜索结果总数 | 现有 → 迁移 |
| `iac_active_parallel_tasks` | Gauge | — | 活跃并行任务数 | 现有 → 迁移 |
| `iac_ai_tokens_total` | Counter | provider, type | Token 消耗总数 | **新增** |

#### 3.5.4 认证相关

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_auth_logins_total` | Counter | status, method | 登录尝试总数 |
| `iac_auth_tokens_issued_total` | Counter | type | Token 发放总数 |

### 3.6 系统指标

由 `prometheus/client_golang` 自动注册，无需额外代码：

- `go_goroutines` — Goroutine 数量
- `go_gc_duration_seconds` — GC 延迟
- `go_memstats_alloc_bytes` — 内存分配
- `process_cpu_seconds_total` — CPU 使用

### 3.7 指标采集架构

```
┌─────────────────────────────────────────────────────────────┐
│                     IaC Backend                              │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │   HTTP       │  │   Database   │  │   Business   │       │
│  │   Metrics    │  │   Metrics    │  │   Metrics    │       │
│  │  Middleware  │  │ GORM Callback│  │   (手动埋点)  │       │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       │
│         │                 │                 │                │
│         └─────────────────┴─────────────────┘                │
│                           │                                  │
│                  ┌────────▼────────┐                         │
│                  │  prometheus     │                         │
│                  │  DefaultRegistry│                         │
│                  └────────┬────────┘                         │
│                           │                                  │
│                  ┌────────▼────────┐                         │
│                  │ promhttp.Handler│                         │
│                  │  GET /metrics   │                         │
│                  └─────────────────┘                         │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. 分布式追踪方案

### 4.1 追踪模型

基于 OpenTelemetry SDK，遵循 W3C Trace Context 规范。

```
Trace (完整请求链路)
├── Span 1: HTTP Request (Root Span)
│   ├── Span 1.1: JWT Authentication
│   ├── Span 1.2: IAM Permission Check
│   ├── Span 1.3: Business Logic
│   │   ├── Span 1.3.1: Database Query
│   │   ├── Span 1.3.2: AI Service Call
│   │   └── Span 1.3.3: Terraform Execution
│   └── Span 1.4: Response Serialization
```

### 4.2 TracerProvider 初始化

```go
// internal/observability/tracing/provider.go
//
// 初始化:
// - Exporter: OTLP gRPC (目标 Tempo/Jaeger)
// - Sampler: TraceIDRatioBased (采样率由 OTEL_TRACES_SAMPLER_ARG 控制)
// - Resource: service.name, service.version, deployment.environment
// - BatchSpanProcessor: 批量导出，减少网络开销
//
// Shutdown: main.go 中 defer tp.Shutdown(ctx) 确保 flush
```

### 4.3 关键追踪点

| 组件 | Span 名称 | 关键属性 | 实现方式 |
|------|----------|----------|----------|
| **HTTP** | `{method} {route}` | method, route, status_code, user_id | Gin 中间件 (`otelgin`) |
| **Database** | `db.{operation}` | operation, table, rows_affected | GORM Callback |
| **AI Service** | `ai.{provider}.{operation}` | provider, model, tokens | AI service 调用处手动创建 |
| **Terraform** | `terraform.{operation}` | operation, workspace_id | Task executor 中手动创建 |
| **Agent Dispatch** | `agent.dispatch` | agent_id, pool_id, task_type | Dispatcher 中手动创建 |

### 4.4 Context 传播

```
外部请求
   │  Header: traceparent: 00-{trace-id}-{parent-id}-{flags}
   ▼
┌──────────────────────────────────────────────────┐
│ Tracing Middleware                                │
│ 1. 从 Header 提取 Trace Context（或创建新 Trace） │
│ 2. 创建 Root Span                                │
│ 3. 注入到 gin.Context / context.Context          │
└──────────────────────────────────────────────────┘
   │
   ├──→ GORM Callback: 从 ctx 继承 Span，创建子 Span
   ├──→ HTTP Client:   注入 traceparent 到出站 Header
   ├──→ Async Task:    通过 context 传递，新 goroutine 中延续
   └──→ Audit Logger:  从 ctx 提取 trace_id 写入审计记录 (后续增强)
```

### 4.5 Span 数据结构示例

```json
{
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "parent_span_id": "00f067aa0ba902b6",
  "name": "POST /api/v1/workspaces/:workspace_id/tasks",
  "kind": "SERVER",
  "start_time": "2026-02-22T10:00:00.000000Z",
  "end_time": "2026-02-22T10:00:01.500000Z",
  "status": "OK",
  "attributes": {
    "http.method": "POST",
    "http.route": "/api/v1/workspaces/:workspace_id/tasks",
    "http.status_code": 200,
    "user.id": "user-123",
    "workspace.id": "ws-456"
  },
  "events": [
    {
      "name": "task_queued",
      "timestamp": "2026-02-22T10:00:00.100000Z",
      "attributes": {"task_id": 1001}
    }
  ]
}
```

### 4.6 导出配置

应用侧通过标准 OpenTelemetry 环境变量配置，无需应用内硬编码：

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://tempo:4317
OTEL_SERVICE_NAME=iac-backend
OTEL_TRACES_SAMPLER=traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1  # 生产 10% 采样，开发环境设为 1.0
```

---

## 5. 健康检查方案

### 5.1 端点设计

替换现有 `GET /health`，新增三级端点：

| 端点 | 用途 | 检查内容 | K8s Probe |
|------|------|----------|-----------|
| `/health/live` | 进程存活 | HTTP 服务器是否响应 | livenessProbe |
| `/health/ready` | 服务就绪 | DB + 关键依赖可用性 | readinessProbe |
| `/health/startup` | 启动完成 | 迁移、调度器、WebSocket 初始化 | startupProbe |

> 保留 `GET /health` 作为 `/health/live` 的别名，向后兼容。

### 5.2 检查项详细设计

#### 5.2.1 Liveness (`/health/live`)

```json
{
  "status": "healthy",
  "timestamp": "2026-02-22T10:00:00Z"
}
```

仅验证 HTTP 服务器响应能力，**不检查外部依赖**。200 = 健康，503 = 不健康。

#### 5.2.2 Readiness (`/health/ready`)

```json
{
  "status": "healthy|unhealthy",
  "timestamp": "2026-02-22T10:00:00Z",
  "checks": {
    "database": {
      "status": "pass|fail",
      "latency_ms": 5
    },
    "vector_database": {
      "status": "pass|fail",
      "latency_ms": 10
    },
    "ai_service": {
      "status": "pass|skip",
      "message": "未配置则 skip"
    }
  }
}
```

任一 `fail` → 整体 503。`skip` 表示该依赖未启用，不影响结果。

#### 5.2.3 Startup (`/health/startup`)

```json
{
  "status": "ready|starting",
  "timestamp": "2026-02-22T10:00:00Z",
  "checks": {
    "database_migration": {"status": "pass"},
    "leader_election": {"status": "pass", "is_leader": true},
    "schedulers": {"status": "pass", "count": 9},
    "websocket_server": {"status": "pass", "port": 8090}
  }
}
```

### 5.3 K8s Probe 配置

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
  failureThreshold: 3

startupProbe:
  httpGet:
    path: /health/startup
    port: 8080
  periodSeconds: 5
  failureThreshold: 30  # 最多 150 秒启动时间
```

---

## 6. 实施约束（安全红线）

本章列出经代码分析确认的实施约束。违反任一条可能导致平台功能异常或服务中断。

### 6.1 中间件必须 panic-safe

**风险**: 现有中间件链无 panic recovery（`ErrorHandler` 只处理 `c.Errors`，不 recover panic）。新增 Metrics / Tracing 中间件若 panic，整个请求崩溃 → K8s livenessProbe 失败 → Pod 重启风暴。

**约束**:
- 所有新增中间件**必须**内置 `defer func() { if r := recover(); r != nil { ... } }()`
- 观测代码 panic 时静默降级（跳过采集），**不得中断业务请求**
- 在中间件链首位添加 `gin.Recovery()` 作为兜底

```go
// 正确：中间件内部 recover
func MetricsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if r := recover(); r != nil {
                // 记录 panic 但不中断请求
                log.Printf("[metrics] panic recovered: %v", r)
            }
        }()
        // ... 指标采集逻辑
        c.Next()
        // ... 记录响应指标
    }
}
```

### 6.2 `/health` 端点行为不变

**风险**: K8s 部署配置 `manifests/base/deployment-backend.yaml:65-78` 的 liveness 和 readiness 探针**都指向 `/health`**，每 10-30s 探测一次。如果该端点加了 DB 检查，DB 短暂抖动 → 503 → Pod 被杀 → 级联故障。

**约束**:
- `GET /health` **保持原样**：返回 `{"status":"ok"}`，HTTP 200，不加任何依赖检查
- 新端点 `/health/live`、`/health/ready`、`/health/startup` 独立注册，不影响原端点
- K8s manifest **分步迁移**：先部署新端点代码并验证稳定，再修改 K8s probe 指向新端点

### 6.3 AI 指标函数签名不变

**风险**: `ai_metrics.go` 导出 12 个函数，在 `ai_cmdb_skill_service.go` 和 `ai_cmdb_skill_service_sse.go` 中有 **111 个调用点**。函数签名变更 = 编译失败或语义错误。

**约束**:
- 迁移到 `prometheus/client_golang` 时，保持所有导出函数签名**完全不变**
- `NewTimer()` / `ElapsedMs()` 接口保持兼容
- Histogram bucket 边界与现有实现保持一致，避免历史数据对比失真
- 迁移完成后编写对比测试：同一组输入，新旧 `/metrics` 输出格式一致

```go
// 迁移策略：只换内部实现，外部接口不动
// Before (自实现)
func RecordAICallDuration(capability, stage string, durationMs float64) {
    GetAIMetrics().histograms["iac_ai_call_duration_ms"].Observe(durationMs, capability, stage)
}

// After (prometheus/client_golang)
func RecordAICallDuration(capability, stage string, durationMs float64) {
    aiCallDurationHist.WithLabelValues(capability, stage).Observe(durationMs)
}
```

### 6.4 GORM Callback 不得干扰事务

**风险**: `task_lock_service.go:44` 使用 `SKIP LOCKED` 实现分布式任务锁。如果 Callback 在事务中执行额外查询或 panic，任务锁获取失败 → 任务队列停滞 → Workspace Plan/Apply 无法执行。

**约束**:
- Callback 内部**必须** `defer recover()`，panic 时丢弃观测数据但不中断查询
- Callback **只读取** `gorm.Statement` 元信息（表名、操作类型、耗时），**不执行任何 SQL**
- 对 `task_lock_service` 编写针对性测试，确认加 Callback 后 `SKIP LOCKED` 行为不变
- 连接池指标通过 `sql.DBStats` 定时采集，不走 Callback

### 6.5 Context 传播渐进式推进

**风险**: 追踪依赖 `context.Context` 传播 Span，但代码中存在三种不兼容模式：
- Handler 层：16+ 文件使用 `c.Request.Context()` ✓
- AI Service 层：`ai_cmdb_skill_service.go` 等**不接受 Context** ✗（111 个调用点无法关联 Trace）
- 异步层：`agent_handler.go:1339` 明确使用 `context.Background()` ✗（Span 孤立）
- DB 事务：3 处使用 `db.Begin()` 而非 `db.WithContext(ctx)` ✗

**约束**:
- **不要**一次性重构所有 Service 添加 Context 参数，避免大面积 regression
- Phase 2 先覆盖已有 Context 的路径（HTTP 层 + GORM Callback via `db.WithContext`）
- AI Service 的 Context 改造作为独立任务排期，需逐个 Service 迁移并测试
- 异步 goroutine 使用 `trace.SpanFromContext(ctx).SpanContext()` 创建 Link 而非强制传 Context

### 6.6 指标标签基数控制

**风险**: 每个唯一标签组合在 `prometheus/client_golang` 中创建独立时间序列。AI 指标的 `capability × stage` 如果接受动态值，时间序列持续增长 → 内存 OOM。

**约束**:
- 所有标签值**必须**为预定义枚举，不接受用户输入或动态生成的值
- HTTP 指标的 `route` 标签使用 Gin 路由模板（`/api/v1/workspaces/:id`），不使用实际路径
- 上线后监控 `prometheus_client_go_metric_desc_total` 指标，时间序列数超过 10000 即告警排查

---

## 7. 实施计划

### 7.1 阶段划分

每个 Phase 的实施必须遵守第 6 章全部约束。

```
Phase 1: 指标体系 + 健康检查
├── 中间件链首位添加 gin.Recovery()                          [约束 6.1]
├── 引入 prometheus/client_golang
├── ai_metrics.go 迁移到官方客户端（签名不变）                [约束 6.3]
├── HTTP 指标中间件（内置 recover，route 标签防基数爆炸）     [约束 6.1, 6.6]
├── DB 指标 GORM Callback（只读 Statement，不执行 SQL）      [约束 6.4]
├── 健康检查三级端点（保留原 /health 不变）                   [约束 6.2]
├── /metrics 端点切换到 promhttp.Handler()
├── task_lock_service SKIP LOCKED 回归测试                   [约束 6.4]
└── ai_metrics 新旧输出对比测试                              [约束 6.3]

Phase 2: 分布式追踪
├── 引入 OpenTelemetry SDK
├── Tracing 中间件（内置 recover + Root Span + Context 注入） [约束 6.1]
├── 已有 Context 路径的 DB Span（db.WithContext）
├── HTTP 层 + GORM 层追踪验证
└── 不改造 AI Service Context（后续独立排期）                  [约束 6.5]

Phase 3: 业务指标补全 + K8s 探针切换
├── Workspace 任务指标
├── Agent 连接/分发指标
├── 认证指标
├── AI Token 消耗指标（新增）
└── K8s manifest 探针切换到 /health/live + /health/ready      [约束 6.2]
```

### 7.2 里程碑与验收标准

| 里程碑 | 交付物 | 验收标准 |
|--------|--------|----------|
| M1: 指标 + 健康检查 | `/metrics` 标准 Prometheus 格式；三级健康端点可用 | `curl /metrics` 包含 `iac_http_*`、`iac_db_*`、`iac_ai_*`；原 `/health` 行为不变；`/health/ready` DB 断开返回 503；task_lock 回归测试通过 |
| M2: 分布式追踪 | HTTP + DB 层链路可追踪 | 一次 API 请求生成 Trace 含 HTTP → DB 子 Span；AI Service 暂不含 Span（已知限制）；中间件 panic 不影响业务请求 |
| M3: 全指标覆盖 | 业务指标上线 + K8s 探针切换 | HTTP/DB/业务指标覆盖率 > 90%；K8s 探针切换后无 Pod 异常重启 |

### 7.3 后续规划（不在本期范围）

| 方向 | 说明 |
|------|------|
| 结构化日志 | `log.Printf` → `slog`，所有日志携带 trace_id / request_id |
| 审计日志增强 | `access_logs` 新增 `trace_id` / `event_type` 列，统一查询 API |
| AI Service Context 改造 | 逐个 Service 添加 `ctx` 参数，补全追踪覆盖 |
| 前端监控 | Error Boundary + Web Vitals + API 监控 |
| 可视化与告警 | Grafana 仪表盘 + Prometheus AlertManager |
| 部署编排 | `docker-compose.observability.yml`（Prometheus + Tempo + Loki + Grafana） |

---

## 附录

### A. 环境变量

```bash
# 指标
METRICS_ENABLED=true

# 追踪
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://tempo:4317
OTEL_SERVICE_NAME=iac-backend
OTEL_TRACES_SAMPLER_ARG=0.1  # 采样率 (0.0-1.0)

# 健康检查
HEALTH_CHECK_DB_TIMEOUT=5s
HEALTH_CHECK_AI_TIMEOUT=10s
```

### B. 依赖引入

```bash
go get github.com/prometheus/client_golang/prometheus
go get github.com/prometheus/client_golang/prometheus/promhttp
go get go.opentelemetry.io/otel
go get go.opentelemetry.io/otel/sdk
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
go get go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin
```

### C. 参考文档

- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)
- [Prometheus Go Client](https://github.com/prometheus/client_golang)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)

### D. 术语表

| 术语 | 说明 |
|------|------|
| Trace | 一次请求在系统中的完整链路 |
| Span | 追踪中的单个操作单元 |
| Metric | 可聚合的系统运行指标 |
| OTLP | OpenTelemetry Protocol |
| Instrumentation | 在代码中添加可观测性数据采集的过程 |
