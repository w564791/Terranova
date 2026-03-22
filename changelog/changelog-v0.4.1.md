## v0.4.1

CMDB Resource Summary: CMDB 同步时 AI 自动生成资源配置摘要，增强向量搜索和变更影响分析。

### New Features

- **CMDB Resource Summary** — CMDB 同步时为每个资源调用 AI 生成配置摘要（网络暴露、安全合规、备份状态、资源规格等），摘要存储在 `resource_index.resource_summary` 字段 (`resource_summary_service.go`)
- **Embedding 增强** — `BuildEmbeddingText` 追加资源摘要内容，向量搜索可匹配"公网暴露"、"删除保护未启用"等语义 (`embedding_service.go`)
- **CMDB 页面展示** — 资源树详情中展示 AI Summary 字段 (`CMDB.tsx`, `cmdb.ts`)
- **外置 CMDB 同步支持** — 外部数据源同步后同样生成资源摘要 (`cmdb_external_source_service.go`)
- **启动补偿机制** — Leader 启动时自动检查并补偿未完成的资源摘要，覆盖服务中断、手动导入等场景 (`main.go`, `resource_summary_service.go`)
- **AI Config 新增 cmdb_resource_summary capability** — 支持自定义 Prompt，建议使用 Haiku 模型降低成本

### Improvements

- **智能变更检测** — 基于 attributes MD5 hash（PostgreSQL JSONB 规范化排序）跳过未变更资源，避免重复调用 AI
- **智能截断** — 超大 attributes 优先保留安全相关 key（ingress/egress/policy/encryption/deletion_protection 等），移除低价值 key（after_unknown/after_sensitive 等）
- **时序保证** — Resource summary 先于 embedding 执行，确保 embedding 包含最新摘要
- **单资源 30 秒超时** — 防止单个慢 AI 调用消耗整体 5 分钟预算

### Database Migration

执行 `backend/migrations/add_resource_summary.sql`，为 `resource_index` 表新增 `resource_summary` 和 `summary_hash` 两列。

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.0...v0.4.1
