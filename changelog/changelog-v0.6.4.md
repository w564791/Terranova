## v0.6.4

resource_drift 仅对 drift_check 任务生效（对齐 TFE 行为）、CMDB 搜索展示已删除资源、搜索去重优先活跃记录、展开详情交互优化、amd64 构建支持、剪贴板 HTTP 兼容、敏感凭据清理、Workspace 列表页与 Overview 页状态显示统一。

### Bug Fixes

#### Workspace 状态显示跨页面不一致

- **修复** Workspace 列表页与详情页 Overview 挑选 Latest Run 的规则不同，导致同一 workspace 在两处展示不同任务状态
  - 后端 `getLatestRun` 与 `SearchWorkspacesWithStatus` 的 SQL 排序统一为「needs_attention 优先（`apply_pending` / `decision_required`），其余一律按 `created_at DESC`」，移除原先对 `running` / `pending` / `waiting` 的多级优先级
  - 详情页 Overview 前端删除独立的 `fetchGlobalLatestRun`（从 `/tasks` 取前 10 条做 JS 筛选），改以 `overview.latest_run` 作为唯一全局数据源，避免 needs_attention 任务排在第 10 条之后时前端选不到的缺陷

#### 列表页 Planned 显示错误

- **修复** 列表页对 `status=success` 的任务一律渲染为 "Applied"，plan 任务被误标为 Applied
  - 后端 `WorkspaceWithStatus` 新增 `latest_run_task_type` 字段暴露 `task_type`
  - 前端按 `task_type=plan` 显示 Planned，`apply` / `plan_and_apply:applied` 显示 Applied，与详情页 `getFinalStatus` 保持一致

#### Overview Latest Run 实时刷新

- **新增** `WorkspaceDetail` 父级挂载 `useTaskAutoRefresh`：任一 tab 下 incomplete 任务状态变化都会同步刷新 overview，Latest Run badge 随轮询实时更新
- **新增** `RunsTab` 检测到 incomplete 任务签名变化时通过 `onLatestRunChange` 回调联动刷新 overview，避免 Runs 页与 Overview 页脱节
- **扩展** `LatestRunInfo` 返回 `description` 字段，供前端 Latest Run 卡片显示任务标题

#### resource_drift 仅限 drift_check 任务

- **修复** `parsePlanChanges`、`parseResourceChanges`、`parseResourceChangesFromPlanJSON` 三处 resource_drift fallback 逻辑：之前当 resource_changes 全为 no-op 时无条件 fallback 到 resource_drift，导致 plan/plan_and_apply 任务误计 drift 资源
- **对齐** TFE 行为：resource_drift 仅在 `drift_check` 类型任务中作为变更来源，plan/plan_and_apply 中 resource_drift 视为 informational，不计入变更统计和资源变更列表

### Enhancements

#### CMDB 搜索展示已删除资源

- **新增** `is_resource_deleted` 字段：向量搜索（`doVectorSearch`）和关键词搜索（`SearchResources`）JOIN `workspace_resources` 时不再过滤 `is_active = true`，改为通过 CASE 表达式标记资源是否已删除
- **新增** 前端"已删除"标签（`deletedBadge`）：已删除资源在搜索结果中显示红色标记，提示"平台资源已删除，Terraform 尚未 apply"
- **新增** `ResourceSearchResult` / `SearchResult` 模型及 API 响应中的 `is_resource_deleted` 字段
- **优化** 搜索去重逻辑：同一 `resource_index` 行可能因 JOIN 产生 active + inactive 两条 `workspace_resources`，去重时优先保留活跃记录（`embedding_controller` 按 `IsResourceDeleted` 偏好替换，`cmdb_service` 按 `workspace_id|terraform_address` 去重后截断）

#### CMDB 展开详情交互优化

- **拆分** 点击行为：卡片主体点击跳转（有 jump_url 时），展开/收起改为独立按钮（`expandToggle`），不再与跳转冲突
- **简化** 详情面板布局：优先展示 Summary，其次 Tags；移除 raw Attributes 展示，无内容时显示 "No summary available"
- **新增** `expandToggle` 按钮和 `deletedBadge` 样式（CSS）

#### 剪贴板 HTTP 非安全上下文兼容

- **优化** `ApplyingView` 和 `PlanCompleteView` 剪贴板复制：`navigator.clipboard` 在 HTTP 环境不可用时 fallback 到 `document.execCommand('copy')`
- **优化** `PlanCompleteView` 复制失败时显示错误提示

#### Docker amd64 构建

- **新增** Makefile `docker-push-amd64` 和 `docker-push-frontend-amd64` 目标，支持 amd64 架构镜像构建推送

### Security

#### 敏感凭据清理

- **移除** `docker-compose.yml` 中硬编码的 AWS 示例凭据（`AWS_ACCESS_KEY_ID`、`AWS_SECRET_ACCESS_KEY`）
- **新增** `.gitignore` 排除 `docker-compose.yml`，防止本地环境配置误提交

### Database Migration

- `ai_error_analyses` 表新增 `process_log` 字段
- `init_seed_data.sql` COPY 语句同步更新
