## v0.4.11

Extended Thinking 支持 + Qwen/DashScope 接入 + CMDB 观测面板 + 评估去重修复。

### Features

#### CMDB 观测面板

- **Dashboard 新增 "CMDB 概览" Tab** — 在首页 Dashboard 增加第三个 Tab，集中展示 CMDB 系统运行状态 (`Dashboard.tsx`, `CMDBOverviewDashboard.tsx`)
- **数据源概览** — 展示 Workspace 数量、外部数据源数量及健康状态、资源总数（含 Workspace/外部源拆分）
- **Embedding & Summary 覆盖率** — 进度条可视化展示覆盖率百分比，颜色按阈值自动变化（绿 ≥90% / 橙 ≥60% / 红 <60%）
- **任务队列监控** — 分别展示 Embedding 队列 (Workspace)、Summary 队列 (外部源)、Embedding 队列 (外部源) 的待处理/处理中/失败数
- **资源类型分布** — 横向条形图展示 Top 10 资源类型，底部附来源占比条（Workspace vs 外部源）
- **同步历史（分页）** — 统一展示 Workspace + 外部源的同步记录，含来源类型、触发方式、增删改计数，后端分页默认 10 条/页 (`GET /cmdb/sync-history`)
- **Workspace 同步日志** — Workspace 同步现在也写入 `cmdb_sync_logs`，和外部源同步日志统一管理，支持区分 `triggered_by`（auto/manual/scheduled）
- **Tab 按需加载** — Dashboard Tabs 启用 `destroyInactiveTabPane`，切换 Tab 时重新加载数据，避免过早请求和数据过期

#### Extended Thinking

- **AI Config 扩展** — 新增 `thinking_enabled` / `thinking_budget_tokens` 配置项，支持 Claude extended thinking 模式 (`ai_config.go`, `ai_config_service.go`)
- **Bedrock Thinking 集成** — 请求构建时注入 thinking block，响应解析提取 thinking content + signature，多轮对话中保持 thinking 链路连贯 (`ai_caller.go`, `ai_agent_loop.go`)
- **Thinking 内容持久化** — plan/apply summary 保存每轮迭代的 thinking content 到 JSONB 字段，便于调试和审计 (`ai_summary_service.go`, `ai_summary.go`)
- **前端 Thinking 展示** — plan/apply summary 结果页增加可折叠的 thinking 内容区域 (`ExecuteSummary.tsx`)
- **前端配置表单** — AI Config 表单新增 Extended Thinking 开关和 budget tokens 输入 (`AIConfigForm.tsx`)

#### Qwen/DashScope 接入

- **QwenCaller** — 新增 DashScope OpenAI-compatible API 调用器，支持 thinking 能力 (`ai_caller.go`)
- **模型列表接口** — 新增 `/ai-config/openai-models` 端点，从 OpenAI-compatible API 获取可用模型列表 (`ai_controller.go`, `router_global.go`)
- **前端模型选择** — Qwen 类型支持下拉选择模型，自动过滤非对话类模型 (`AIConfigForm.tsx`, `ai.ts`)
- **DashScope API Key** — 支持环境变量 `DASHSCOPE_API_KEY` 作为 fallback (`config.go`)

#### CMDB 搜索召回质量分析

- **搜索日志记录** — VectorSearch handler 异步写入搜索日志到 `cmdb_search_logs` 表，记录 query、search_method、结果数、similarity、耗时等指标 (`embedding_controller.go`)
- **搜索来源标记** — 区分用户主动搜索 (manual) 和输入防抖自动搜索 (auto)，分析 API 默认只统计 manual，避免中间态查询污染数据 (`CMDB.tsx`, `cmdb.ts`)
- **Query 归一化** — 搜索日志写入前执行 `ToLower + TrimSpace`，避免大小写/空格导致聚合分裂
- **搜索分析 API** — 新增 `GET /cmdb/search-analytics?period=7d` 端点，返回使用统计（搜索次数、零结果率、平均结果数）、质量指标（method 分布、similarity、fallback 率、耗时）、热门查询 Top 30、零结果查询 Top 10
- **Dashboard 搜索质量面板** — CMDB 概览 Tab 新增搜索召回质量 section：5 个指标卡片 + 搜索方式分布条形图 + 纯 CSS 词云（热门查询 Top 30）+ 零结果查询列表 (`CMDBOverviewDashboard.tsx`)
- **Period 切换** — 支持 24h / 7d / 30d 时间范围切换，独立加载不影响其他 section
- **日志自动清理** — 复用 EmbeddingWorker 每日清理机制，自动删除 30 天前的搜索日志 (`embedding_worker.go`)

### Bug Fixes

