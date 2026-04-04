## v0.5.0

Variable Set + Variable Snapshot -- 组织级变量集管理，支持 Global/Specific 作用域，软连接 attach 到 Workspace，优先级解析，版本控制，独立变量快照机制，全模式（Local/Agent/K8s）Terraform 执行路径集成。

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
- **约束** 同一变量集内及同一 workspace 内 key 唯一（不区分 variable_type）

#### 变量优先级解析

- **新增** `VariableResolutionService`，支持 Display（前端展示）和 Resolve（执行器）双模式
- **优先级** Workspace 变量 > Workspace 级 VarSet > Project 级 VarSet > Global VarSet
- **同层级** 后 attach 的变量集覆盖先 attach 的；Global 按创建时间排序

#### Variable Snapshot（执行路径改造）

- **新增** `variable_snapshots` 表，存储变量快照引用（vsnap_id, variable_id, version, source_type），替代旧 workspace_tasks.snapshot_variables JSONB
- **新增** POST `/workspaces/:id/variable-snapshots` API，创建变量快照，返回 vsnap_id（无变量时返回 null）
- **新增** DELETE `/workspaces/:id/variable-snapshots/:vsnap_id` API，删除快照
- **改造** 任务创建流程：强制先创建 variable snapshot 再创建 task（保证变量一致性）
- **改造** Local 模式：ExecutePlan/ExecuteApply 开始时通过 LoadSnapshot 加载快照变量到 DataAccessor 缓存
- **改造** Agent/K8s 模式：GetTaskData 和 GetPlanTask 从 snapshot 表加载变量返回给 agent
- **改造** DataAccessor 接口新增 LoadSnapshot 方法，LocalDataAccessor 实现缓存机制
- **删除** workspace_tasks.snapshot_variables JSONB 列，改为 variable_snapshot_id 关联
- **优化** LoadFromSnapshot 按 source_type 批量查询（2 次查询代替 N 次），未找到引用时打 WARN 日志

#### Terraform 执行路径集成

- **改造** `LocalDataAccessor.GetWorkspaceVariables()` 优先从 snapshot 缓存读取，fallback 到实时解析
- **改造** TF_LOG 读取从直接 DB 查询改为通过 DataAccessor（使用 snapshot 缓存）
- **新增** 任务执行日志打印已加载的 terraform/environment 变量 key（INFO 级别，不打印 value）

#### 前端

- **新增** Variable Sets 列表页（`/variable-sets`），一站式创建：名称 + 作用域 + 变量 + 分配全部 inline 完成
- **新增** Variable Set 详情页（`/variable-sets/:varsetId`），直接编辑名称/描述 + 变量管理 + 分配管理
- **新增** Sidebar 导航 "Variable Sets" 项（Workspaces 下方），带 VARIABLE_SETS 权限检查
- **新增** Workspace 变量页面 "Variable Set Variables" 区域，展示来自变量集的变量及覆盖状态
- **新增** 被覆盖的变量显示 "Overridden {key}" 超链接，点击跳转到覆盖方变量
- **设计** 参考 HCP Terraform Variable Set UI：垂直布局、scope radio 带描述、project/workspace 同时选择、变量表格（KEY/VALUE/CATEGORY）

### Tests

- **新增** 16 个集成测试（9 个 Variable Set + 7 个 Variable Snapshot）
- 覆盖：版本控制、键唯一性、sensitive 约束、优先级解析、覆盖优先级、快照创建/加载/隔离/缓存/null/删除
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
- **修复** Classic View 日志重复：WebSocket 重连时清空旧行，避免服务端回放历史消息导致 stage 重复显示
- **修复** Classic View 阶段过滤：切换 stage tab 时只显示选中阶段的 marker，不再显示所有阶段的 marker
- **修复** Classic View 历史回看默认显示全部日志，自动切换 tab 仅在实时运行中生效

### AI 错误分析 Agent Loop 改造

#### 后端

- **改造** `AnalyzeErrorByTaskID` 从单次 API 调用改为 `AIAgentLoop` 多轮工具调用模式，AI 自主决定查询哪些上下文
- **新增** `QueryModuleInputsTool` -- 从 plan_json 的 `configuration.root_module.module_calls.expressions` 提取用户传入模块的原始参数值（含 plan 阶段 `(known after apply)` 字段的实际输入，如 bucket policy JSON）
- **新增** `QueryTaskResourceChangesTool` -- 查询任务的资源变更记录，支持按 apply_status 过滤（all/failed/completed）
- **复用** `QueryResourceAttributesTool` -- CMDB 资源属性搜索，错误分析场景可按需查询
- **新增** `errorAnalysisValidator` 输出验证器，确保 AI 返回合法 JSON（error_type/root_cause/solutions 必填）
- **新增** `process_log` 字段持久化 Agent Loop 过程日志（工具调用、thinking、结果），支持事后审计
- **删除** 旧的 `AnalyzeError` 单次调用方法及 `callBedrock`/`callOpenAICompatible` 直接调用路径，统一走 `AICaller` 抽象
- **效果** 以 S3 bucket policy ARN 不匹配为例：旧版只能给出"ARN 格式可能错误"的泛化建议，新版精准定位"policy 中写的 `ken-test-2026` 与实际 bucket 名 `ken-test-2026-02-190344de-0223-0404` 不匹配"

### Assessment 补偿机制

- **新增** `partial` 评估状态：L1 完成但 L2/L3 LLM 评估失败时标记为 `partial`，而非直接标记 `assessed`
- **修复** Summary Assessment：L2/L3 调用失败后资源被标记为 `assessed`，补偿任务无法捡到，导致前端显示 0/0/0
- **修复** Skill Assessment Worker：同上，L2/L3 error 后 scanner 不再重试
- **改进** `compensatePendingAssessments` 同时查询 `pending` 和 `partial` 状态的资源
- **改进** `AssessSource` 同时处理 `pending` 和 `partial` 状态的资源
- **改进** Skill Assessment Worker scanner 和 CAS 同时接受 `partial` 状态
- **修复** Resource Summary 生成：attributes 变更时先清旧 `summary_hash`，确保 AI 失败后启动补偿能捡到过期摘要
- **新增** CMDB Overview 任务队列增加 "L2/L3 待补偿" 指标，显示 `partial` 状态的资源数量

### Database

#### 新表

- `variable_sets` -- 变量集元数据（varset_id, name, scope, is_deleted）
- `varset_variables` -- 变量集内变量（variable_id, key, value, version, sensitive, 加密存储）
- `varset_assignments` -- 软连接分配（scope_type, project_id/workspace_id, attached_at）
- `variable_snapshots` -- 变量快照引用（vsnap_id, variable_id, version, variable_type, source_type）

#### 表变更

- `workspace_tasks` -- 新增 `variable_snapshot_id VARCHAR(30)`，删除 `snapshot_variables JSONB`
- `workspace_variables` -- 唯一索引去掉 variable_type（同名 key 不区分类型）

#### 新字段

- `ai_plan_summaries.process_log text` -- AI Plan 分析过程日志
- `ai_apply_summaries.process_log text` -- AI Apply 分析过程日志
- `ai_error_analyses.process_log text` -- AI 错误分析 Agent Loop 过程日志

#### Migration 文件

- `backend/migrations/add_variable_sets.sql`
- `backend/migrations/add_variable_snapshots.sql`
- `backend/migrations/add_process_log_to_ai_summaries.sql`
- `backend/migrations/add_process_log_to_ai_error_analyses.sql`
- `manifests/db/init_seed_data.sql` 同步更新
