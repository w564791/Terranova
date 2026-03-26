## v0.4.10

Skill 质量评估体系 Phase 1-4：三层自动评估引擎 + Dashboard 可视化 + 用户反馈闭环。

### Features

#### 三层评估引擎

- **Layer 1 — Schema 校验** — 基于 JSON Schema 验证 AI 输出结构完整性，支持 `plan_summary` / `apply_summary` / `module_skill_generation` 三种 capability；Schema 从 skill metadata 自动加载 (`skill_schema_validator.go`)
- **Layer 2 — 规则一致性 (LLM)** — 独立 AI Config `skill_rule_evaluation`，检查输出是否违反 Skill 定义中的业务规则；采样决策控制进入比例 (`skill_llm_evaluator.go`, `skill_assessment_sampler.go`)
- **Layer 3 — 语义质量 (LLM)** — 独立 AI Config `skill_semantic_evaluation`，评估输出的表述质量、信息量和可读性 (`skill_llm_evaluator.go`)
- **评估 Worker** — 后台 goroutine 池异步处理待评估记录，定期扫描 pending 状态的 usage log (`skill_assessment_worker.go`)
- **Skill 用量采集** — AI 调用时自动记录 input/output snapshot、skill content hash、执行耗时；覆盖 plan_summary、apply_summary、module_skill_generation、AI CMDB (`ai_summary_service.go`, `ai_cmdb_skill_service.go`, `module_skill_ai_service.go`)
- **用户行为上报** — 前端报告 accepted / aborted 操作，写入 usage log 关联评估结果 (`skill_controller.go`)

#### Dashboard & 可视化

- **质量监控总览** — 通过率、平均分、告警趋势、capability 切换；URL 参数同步 `?cap=` 支持页面刷新保留状态 (`SkillQualityDashboard.tsx`)
- **Skill 详情页** — 版本时间线、违规 Top 排行（L2/L3 分层展示）、反馈矩阵 (`SkillDetailTab.tsx`)
- **版本对比 API** — 按 skill_content_hash 聚合，对比不同版本的评估指标 (`skill_assessment_service.go`)
- **服务端分页** — failure 和 assessment 表格支持服务端分页 (`skill_assessment_controller.go`)

#### 用户反馈

- **plan_summary 决策反馈** — 用户 confirm/abort 后弹出评分卡片 (`ExecuteSummary.tsx`, `FeedbackBanner.tsx`)
- **module_skill_generation 反馈** — 表单生成完成后收集反馈 (`AIConfigGenerator.tsx`)
- **全局 FeedbackBanner** — 持久化 dismiss 状态到 localStorage，单次只展示一张卡片，按 task 粒度限定范围 (`FeedbackBanner.tsx`, `App.tsx`)

#### Golden Set

- **黄金测试集表** — 存储标注数据用于评估质量基准，支持按 skill + layer 查询 (`skill_golden_sets`)

### Bug Fixes

- **V2/V3 plan_summary 格式兼容** — `RequiredOneOf` 字段支持 (`skill_schema_validator.go`)
- **modify_rate 替换为 abort_rate** — 前端无 modify 入口，指标改为 abort_rate (`skill_assessment_service.go`)
- **L2/L3 版本表显示 avg score** — 替代原 pass rate，更准确反映质量 (`skill_assessment_service.go`)
- **React Hooks 顺序错误** — useState 移至 early return 前，修复 error #310 (`SkillQualityDashboard.tsx`)
- **Segmented 时间范围切换闪烁** — 改用 local state 消除 flicker (`SkillQualityDashboard.tsx`)
- **feedback 通知时序** — 同步显示通知，避免组件 unmount 导致通知丢失 (`ExecuteSummary.tsx`)
- **user_id=system 权限** — 修复 system 用户在 pending-feedback 和 action report 中的权限检查 (`skill_controller.go`)

### Security

- **用户所有权校验** — API 端点增加 ownership check，防止越权访问评估数据 (`skill_controller.go`)
- **敏感数据脱敏** — assessment_raw_output 等字段在非 admin 响应中过滤 (`skill_assessment_service.go`)

### Database Migrations

```sql
-- skill_usage_logs 扩展 (add_skill_assessment.sql)
ALTER TABLE skill_usage_logs
    ADD COLUMN input_snapshot JSONB,
    ADD COLUMN output_snapshot JSONB,
    ADD COLUMN skill_content_hash VARCHAR(64),
    ADD COLUMN skill_content_snapshot TEXT,
    ADD COLUMN user_action VARCHAR(16),
    ADD COLUMN user_modification_diff TEXT,
    ADD COLUMN latency_ms INTEGER,
    ADD COLUMN assessment_status VARCHAR(16) DEFAULT 'pending';

-- 新表: skill_assessment_results (add_skill_assessment.sql)
CREATE TABLE skill_assessment_results (
    id VARCHAR(36) PRIMARY KEY,
    usage_log_id VARCHAR(36) NOT NULL REFERENCES skill_usage_logs(id),
    skill_name VARCHAR(128), skill_content_hash VARCHAR(64),
    assessed_at TIMESTAMPTZ, assessment_layer VARCHAR(16),
    verdict VARCHAR(16), score SMALLINT,
    assessment_latency_ms INTEGER, schema_valid BOOLEAN,
    missing_fields TEXT[], invalid_enum_fields TEXT[],
    rule_violations JSONB, quality_issues JSONB,
    assessment_confidence VARCHAR(16), assessment_model VARCHAR(64),
    assessment_raw_output TEXT
);

-- 新表: skill_golden_sets (add_golden_set_table.sql)
CREATE TABLE skill_golden_sets (
    id VARCHAR(36) PRIMARY KEY,
    skill_name VARCHAR(128), assessment_layer VARCHAR(16),
    input_snapshot JSONB, output_snapshot JSONB,
    expected_verdict VARCHAR(16),
    expected_score_min SMALLINT, expected_score_max SMALLINT,
    annotations JSONB, created_by VARCHAR(128),
    created_at TIMESTAMPTZ, is_active BOOLEAN DEFAULT true
);

-- AI Config: L2 + L3 评估 (add_skill_quality_assessment_config.sql)
INSERT INTO ai_configs (...) -- id=18 skill_rule_evaluation
INSERT INTO ai_configs (...) -- id=19 skill_semantic_evaluation

-- Skill: 评估 prompt 模板 (add_skill_quality_evaluation_skills.sql)
INSERT INTO skills (...) -- skill_quality_rule_evaluation
INSERT INTO skills (...) -- skill_quality_semantic_evaluation
```

### Documentation

- Skill 质量评估设计文档 (`docs/ai/06-skill-质量评估.md`)
- 运营指南 (`docs/ai/07-skill-质量评估-运营指南.md`)
- Dashboard mockup (`docs/ai/skill-assessment-dashboard-mockup.html`)

### Full Changelog

https://github.com/w564791/iac-platform/compare/v0.4.9...v0.4.10
