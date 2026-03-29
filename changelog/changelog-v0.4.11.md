## v0.4.11

Extended Thinking 支持 + Qwen/DashScope 接入 + CMDB 观测面板 + Summary 质量评估 + Skill 迭代。

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
- **搜索来源标记** — 区分用户主动搜索 (manual)、输入防抖自动搜索 (auto)、AI agent 搜索 (agent)，避免中间态查询污染数据 (`CMDB.tsx`, `cmdb.ts`, `ai_summary_tools.go`)
- **AI Agent 搜索日志** — `QueryResourceAttributesTool` 和 `QueryCMDBDependenciesTool` 也写入搜索日志（`source='agent'`），覆盖成功、零结果、查询失败三条路径 (`ai_summary_tools.go`)
- **Query 归一化** — 搜索日志写入前执行 `ToLower + TrimSpace`，避免大小写/空格导致聚合分裂
- **搜索分析 API** — 新增 `GET /cmdb/search-analytics?period=7d&source=all` 端点，返回使用统计（搜索次数、零结果率、平均结果数）、质量指标（method 分布、similarity、fallback 率、耗时）、热门查询 Top 30、零结果查询 Top 10。source 参数支持 `all`/`manual`/`auto`/`agent` 过滤
- **Dashboard 搜索质量面板** — CMDB 概览 Tab 新增搜索召回质量 section：5 个指标卡片 + 搜索方式分布条形图 + 纯 CSS 词云（热门查询 Top 30）+ 零结果查询列表 (`CMDBOverviewDashboard.tsx`)
- **Period + Source 切换** — 支持 24h / 7d / 30d 时间范围 + 全部 / 用户 / Agent 来源切换，独立加载不影响其他 section
- **日志自动清理** — 复用 EmbeddingWorker 每日清理机制，自动删除 30 天前的搜索日志 (`embedding_worker.go`)

#### Summary 质量评估

- **三层评估架构** — L1 文本规则校验 (`summary_text_validator.go`)、L2 规则一致性 LLM 评估、L3 语义质量 LLM 评估，复用 skill assessment 框架扩展而来
- **采样策略** — 按资源类型分层采样，支持配置采样率，避免全量评估的开销 (`summary_assessment_sampler.go`)
- **编排服务** — 串联 L1→L2→L3 评估流程，L1 失败可提前终止 (`summary_assessment_service.go`)
- **PostSyncWorker 集成** — 外部源同步完成后自动触发 summary 评估，评估结果写入 `skill_assessment_results`（`source_type='summary'`）
- **启动补偿** — 服务启动时自动恢复 `pending` 状态的评估任务，防止因重启丢失 (`summary_assessment_service.go`)
- **Dashboard 质量 Tab** — 新增 Summary Quality Tab，展示评估覆盖率、各层通过率、问题分布 (`SummaryQualityTab.tsx`)
- **Issue Drill-down** — 点击问题类型可查看具体资源列表，支持按 resource_id 跳转 (`SummaryQualityTab.tsx`)
- **摘要重新生成** — Issue 详情中支持一键重新生成 summary，并将评估发现的问题作为质量反馈传给 AI (`summary_regeneration_hint`)
- **评估 AI Config** — 新增 `summary_rule_evaluation` / `summary_semantic_evaluation` 两个 capability config，mode 为 `prompt`（无 skill 编排）
- **Dashboard API** — 新增 `GET /summary-assessment/dashboard` 端点，返回评估统计和问题列表 (`summary_assessment_controller.go`)

#### Skill 质量迭代

- **service_disruption 风险因子** — execute_summary_workflow 新增服务中断风险评估，高影响变更标记 `service_disruption` (`execute_summary_workflow.md`)
- **高 blast radius 深度分析** — 当变更影响范围大时，强制要求 deep-dive 查询依赖关系 (`execute_summary_workflow.md`)
- **Tool calls 两轮拆分** — 将工具调用拆分为两轮，第一轮查基础信息，第二轮查依赖关系，确保依赖查询基于第一轮结果 (`execute_summary_workflow.md`)
- **resource_summary 用于风险评估** — plan/apply 阶段引入已有的 resource_summary 作为上下文，提升变更影响分析的准确性 (`ai_cmdb_service.go`)

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

-- Summary 评估扩展 (add_summary_assessment.sql)
ALTER TABLE skill_assessment_results DROP CONSTRAINT IF EXISTS skill_assessment_results_usage_log_id_fkey;
ALTER TABLE skill_assessment_results ALTER COLUMN usage_log_id DROP NOT NULL;
ALTER TABLE skill_assessment_results ADD COLUMN IF NOT EXISTS source_type VARCHAR(16) DEFAULT 'skill';
ALTER TABLE skill_assessment_results ADD COLUMN IF NOT EXISTS resource_id INTEGER;
ALTER TABLE skill_assessment_results ADD COLUMN IF NOT EXISTS format_violations TEXT[];
ALTER TABLE skill_assessment_results ADD COLUMN IF NOT EXISTS security_tag_misses JSONB;
ALTER TABLE skill_assessment_results ADD COLUMN IF NOT EXISTS hallucination_suspects TEXT[];
CREATE INDEX IF NOT EXISTS idx_assessment_source_type ON skill_assessment_results (source_type);
CREATE INDEX IF NOT EXISTS idx_assessment_resource_id ON skill_assessment_results (resource_id) WHERE resource_id IS NOT NULL;
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS summary_assessment_status VARCHAR(16) DEFAULT '';
ALTER TABLE resource_index ADD COLUMN IF NOT EXISTS summary_regeneration_hint TEXT DEFAULT '';

-- Summary 评估 AI Config (add_skill_quality_assessment_config.sql 部分)
-- summary_rule_evaluation + summary_semantic_evaluation 两个 capability config (mode='prompt')

-- 评估去重 (fix_duplicate_assessment_records.sql)
CREATE UNIQUE INDEX idx_assessment_usage_log_layer_unique
  ON skill_assessment_results (usage_log_id, assessment_layer);
```

### API Changes

- `GET /cmdb/overview` — CMDB 观测面板数据（数据源、资源、Embedding/Summary 覆盖率、任务队列）
- `GET /cmdb/sync-history?page=1&size=10` — 同步历史分页查询（统一 Workspace + 外部源）
- `GET /cmdb/search-analytics?period=7d&source=all` — 搜索召回质量分析（使用统计 + 质量指标 + 热门查询 + 零结果查询），source 支持 `all`/`manual`/`auto`/`agent`
- `GET /admin/summary-assessment/overview` — Summary 质量评估统计（覆盖率、各层通过率、问题分布）
- `GET /admin/summary-assessment/issue-resources?type=over_length&days=7` — 按问题类型查询具体资源列表
- `POST /admin/summary-assessment/regenerate` — 根据评估反馈批量重新生成 summary

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.10...v0.4.11
