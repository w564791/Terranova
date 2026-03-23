## v0.4.3

Plan 人机协同决策 + 任务创建优化。

### New Features

- **Plan 风险决策确认** — AI 判断 plan 变更为 high/critical 风险时，阻塞 apply 流程（`decision_required` 状态），要求任务提交人/admin 确认决策后才能继续。决策自动写入 task 评论供审批人参考 (`ai_summary_service.go`, `ai_summary_controller.go`)
- **决策确认 UI** — Plan Summary 区域显示场景化决策选项（安全组变更/资源删除/IAM 权限变更/核心网络变更），支持单选 + 补充说明 (`ExecuteSummary.tsx`)
- **Apply 前弹窗提示** — 点击 Confirm Apply 时检查 summary 状态，未完成或未确认时弹窗提醒 (`TaskDetail.tsx`)
- **Execute Summary Skill V3** — 完整的风险评估框架：blast_radius 判定、不确定性模型、风险评分规则、人工确认触发规则、结构化决策场景 (`execute_summary_workflow.md`)

### Bug Fixes

- **Plan-only 任务创建** — Workspace 锁定时允许创建和执行 plan-only 任务（不修改 state），plan_and_apply 排队等待 (`workspace_task_controller.go`, `task_queue_manager.go`)
- **Plan 卡片消失** — `decision_required` 状态下 Plan Card 和 StructuredRunOutput 正常显示 (`TaskTimeline.tsx`, `StructuredRunOutput.tsx`)
- **AI 输出防御性渲染** — 所有 AI 返回数据使用 `safeRender` 防止对象类型导致 React 白屏 (`ExecuteSummary.tsx`)
- **Apply 摘要简化** — 去掉决策回顾（变更已完成无意义），只做执行结果分析
- **用户名显示** — 决策确认人显示 username 而非 user_id
- **New Run 页面刷新** — 任务详情页 New Run 后正确切换到新任务

### Database Migration

执行 `backend/migrations/add_plan_decision.sql`，为 `ai_plan_summaries` 表新增 7 个决策字段。

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.2...v0.4.3
