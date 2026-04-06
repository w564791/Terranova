## v0.6.1

AI 风险评估准确性增强、query_resource_code_diff 性能优化、Plan Summary 前端重构、K8s/Agent 模式 Resource ID 提取修复。

### Features

#### R2 Risk Guardrail（Go 后处理兜底）

- **新增** `inferServiceDisruption` 确定性推断：当 AI 标记了 `dependency_break` 或 `permission_scope_change`，且 CMDB `query_resource_attributes` 返回 `found=false`，且变更包含 update/delete 操作时，自动补充 `service_disruption` 风险因子
- **新增** `hasCMDBNotFound`：扫描 tool_calls 中 CMDB 查询结果
- **新增** `hasModifyAction`：检查 impact_analysis 中是否包含 update/delete 操作，纯 create 不触发
- **修复** `ToolCalls`/`ThinkingContent` 赋值时序：移至 `buildRiskScoringInput` 调用之前，确保 R2 能读到 tool_calls 数据

#### Skill Prompt 强化

- **新增** 第十三节「必须查询 CMDB 的场景」：资源引用标识符变更时，必须对变更前后的标识符分别查询 CMDB，确认是否存在
- **新增** 覆盖 Bucket Policy/Resource Policy/Trust Policy/Security Group Rule 中的引用变更场景
- **新增** known after apply 字段的代码变更分析结果同样适用引用标识符查询规则

#### query_resource_code_diff 性能优化

- **新增** `workspace_task_resource_changes.applied_code_version` 字段：apply 完成时回填 `resource_code_versions.version`
- **优化** `query_resource_code_diff` 快速路径：通过 `module_address` + `apply_status='completed'` 直接查版本号，替代 `snapshot_resource_versions` JSONB 扫描
- **保留** Fallback 路径：未回填的历史数据仍走 JSONB 扫描，向后兼容
- **新增** `backfillAppliedCodeVersion`：apply 成功和部分失败时均回填已完成资源的版本号

### Enhancements

#### Plan Summary 前端布局优化

- **优化** 变更概述与影响分析合并展示：概述直接显示，影响分析紧跟其下
- **新增** 影响分析色条：左边框和背景色随风险等级联动（critical 红/high 橙/medium 黄/low 绿）
- **优化** Risk Score 展示：内联分数 + 进度条，默认折叠，展开显示结构化 breakdown 表格
- **优化** Decision Card 状态区分：已确认（绿色边框+背景）、已取消（灰色边框+背景）

#### Apply 资源状态修正

- **修复** Apply 失败时 `applying` 状态的资源未标记为 `failed`，现在 `saveTaskFailure` 会将 `applying` 状态批量更新为 `failed`

### Bug Fixes

#### K8s/Agent 模式 Resource ID 缺失

- **修复** Agent/K8s 模式下 Apply 完成后 Resource ID 始终显示 `(not available)`：Agent 端 `db=nil` 导致 `ExtractResourceDetailsFromState` 被跳过，Server 端 `handleTaskCompleted` 未补做提取
- **新增** `RawAgentCCHandler.extractResourceIDsFromState`：Server 端收到 Agent `task_completed` 后，从最新 state version 提取 Resource ID 写入 DB 并通过 WebSocket 广播

#### 其他

- **修复** State 版本下载 401 错误：`StatePreview.tsx` 硬编码 `http://localhost:8080` 导致非本地环境请求失败，改为使用 api 客户端

### Database Migration

- `backend/migrations/add_applied_code_version.sql`
  - workspace_task_resource_changes: 新增 `applied_code_version` 字段
  - 回填脚本：从 `snapshot_resource_versions` 提取历史 completed 记录的版本号
