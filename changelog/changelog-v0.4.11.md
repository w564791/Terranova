## v0.4.11

Extended Thinking 支持 + Qwen/DashScope 接入 + 评估去重修复。

### Features

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

### Bug Fixes

- **评估记录去重** — 修复竞态条件导致的重复 assessment 记录，添加 `(usage_log_id, assessment_layer)` 唯一约束 (`fix_duplicate_assessment_records.sql`, `skill_assessment_worker.go`)
- **API Key 更新逻辑** — 切换 service type 时正确处理 API Key 持久化，支持清除已存储的 Key (`ai_config_service.go`)
- **Summary prompt 优化** — plan/apply 阶段标注 stage 前缀，过滤 no-op 资源 (`ai_summary_service.go`)

### Database Migrations

```sql
-- Extended Thinking 配置 (add_thinking_config.sql)
ALTER TABLE ai_configs ADD COLUMN thinking_enabled boolean NOT NULL DEFAULT false;
ALTER TABLE ai_configs ADD COLUMN thinking_budget_tokens integer NOT NULL DEFAULT 10000;
ALTER TABLE ai_plan_summaries ADD COLUMN thinking_content jsonb;
ALTER TABLE ai_apply_summaries ADD COLUMN thinking_content jsonb;

-- 评估去重 (fix_duplicate_assessment_records.sql)
-- 删除重复记录，保留最早的
CREATE UNIQUE INDEX idx_assessment_usage_log_layer_unique
  ON skill_assessment_results (usage_log_id, assessment_layer);
```

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.10...v0.4.11
