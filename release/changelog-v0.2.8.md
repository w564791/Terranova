## v0.2.8

Skill 组装引擎元规则优化、Skill 内容去重、CMDB 两阶段查询流程修复。

### Features

- **Skill 组装元规则声明** — 新增 `MetaRulesConfig` 配置，启用后在组装 Prompt 最前面注入优先级层级、冲突解决原则、已加载 Skill 清单表格，让模型感知各层 Skill 的权重关系 (`skill_assembler.go`, `models/skill.go`)
- **Skill 分段标记** — 启用元规则后，每个 Skill 前添加层级标题（`[Foundation Layer]` / `[Domain Layer - Best Practice]` / `[Domain Layer - Module Constraints]` / `[Task Layer]`）和版本标识，模型可清楚区分内容来源
- **SourceType 排序** — `sortSkills` 同层级内按 SourceType 分组排序（manual → hybrid → module_auto），确保 Best Practice 排在 Module Constraints 之前

### Bug Fixes

- **Policy 字段整体占位符化** — 修改 `placeholder_standard` Skill 新增"复合对象字段规则"，禁止对 `policy`/`assume_role_policy` 等结构化字段使用整体占位符（`{{PLACEHOLDER:policy}}`），要求占位符下沉到叶子节点
- **CMDB 查询阶段 Skill 选择错误** — `executeParallel` 中移除错误的阶段一 Skill AI 选择（选出 policy 类 Skill 而非 CMDB Skill），CMDB 查询阶段改为使用 `cmdb_query_plan` AI Config 的固定 SkillComposition (`ai_cmdb_skill_service.go`)
- **CMDB 两阶段流程拆分** — 将 `assessCMDBWithQueryPlan` 拆分为独立的两步：`shouldUseCMDBByAI`（读取 `cmdb_need_assessment` AI Config ID=14）+ `cmdbService.parseQueryPlan`（读取 `cmdb_query_plan` AI Config ID=8），均从 DB 读取 SkillComposition，不再硬编码
- **SSE 进度展示修正** — 步骤 3 "CMDB查询+Skill选择" 现在正确显示 CMDB 查询阶段使用的 Skill（来自 `cmdb_query_plan` 配置），不再错误展示资源生成阶段的 Domain Skill
- **阶段二 Skill 选择缺失** — CMDB 查询完成后新增 `selectDomainSkillsByAI(_, "second")` 调用，确保资源生成阶段有正确的 Domain Skill 选择

### Skill Changes

- **placeholder_standard** — 新增复合对象字段规则段落，包含正确/禁止做法示例和叶子节点占位符命名规范
- **aws_policy_core_principles** — 新增"Terraform 场景：避免循环依赖"章节，说明 `Principal: "*"` + `aws:PrincipalArn` Condition 模式
- **aws_s3_policy_patterns** — 模式 9 从 ~130 行详细内容精简为 3 行引用，指向 `aws_policy_core_principles` 的循环依赖章节（去重）
- **resource_generation_workflow** — 移除步骤 4.5（Policy 处理指导已内化到 `placeholder_standard` 的复合对象规则中），新增 `domain_tags` frontmatter
- **种子 SQL 同步** — `init_seed_data.sql` 中同步更新以上 4 个 Skill 的 content 列

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.2.7...v0.2.8