- **Embedding 覆盖扩展** — 对没有 summary 的资源，也用 `BuildEmbeddingText` 生成 embedding（这些资源有 name/description/tags，足以生成有意义的 embedding）
- **Summary 覆盖无 attributes 资源** — 没有 attributes 但有 name/description/tags 的资源不再跳过，用元数据生成 summary (`resource_summary_service.go`)
- **评估记录去重** — 修复竞态条件导致的重复 assessment 记录，添加 `(usage_log_id, assessment_layer)` 唯一约束 (`fix_duplicate_assessment_records.sql`, `skill_assessment_worker.go`)
- **外部 CMDB 重建优化** — 重建时 summary 只对缺失的资源生成（靠 hash 跳过未变更的），embedding 全量重建；同时清理已删除数据源的孤儿资源 (`embedding_controller.go`)
- **Embedding 重建不再清空数据** — `RebuildWorkspace` 和 `rebuildExternalEmbedding` 不再先清空所有 embedding，改为逐条覆盖，重建期间旧 embedding 仍可搜索 (`embedding_worker.go`, `embedding_controller.go`)
- **Summary 更新不再清空 embedding** — summary 生成后不再置空 embedding，由 PostSyncWorker 通过 `embedding_text != resource_summary` 检测变更并自动覆盖 (`resource_summary_service.go`)
- **API Key 更新逻辑** — 切换 service type 时正确处理 API Key 持久化，支持清除已存储的 Key (`ai_config_service.go`)
- **重建进度显示修正** — 进度 badge 改为显示任务数而非百分比；外部 CMDB 进度从 `cmdb_post_sync_jobs` 表查询（之前查 `embedding_tasks` 导致始终为 0）；确认弹窗文案更新 (`embedding_worker.go`, `CMDB.tsx`, `ExternalSourcesTab.tsx`)
- **Summary prompt 优化** — plan/apply 阶段标注 stage 前缀，过滤 no-op 资源 (`ai_summary_service.go`)

### Database Migrations

```sql
-- 扩展 cmdb_sync_logs 支持 Workspace 同步日志 (add_cmdb_sync_log_source_type.sql)
ALTER TABLE cmdb_sync_logs DROP CONSTRAINT IF EXISTS fk_cmdb_sync_logs_source;
ALTER TABLE cmdb_sync_logs ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) NOT NULL DEFAULT 'external';
ALTER TABLE cmdb_sync_logs ADD COLUMN IF NOT EXISTS source_name VARCHAR(200) DEFAULT '';
ALTER TABLE cmdb_sync_logs ADD COLUMN IF NOT EXISTS triggered_by VARCHAR(20) DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_source_type ON cmdb_sync_logs (source_type);
CREATE INDEX IF NOT EXISTS idx_cmdb_sync_logs_completed_at ON cmdb_sync_logs (completed_at DESC);

-- Extended Thinking 配置 (add_thinking_config.sql)
ALTER TABLE ai_configs ADD COLUMN thinking_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE ai_configs ADD COLUMN thinking_budget_tokens integer NOT NULL DEFAULT 10000;
ALTER TABLE ai_plan_summaries ADD COLUMN thinking_content jsonb;
ALTER TABLE ai_apply_summaries ADD COLUMN thinking_content jsonb;

-- CMDB 同步后处理任务队列 (add_cmdb_post_sync_jobs.sql)
CREATE TABLE IF NOT EXISTS cmdb_post_sync_jobs (
    id            SERIAL PRIMARY KEY,
    source_id     VARCHAR(50) NOT NULL,
    job_type      VARCHAR(20) NOT NULL,
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    depends_on    INTEGER REFERENCES cmdb_post_sync_jobs(id) ON DELETE SET NULL,
    error_message TEXT DEFAULT '',
    retry_count   INTEGER DEFAULT 0,
    created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    started_at    TIMESTAMP,
    completed_at  TIMESTAMP
);
CREATE INDEX idx_post_sync_jobs_source ON cmdb_post_sync_jobs (source_id);
CREATE INDEX idx_post_sync_jobs_status ON cmdb_post_sync_jobs (status);
CREATE INDEX idx_post_sync_jobs_depends ON cmdb_post_sync_jobs (depends_on);

-- CMDB 搜索日志表 (add_cmdb_search_logs.sql)
CREATE TABLE IF NOT EXISTS cmdb_search_logs (
    id              BIGSERIAL PRIMARY KEY,
    query           TEXT        NOT NULL,
    resource_type   VARCHAR(100) DEFAULT '',
    search_method   VARCHAR(20) NOT NULL,
    source          VARCHAR(10) NOT NULL DEFAULT 'manual',
    total_count     INT         NOT NULL DEFAULT 0,
    vector_count    INT         NOT NULL DEFAULT 0,
    keyword_count   INT         NOT NULL DEFAULT 0,
    top_similarity  REAL        DEFAULT 0,
    avg_similarity  REAL        DEFAULT 0,
    duration_ms     INT         NOT NULL DEFAULT 0,
    fallback_reason VARCHAR(200) DEFAULT '',
    user_id         VARCHAR(100) DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_search_logs_created_at ON cmdb_search_logs (created_at);

-- 评估去重 (fix_duplicate_assessment_records.sql)
CREATE UNIQUE INDEX idx_assessment_usage_log_layer_unique
  ON skill_assessment_results (usage_log_id, assessment_layer);
```

### API Changes

- `GET /cmdb/overview` — CMDB 观测面板数据（数据源、资源、Embedding/Summary 覆盖率、任务队列）
- `GET /cmdb/sync-history?page=1&size=10` — 同步历史分页查询（统一 Workspace + 外部源）
- `GET /cmdb/search-analytics?period=7d&source=manual` — 搜索召回质量分析（使用统计 + 质量指标 + 热门查询 + 零结果查询）

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.10...v0.4.11
