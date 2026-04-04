## v0.5.0

Variable Set -- 组织级变量集管理，支持 Global/Specific 作用域，软连接 attach 到 Workspace，优先级解析，版本控制，Terraform 执行路径集成。

### Features

#### Variable Set 核心功能

- **新增** `variable_sets`、`varset_variables`、`varset_assignments` 三张表，支持变量集的完整生命周期管理
- **新增** `VARIABLE_SETS` IAM 资源类型（ORGANIZATION scope），READ/WRITE/ADMIN 三级权限分控
- **新增** Variable Set CRUD API（`/api/v1/variable-sets`），支持 Global/Specific 两种作用域模式
- **新增** Varset Variable CRUD API，支持 terraform/environment 变量类型、string/hcl 值格式、sensitive 加密
- **新增** Assignment API，支持将 Specific 变量集 attach 到 Project 和 Workspace

#### 变量版本控制

- **新增** varset_variables 版本控制：更新变量创建新版本行，旧版本保留供任务快照引用
- **约束** 变量 key、variable_type、value_format 创建后不可修改；sensitive 只能 false->true
- **约束** 同一变量集内 key 唯一（不区分 variable_type）

#### 变量优先级解析

- **新增** `VariableResolutionService`，支持 Display（前端展示）和 Resolve（执行器）双模式
- **优先级** Workspace 变量 > Workspace 级 VarSet > Project 级 VarSet > Global VarSet
- **同层级** 后 attach 的变量集覆盖先 attach 的；Global 按创建时间排序

#### Terraform 执行路径集成

- **改造** `LocalDataAccessor.GetWorkspaceVariables()` 使用 VariableResolutionService 合并变量集变量
- **改造** `createTaskSnapshot()` 通过 ResolutionService 获取合并变量，varset 变量也纳入快照（variable_id + version）
- **改造** Agent/K8s `GetTaskData` API 返回合并后的变量（含变量集）
- **改造** Agent `GetPlanTask` 快照重建时 fallback 查询 varset_variables 表
- **新增** 任务执行日志打印已加载的 terraform/environment 变量 key（INFO 级别，不打印 value）

#### 前端

- **新增** Variable Sets 列表页（`/variable-sets`），支持创建、编辑、删除变量集
- **新增** Variable Set 详情页（`/variable-sets/:varsetId`），Variables Tab + Assignments Tab
- **新增** Sidebar 导航 "Variable Sets" 项（Workspaces 下方），带 VARIABLE_SETS 权限检查
- **新增** Assignment 选择器：下拉选择 Project/Workspace（从 API 加载列表）
- **改进** Workspace 变量页面增加 "Variable Set Variables" 区域，展示来自变量集的变量
- **改进** 被覆盖的变量显示 "Overridden {key}" 超链接，点击跳转到覆盖方变量
- **改进** 创建变量集后自动跳转到详情页，方便立即添加变量和分配

### Tests

- **新增** 9 个集成测试，覆盖版本控制、键唯一性、sensitive 约束、全局解析、覆盖优先级、版本传播
- **新增** TestMain 自动管理测试库生命周期（创建 iac_platform_test -> 初始化 schema -> 执行测试 -> 清理）

### AI Plan Summary 实时日志流

#### 后端

- **新增** `AIAgentLoop` observer callback 机制，支持 thinking/tool_call/tool_result/output/retry 五类中间事件实时推送
- **新增** `AISummaryService` 接入 WebSocket stream，AI 分析过程实时广播到前端（`post_plan_summary`/`post_apply_summary` 阶段）
- **新增** `process_log` 字段持久化 AI 分析过程日志，支持刷新后回看
- **新增** process log 行格式统一为 `[timestamp] [LEVEL] [Step N] message`，与 Terraform 日志风格一致
- **修复** `completed` WebSocket 消息在 AI summary 完成前发送，导致前端提前显示 Completed 且断连后不重连
- **修复** stream 生命周期：使用 WaitGroup 延迟 stream 关闭和 `completed` 广播，等待 AI summary 完成
- **修复** thinking content 截断使用 rune 安全截取，避免中文 UTF-8 字符被截断
- **新增** Agent/K8s 模式：`RawAgentCCHandler.handleTaskCompleted` 服务端触发 AI summary（agent 端无 DB）

#### 前端

- **新增** `TerraformOutputViewer`（实时）和 `StageLogViewer`（历史）同步支持 Plan Summary / Apply Summary 阶段 tab
- **新增** summary 阶段默认折叠，不自动跳转
- **改进** `StageLogViewer` 渲染风格统一为逐行 div + 行号 + stage marker 特殊样式，与 `TerraformOutputViewer` 一致
- **新增** `StageLogViewer` 从 summary API 获取 `process_log` 拼接到日志末尾，支持历史回看 AI 分析过程
- **新增** `TerraformOutputViewer` 空 stream 检测：连接 3 秒无数据自动降级到 HTTP 历史日志（解决刷新后空白问题）

### Database

- **新字段** `ai_plan_summaries.process_log text` -- AI Plan 分析过程日志
- **新字段** `ai_apply_summaries.process_log text` -- AI Apply 分析过程日志
- **Migration**: `manifests/migrations/add_process_log_to_ai_summaries.sql`
- **Seed SQL**: `manifests/db/init_seed_data.sql` 同步更新建表语句

---

### Variable Snapshot (执行路径改造)

- **新表** `variable_snapshots` -- 变量快照引用（vsnap_id, variable_id, version, source_type），替代旧 workspace_tasks.snapshot_variables JSONB
- **新增** POST `/workspaces/:id/variable-snapshots` API，创建变量快照，返回 vsnap_id
- **新增** DELETE `/workspaces/:id/variable-snapshots/:vsnap_id` API，删除快照
- **改造** 任务创建流程：强制先创建 variable snapshot 再创建 task（保证变量一致性）
- **改造** Local 模式执行：ExecutePlan/ExecuteApply 开始时通过 LoadSnapshot 加载快照变量到 DataAccessor 缓存
- **改造** Agent/K8s 模式：GetTaskData 和 GetPlanTask 从 snapshot 表加载变量返回给 agent
- **改造** DataAccessor 接口新增 LoadSnapshot 方法，LocalDataAccessor 实现缓存机制
- **删除** workspace_tasks.snapshot_variables JSONB 列，改为 variable_snapshot_id 关联
- **测试** 7 个集成测试覆盖 snapshot 创建/加载/隔离/缓存/null/删除

### Variable Set Database

- **新表** `variable_sets` -- 变量集元数据（varset_id, name, scope, is_deleted）
- **新表** `varset_variables` -- 变量集内变量（variable_id, key, value, version, sensitive, 加密存储）
- **新表** `varset_assignments` -- 软连接分配（scope_type, project_id/workspace_id, attached_at）
- **索引** partial unique `(varset_id, key, version) WHERE is_deleted=false`、partial unique `(name) WHERE is_deleted=false`
- **约束** CHECK scope_type 互斥、FK CASCADE 到 variable_sets/projects/workspaces
