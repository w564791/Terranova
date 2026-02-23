# IaC Platform 指标参考手册

> **端点:** `GET /metrics` (Prometheus 文本格式)
>
> **实现版本:** Phase 1 (`feat/observability-phase1`)

---

## 1. HTTP 指标

由 Gin 中间件自动采集，覆盖所有进入的 HTTP 请求。

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_http_requests_total` | Counter | `method`, `route`, `status` | HTTP 请求累计总数 |
| `iac_http_request_duration_seconds` | Histogram | `method`, `route`, `status` | HTTP 请求耗时（秒） |
| `iac_http_requests_in_flight` | Gauge | — | 当前正在处理的请求数 |

**标签说明:**

- `method` — HTTP 方法 (`GET`, `POST`, `PUT`, `DELETE` 等)
- `route` — Gin 路由模板 (如 `/api/v1/workspaces/:id`)，未匹配路由为 `unknown`
- `status` — HTTP 状态码 (`200`, `404`, `500` 等)

> `route` 使用 `c.FullPath()` 返回路由模板而非实际路径，防止高基数标签。
> 例如 `/api/v1/workspaces/ws-abc123` 记录为 `/api/v1/workspaces/:id`。

**Histogram Buckets:** `prometheus.DefBuckets` — `.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10` (秒)

**代码位置:** `backend/internal/observability/metrics/http.go`

---

## 2. 数据库指标

### 2.1 查询指标 (GORM Callback)

通过 GORM Before/After 回调自动采集，覆盖所有 Create/Query/Update/Delete 操作。

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_db_queries_total` | Counter | `operation` | 数据库查询累计总数 |
| `iac_db_query_duration_seconds` | Histogram | `operation` | 数据库查询耗时（秒） |

**标签说明:**

- `operation` — GORM 操作类型: `create`, `query`, `update`, `delete`

**Histogram Buckets:** `prometheus.DefBuckets`

### 2.2 连接池指标 (sql.DBStats)

后台 goroutine 每 15 秒采集一次 `sql.DBStats`。

| 指标名称 | 类型 | 标签 | 说明 |
|----------|------|------|------|
| `iac_db_connections_open` | Gauge | — | 当前打开的数据库连接数 |
| `iac_db_connections_max` | Gauge | — | 最大允许连接数 |
| `iac_db_connections_waiting` | Gauge | — | 等待连接的累计次数 |

**代码位置:** `backend/internal/observability/metrics/database.go`

---

## 3. AI 服务指标

### 3.1 耗时分布 (Histogram)

所有 Histogram 使用毫秒为单位，Buckets: `10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000`

| 指标名称 | 标签 | 说明 | 记录函数 |
|----------|------|------|----------|
| `iac_ai_call_duration_ms` | `capability`, `stage` | AI 调用耗时 | `RecordAICallDuration()` |
| `iac_vector_search_duration_ms` | `resource_type`, `stage` | 向量搜索耗时 | `RecordVectorSearchDuration()` |
| `iac_skill_assembly_duration_ms` | `capability`, `skill_count` | Skill 组装耗时 | `RecordSkillAssemblyDuration()` |
| `iac_parallel_execution_ms` | `task`, `status` | 并行执行耗时 | `RecordParallelExecutionDuration()` |
| `iac_domain_skill_selection_ms` | `skill_count`, `method` | Domain Skill 选择耗时 | `RecordDomainSkillSelection()` |
| `iac_cmdb_assessment_ms` | `need_cmdb`, `resource_type_count`, `method` | CMDB 评估耗时 | `RecordCMDBAssessment()` |

### 3.2 计数器 (Counter)

| 指标名称 | 标签 | 说明 | 记录函数 |
|----------|------|------|----------|
| `iac_ai_call_total` | `capability`, `status` | AI 调用次数 | `IncAICallCount()` |
| `iac_vector_search_total` | `resource_type`, `status` | 向量搜索次数 | `IncVectorSearchCount()` |
| `iac_cmdb_query_total` | `resource_type`, `status`, `candidate_count` | CMDB 查询次数 | `IncCMDBQueryCount()` |

**`status` 标签取值:**

- `IncAICallCount`: 由调用方传入 (通常 `success` / `error`)
- `IncVectorSearchCount`: `found` / `not_found` (由 `found bool` 参数自动派生)
- `IncCMDBQueryCount`: `found` / `not_found` / `multiple` (由 `found bool` + `candidateCount int` 自动派生)

### 3.3 仪表盘 (Gauge)

