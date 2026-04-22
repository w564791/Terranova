## v0.6.4

resource_drift 仅对 drift_check 任务生效（对齐 TFE 行为）、CMDB 搜索展示已删除资源、搜索去重优先活跃记录、展开详情交互优化、amd64 构建支持、剪贴板 HTTP 兼容、敏感凭据清理。

### Bug Fixes

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
