## v0.6.1

Plan 变更展示优化与 after_unknown 智能分析 -- 修复 Terraform plan 中 "Known after apply" 字段的展示问题，新增 read 资源详情展示和 plan changes 下载，并通过代码变更分析让 AI 能准确判断 unknown 字段是否会发生实质变更。

### Features

#### after_unknown 代码变更分析

- **新增** `query_resource_code_diff` AI tool，通过 state version -> task snapshot -> 版本号链路，对比上次 apply 时的代码与当前代码的字段级精准 diff
- **新增** AI summary 分析流程步骤 2a：遇到 after_unknown 字段时自动调用代码 diff 分析
- **新增** skill prompt 十二A节：完整的 after_unknown 代码变更分析规则，含触发条件、分析方法、结论分类和示例
- **优化** `extractPlanChanges` 保留带 `action_reason` 的 read 资源，AI 可看到完整依赖链上下文

#### Plan 变更展示优化

- **修复** UPDATE/REPLACE 资源中 after 为 null 的字段显示为红色删除，现正确显示为 "Known after apply" 对比
- **新增** read（data source）资源的展示支持：灰色边框、READ badge、filter 选项、展开查看属性
- **新增** Plan changes 下载按钮，位于 plan stage 展开区右上角，下载 resource-changes JSON 数据
