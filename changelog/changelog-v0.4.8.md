## v0.4.8

State File Watcher 实时状态监控、AI Plan Summary CMDB 上下文修复、风险决策 UI 重构。

### Features

#### State File Watcher — Apply 过程中实时监控 tfstate 变化

- **核心组件 `StateFileWatcher`** — 使用 fsnotify 监控 terraform.tfstate 文件变化，实时解析 state 并写入数据库临时记录（`is_temp=true`），Apply 完成后自动清理 (`state_file_watcher.go`)
- **集成到 ExecuteApply** — Apply 执行前启动 watcher，执行后停止并清理；支持 fallback，watcher 启动失败不影响 Apply 执行 (`terraform_executor.go`)
- **Agent API 临时状态端点** — 新增 `GET /latest-temp-state`、`DELETE /temp-states` 端点，供外部系统查询和清理临时状态 (`agent_handler.go`, `router_agent.go`)
- **DataAccessor 扩展** — `LocalDataAccessor` 和 `RemoteDataAccessor` 新增 `SaveTempStateVersion`、`CleanupTempStateVersions` 方法 (`data_accessor.go`, `local_data_accessor.go`, `remote_data_accessor.go`)
- **is_temp 全局过滤** — 通过 GORM callback 自动为 `workspace_state_versions` 查询添加 `is_temp = false` 条件，防止临时记录污染正常查询 (`state_version_scope.go`)
- **数据库迁移** — `workspace_state_versions` 新增 `is_temp` 列，默认 false，含部分索引 (`add_is_temp_to_state_versions.sql`)
- **单元测试** — StateFileWatcher 核心逻辑测试覆盖 (`state_file_watcher_test.go`)

#### AI Plan Summary — CMDB 上下文修复与风险决策 UI 重构

- **query_resource_attributes 复用 CMDB 关键字搜索** — 入参简化为单个 `query` 字段，复用 `CMDBService.SearchResources` 做模糊匹配，自动跨 workspace 查询含外部 CMDB 数据 (`ai_summary_tools.go`)
- **Plan Summary 注入 CMDB 上下文** — `buildSystemPrompt` 设置 `UseCMDB: true` (`ai_summary_service.go`)
- **风险决策 UI 重构** — 去掉 4 个固定 scenario 模板，改为 AI 动态生成：
  - `decision_title`: 具体风险标题（含资源 ID）
  - `risk_highlights`: 3-5 条关键风险点
  - `recommended_actions`: "我已经知晓: xxx" 确认项（checkbox 多选），全部勾选才能确认
  - ABORT 渲染为独立的"终止本次变更"按钮
  - (`ExecuteSummary.tsx`, `ExecuteSummary.module.css`, `execute_summary_workflow.md`)
- **Skill 引导 create 场景查询** — AI 在 create 场景通过 `query_resource_attributes` 查询引用的子网/VPC 属性 (`execute_summary_workflow.md`)
- **Agent Loop 并发 tool call** — 同一轮多个 tool call 并发执行 (`ai_agent_loop.go`)
- **Agent Loop Prometheus 监控** — 新增 `iac_agent_loop_step_duration_ms`（每轮 AI 耗时）、`iac_agent_loop_tool_duration_ms`（tool 执行耗时）、`iac_agent_loop_total_duration_ms`（总耗时）、`iac_agent_loop_step_total`（轮次计数）(`ai_metrics.go`)
- **CMDB 测试数据** — cmdb-test-server 新增 subnet-01bc9ccfe9259b6e7 测试资源 (`cmdb-test-server/main.go`)
- **V3/V4 decision_hints 兼容** — `parseDecisionHints` 同时支持数组（V3）和对象（V4）格式 (`ai_summary_service.go`)

### Bug Fixes

- **外部 CMDB 子网/VPC 数据查不到** — `query_resource_attributes` 限定 workspace_id 导致 `__external__` 数据不可见 (`ai_summary_tools.go`)
- **Plan Summary 风险确认与变更内容不匹配** — 固定 scenario 模板（如 "Security Group Change Confirmation"）在 create EC2 场景出现 (`execute_summary_workflow.md`)
- **多选 decision_code 校验失败** — confirm 接口改为支持逗号分隔多 code (`ai_summary_controller.go`)
- **State watcher 孤儿记录清理** — 事务内按正确顺序清理，防止残留 (`state_file_watcher.go`)
- **Agent API 查询泄露临时记录** — 通过 GORM scope 全局过滤 is_temp (`state_version_scope.go`)

### Database Migrations

```sql
-- State watcher
ALTER TABLE workspace_state_versions
    ADD COLUMN is_temp boolean DEFAULT false;
CREATE INDEX idx_wsv_is_temp ON workspace_state_versions (is_temp) WHERE is_temp = true;

-- Plan summary decision
ALTER TABLE ai_plan_summaries
    ADD COLUMN decision_title text,
    ADD COLUMN risk_highlights jsonb;
ALTER TABLE ai_plan_summaries
    ALTER COLUMN user_decision_code TYPE text;
```

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.7...v0.4.8
