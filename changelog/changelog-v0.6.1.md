## v0.6.1

AI 风险评估准确性增强 -- 新增 R2 确定性兜底规则修正 AI 漏标 service_disruption 的问题，强化 Skill Prompt 的 CMDB 查询规则，优化 Plan Summary 前端布局。

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

### Enhancements

#### Plan Summary 前端布局优化

- **优化** 变更概述与影响分析合并展示：概述直接显示，影响分析紧跟其下
- **新增** 影响分析色条：左边框和背景色随风险等级联动（critical 红/high 橙/medium 黄/low 绿）
- **优化** Risk Score 展示：内联分数 + 进度条，默认折叠，展开显示结构化 breakdown 表格
- **优化** Decision Card 状态区分：已确认（绿色边框+背景）、已取消（灰色边框+背景）
