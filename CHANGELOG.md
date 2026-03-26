# Changelog

## v0.4.10 - 2026-03-26

### Skill 质量评估体系 (Phase 1-4)

完整实现 Skill 质量评估功能，覆盖三层自动评估、Dashboard 可视化和用户反馈闭环。

#### 三层评估引擎

- **Layer 1 - Schema 校验**: 基于 JSON Schema 验证 AI 输出结构完整性，支持 plan_summary / apply_summary / module_skill_generation
- **Layer 2 - 规则一致性**: LLM 评估输出是否违反 Skill 定义中的业务规则（独立 AI Config: `skill_rule_evaluation`）
- **Layer 3 - 语义质量**: LLM 评估输出的表述质量、信息量和可读性（独立 AI Config: `skill_semantic_evaluation`）
- 采样决策逻辑，支持按比例抽样进入 L2/L3 评估
- 后台 Worker 异步评估，goroutine 池控制并发

#### Dashboard & 可视化

- Skill 质量监控总览页：通过率、平均分、告警、趋势图
- Skill 详情页：版本时间线、违规 Top 排行（L2/L3 分层）、反馈矩阵
- 服务端分页、URL 参数同步、时间范围切换
- 版本对比 API

#### 用户反馈

- plan_summary 决策反馈（accepted / aborted）
- module_skill_generation 反馈
- 全局 FeedbackBanner 组件，持久化 dismiss 状态
- 反馈数据写入 skill_usage_logs，关联评估结果

#### 数据模型

- 新增表: `skill_assessment_results`（17 列 + 4 索引 + FK）
- 新增表: `skill_golden_sets`（12 列 + 1 partial index）
- 扩展表: `skill_usage_logs` 新增 8 列（input/output snapshot, content hash, user action 等）
- 新增 2 条 AI Config: id=18 (`skill_rule_evaluation`), id=19 (`skill_semantic_evaluation`)
- 新增 2 条 Skill: `skill_quality_rule_evaluation`, `skill_quality_semantic_evaluation`

#### 安全

- 用户所有权校验，敏感数据脱敏
- user_id=system 权限检查修复

#### 文档

- Skill 质量评估设计文档
- 运营指南
- Dashboard mockup

#### 其他

- 数据清理定时任务
- Schema 自动加载（从 skill metadata）
- 种子 SQL 与迁移 SQL 同步修正
