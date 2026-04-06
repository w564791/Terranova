## v0.6.2

Agent 模式 resource_drift 双重计数修复、Apply 后 Server 端补偿逻辑、Plan/Apply 前端显示优化、AI Summary 运维工具增强。

### Bug Fixes

#### Agent 模式 resource_drift 双重计数

- **修复** `parseResourceChangesFromPlanJSON` 无条件追加 `resource_drift` 到 `resource_changes`，导致正常 plan 模式下 drift 资源被错误写入 `workspace_task_resource_changes`（如 no-op 的 update drift 被当作真实 update）
- **对齐** `plan_parser_service.go` 的 `hasRealChanges` 判断逻辑：仅在 refresh-only plan（全 no-op）时才 fallback 到 `resource_drift`

#### Agent 模式 Apply 完成后缺少 Server 端补偿

- **修复** Agent apply 完成后 `pending` 资源未批量更新为 `completed`：Agent 端 `db=nil` 跳过了 `terraform_executor.go` 中的批量更新，Server 端 `postApplyCompletionTasks` 未补做
- **修复** Agent apply 完成后 `applied_code_version` 未回填：同上原因，`backfillAppliedCodeVersion` 在 Agent 模式下被跳过
- **新增** `postApplyCompletionTasks` 两个步骤：pending->completed 批量更新 + applied_code_version 回填，与 Local 模式对齐

#### Plan/Apply 前端显示优化

- **修复** Plan tab 资源过滤器默认包含 data source (read)，现在默认排除，用户可手动勾选查看
- **修复** Apply tab data source 资源显示无意义的 Pending 状态，现在 apply 视图不再显示 data source
- **修复** Apply 完成后 plan 预测变更但实际未变的资源显示 Pending，现在推断为 Unchanged 状态
- **优化** Apply 完成后去掉冗余的 action badge（UPDATE/CREATE），状态文字已足够
- **优化** plan changes download 按钮移入 filter 行，与搜索框和过滤器同行显示

### Enhancements

#### AI Summary 运维工具

- **新增** StopApplySummary API：强制终止卡住的 running 状态 apply summary
- **优化** RetryApplySummary 放宽条件：支持 `running` 状态重试，处理部署重启导致的孤儿记录
- **修复** `extractJSON` 增加裸 JSON 提取：当 AI 响应无 markdown 标记时，定位第一个 `{` 到最后一个 `}` 之间的内容

#### CMDB Dashboard

- **优化** 词云组件：条目上限 30、容器 max-height 防溢出、文字 overflow ellipsis、字体范围缩小