| 指标名称 | 标签 | 说明 | 记录函数 |
|----------|------|------|----------|
| `iac_active_parallel_tasks` | — | 当前活跃并行任务数 | `SetActiveParallelTasks()` |

**代码位置:** `backend/services/ai_metrics.go`

---

## 4. Go 运行时指标

由 `prometheus/client_golang` 的 `GoCollector` 和 `ProcessCollector` 自动注册。

| 指标名称 | 类型 | 说明 |
|----------|------|------|
| `go_goroutines` | Gauge | 当前 goroutine 数量 |
| `go_gc_duration_seconds` | Summary | GC 暂停耗时 |
| `go_memstats_alloc_bytes` | Gauge | 当前堆内存分配 |
| `go_memstats_sys_bytes` | Gauge | 从 OS 获取的总内存 |
| `go_threads` | Gauge | OS 线程数 |
| `process_cpu_seconds_total` | Counter | 进程 CPU 使用时间 |
| `process_open_fds` | Gauge | 打开的文件描述符数 |
| `process_resident_memory_bytes` | Gauge | 常驻内存大小 |

> 以上为常用子集，完整列表见 `curl /metrics | grep "^go_\|^process_"`。

---

## 5. 健康检查端点

非 Prometheus 指标，但属于可观测性端点，一并记录。

| 端点 | 用途 | 检查内容 | 成功响应 | 失败响应 |
|------|------|----------|----------|----------|
| `GET /health` | 向后兼容 | 无 (固定返回) | `200 {"status":"ok"}` | — |
| `GET /health/live` | K8s livenessProbe | HTTP 服务可响应 | `200 {"status":"healthy"}` | — |
| `GET /health/ready` | K8s readinessProbe | PostgreSQL Ping (2s 超时) | `200 {"status":"healthy","checks":{"database":"ok"}}` | `503 {"status":"unhealthy","checks":{"database":"..."}}` |
| `GET /health/startup` | K8s startupProbe | 启动标记 + DB Ping | `200 {"status":"healthy","checks":{"startup":"ready","database":"ok"}}` | `503 {"status":"unhealthy","checks":{...}}` |

**代码位置:** `backend/internal/observability/health/handler.go`, `checkers.go`

---

## 6. 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                     IaC Backend                             │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   HTTP       │  │   Database   │  │   AI Service │      │
│  │   Metrics    │  │   Metrics    │  │   Metrics    │      │
│  │  Middleware   │  │ GORM Callback│  │  (手动埋点)   │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         │                 │                 │               │
│  ┌──────▼─────────────────▼──┐  ┌───────────▼──────────┐   │
│  │  Infrastructure Registry  │  │   AI Metrics Registry │   │
│  │  (go_*, iac_http_*,       │  │   (iac_ai_*, iac_     │   │
│  │   iac_db_*, process_*)    │  │   vector_*, iac_cmdb_*│   │
│  └──────────┬────────────────┘  └───────────┬──────────┘   │
│             │                               │               │
│             └───────────┬───────────────────┘               │
│                         │                                   │
│              ┌──────────▼──────────┐                        │
│              │ prometheus.Gatherers │                        │
│              │   promhttp.Handler  │                        │
│              │    GET /metrics     │                        │
│              └─────────────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

**双 Registry 设计:** 基础设施指标和 AI 业务指标分别使用独立的 `prometheus.Registry`，通过 `prometheus.Gatherers` 合并输出到 `/metrics` 端点。这保证了两组指标的注册隔离，避免命名冲突。

---

## 7. PromQL 查询示例

### HTTP 请求速率 (QPS)

```promql
rate(iac_http_requests_total[5m])
```

### P99 请求延迟

```promql
histogram_quantile(0.99, rate(iac_http_request_duration_seconds_bucket[5m]))
```

### 按路由分组的错误率

```promql
sum(rate(iac_http_requests_total{status=~"5.."}[5m])) by (route)
/
sum(rate(iac_http_requests_total[5m])) by (route)
```

### 数据库查询速率（按操作类型）

```promql
rate(iac_db_queries_total[5m])
```

### P95 数据库查询延迟

```promql
histogram_quantile(0.95, rate(iac_db_query_duration_seconds_bucket[5m]))
```

### AI 调用成功率

```promql
sum(rate(iac_ai_call_total{status="success"}[5m]))
/
sum(rate(iac_ai_call_total[5m]))
```

### AI 调用 P99 延迟（毫秒）

```promql
histogram_quantile(0.99, rate(iac_ai_call_duration_ms_bucket[5m]))
```

### 连接池使用率

```promql
iac_db_connections_open / iac_db_connections_max
```
