## v0.4.7

修复 Cancel Previous、Swagger 文档、通知系统及 decision_required 状态显示。

### Bug Fixes

- **Cancel Previous 无法取消 decision_required 任务** — 查询条件遗漏了 `decision_required` 和 `waiting` 状态，导致处于风险决策阶段的任务无法被取消，workspace 锁无法释放 (`workspace_task_controller.go`)
- **Cancel Previous 失败时仍返回 200** — 当没有任务被取消时，API 现在正确返回 400 错误而非虚假的成功响应 (`workspace_task_controller.go`)
- **Swagger 文档 CORS 错误** — 前端 SwaggerUI 写死 `http://localhost:8080`，改为相对路径 `/swagger/doc.json`；nginx 新增 `/swagger/` 反向代理 (`SwaggerUI.tsx`, `docker-compose.nginx.conf`, `nginx.conf`, `vite.config.ts`)
- **Swagger host 字段清理** — 移除 `@host localhost:8080` 注释及生成文件中的硬编码 host，Swagger UI 自动使用当前访问地址 (`main.go`, `docs.go`, `swagger.json`, `swagger.yaml`)
- **AI Plan Summary 风险决策不触发** — 当 AI 返回 V2 格式（无 `risk_evaluation` 结构）时，`requires_confirmation` 始终为 false；新增 V2 兜底逻辑：`risk_level` 为 high/critical 时自动启用人工确认 (`ai_summary_service.go`)
- **decision_required 状态显示缺失** — Workspace 列表、TaskDetail、WorkspaceDetail 的 Runs 列表均未处理该状态，现已补全显示为 "Decision Required"（黄色，与 apply_pending 一致）

### Improvements

- **全局通知系统** — 新增 `NotificationProvider` + `NotificationContainer`，在 App 层挂载，左下角弹窗通知，样式与 SimpleToast 统一（纯色背景、320px 宽、4 秒自动消失）
- **TaskDetail 通知统一** — 所有操作通知（cancel previous、confirm apply、cancel、override）从 `useToast` 迁移至左下角 `NotificationContext`

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.6...v0.4.7
