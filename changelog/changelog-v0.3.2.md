## v0.3.2

AI 错误分析 prompt 重构：失败资源/成功资源/错误信息独立占位符，支持资源配置详情，支持用户自定义 prompt 模板。

### Enhancements

- **AI 错误分析 prompt 占位符拆分** — 将原有的单一 `{resource_context}` 拆分为 `{failed_resources}`、`{succeeded_resources}`、`{error_message}` 三个独立占位符，用户可在 AI config 的 `capability_prompts.error_analysis` 中自由控制各部分的渲染位置和格式；默认 prompt 同样使用占位符替换，与自定义 prompt 统一逻辑 (`ai_analysis_service.go`)
- **错误分析携带资源配置详情** — 失败资源附带 `ChangesAfter`（计划配置 JSON），成功资源从 `workspace_task_resource_changes` 历史记录中查询同 workspace + 同 module 下最新的 `ChangesAfter`；AI 可基于实际配置分析依赖关系和配置差异 (`ai_analysis_service.go`)
- **成功资源查询兼容重试场景** — 重试任务可能写入 `changes_after = NULL` 的记录，查询时增加 `changes_after IS NOT NULL` 过滤，确保取到首次任务或最近一次 update 的完整配置数据 (`ai_analysis_service.go`)
- **错误分析 prompt 日志输出** — 调用 AI 前输出完整 prompt（含 taskID、service 类型、model ID），便于排查分析结果不理想的问题 (`ai_analysis_service.go`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.3.1...v0.3.2
