## v0.4.0

Execute Flow Summary: AI 驱动的变更影响分析与执行结果摘要。

### New Features

- **AI Agent Loop Framework** — 通用 AI 自主循环框架，类似 n8n AI Agent 节点。AI 完全控制流程：自主决定调用工具、收集信息、生成最终输出。支持 Bedrock (Claude tool_use) 和 OpenAI (function_calling) 两种 tool-calling 协议，可复用于未来其他 AI 场景 (`ai_agent_loop.go`, `ai_caller.go`)
- **Plan Summary（变更影响分析）** — Plan 完成后自动异步生成。AI 分析资源变更，通过 CMDB 查询依赖关系，评估影响范围和风险等级。结果不可变，存储在 `ai_plan_summaries` 表 (`ai_summary_service.go`, `ai_summary_tools.go`)
- **Apply Summary（执行结果分析）** — Apply 完成后自动异步生成。AI 总结执行结果并与 Plan 阶段预测对比，标注偏差和意外变更。结果存储在 `ai_apply_summaries` 表
- **Summary 工具集** — 5 个 AgentTool 实现：query_module_resources（补全 module 上下文）、query_cmdb_dependencies（依赖方搜索）、query_resource_attributes（资源属性查询）、query_state_resources（工作空间资源概览）、query_plan_summary（Plan 预测对比）(`ai_summary_tools.go`)
- **输出验证与自动重试** — AI Agent Loop 支持 OutputValidator 回调，AI 输出格式不正确时自动反馈错误让 AI 修正，最多重试 2 次 (`ai_agent_loop.go`)
- **基础设施风险基线 Skill** — Foundation 层 skill，定义 critical/high 风险判定规则：公网暴露数据库端口、删除安全组/IAM、禁用加密等 (`skill/security/foundation/infrastructure_risk_baseline.md`)
- **执行摘要工作流 Skill** — Task 层 skill，定义 Plan/Apply 两阶段的分析策略和输出格式 (`skill/execute_summary/task/execute_summary_workflow.md`)
- **前端 ExecuteSummary 组件** — Plan Card 内展示变更影响分析（位于资源详情上方），Apply Card 内展示执行结果分析（位于资源详情下方）。支持加载/分析中/失败状态，失败可手动重试。影响详情和受影响资源默认折叠 (`ExecuteSummary.tsx`)
- **AI Config 新增 summary capability** — 支持 prompt/skill/auto 三种模式配置，可在 AI 配置管理界面选择 Skill 模式并配置 foundation + task skills

### Bug Fixes

- **Plan Summary 触发时序** — 修复 plan_and_apply 类型任务的 summary 触发点，确保在 changes 字段写入 DB 后再触发
- **前端数据获取** — 修复 axios 拦截器双重 `.data` 访问和 error 字符串处理
- **Controller 授权** — 新增 workspace 归属校验，防止跨 workspace 越权查询 summary
- **JSON 解析容错** — AICaller 的 tool call 参数解析错误不再静默忽略，记录日志
- **Retry 路径** — delete 操作增加错误检查
- **输入框 `/` 触发** — 安全组 CIDR 等含 `/` 的值不再误触发引用选择器，继续输入自动关闭菜单并保留 `/`

### Improvements

- **docker-compose.yml** — backend/frontend 新增 `pull_policy: always`
- **Release CI** — changelog 目录从 `release/` 改为 `changelog/`

### Database Migration

执行 `backend/migrations/add_ai_summary_tables.sql` 创建 `ai_plan_summaries` 和 `ai_apply_summaries` 表。

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.3.5...v0.4.0
