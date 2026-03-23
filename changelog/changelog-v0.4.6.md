## v0.4.6

AI 能力开关。

### New Features

- **AI 能力开关** — AI Config 页面新增"能力开关" tab，提供全局开关控制平台中嵌入的 AI 能力。三个能力独立可控 (`ai_feature_service.go`, `AIConfigList.tsx`)
  - CMDB 向量搜索 (Embedding)
  - CMDB 资源摘要
  - 变更影响分析与风险决策（Plan/Apply Summary + decision_required）

### Improvements

- **30 秒内存缓存** — 开关状态缓存避免高频 DB 查询（EmbeddingWorker 每秒 tick）
- **部分更新** — PUT API 只更新请求中包含的开关，不影响其他
- **默认启用** — key 不存在时视为 true，向后兼容

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.5...v0.4.6
