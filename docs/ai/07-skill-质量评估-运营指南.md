# Skill 质量评估系统 — 运营指南

> 本文档面向运营/开发团队，说明系统上线后需要持续做的运营工作。
> 技术设计见 `06-skill-质量评估.md`

---

## 一、Golden Set 标注

### 1.1 什么是 Golden Set

每个核心 Skill 维护一组"标准答案"——人工标注的 input/output 对，标注期望的评估结果（verdict + score 范围）。用于：
- 校验评估器本身是否准确（评估器的"单元测试"）
- 检测评估器漂移（prompt 变更、模型升级后是否还准）

### 1.2 标注流程

**Step 1: 挑选样本**

从 Dashboard（`/?tab=quality&subtab=detail`）的"最近评估记录"中选取有代表性的 case：
- 至少包含 pass / warn / fail 各一个
- 优先选评估置信度为 high 的记录
- 每个核心 capability 标注 3-5 个样本

**Step 2: 获取 input/output**

```sql
-- 通过 usage_log_id 获取完整数据
SELECT input_snapshot, output_snapshot, skill_content_hash
FROM skill_usage_logs
WHERE id = '<usage_log_id>';
```

**Step 3: 插入 Golden Set**

```sql
INSERT INTO skill_golden_sets (id, skill_name, assessment_layer, input_snapshot, output_snapshot, expected_verdict, expected_score_min, expected_score_max, annotations, created_by, is_active)
VALUES (
  gen_random_uuid()::text,
  'plan_summary',           -- capability 名
  'rule',                    -- 评估层：rule 或 semantic
  '<input_snapshot JSON>',
  '<output_snapshot JSON>',
  'fail',                    -- 期望 verdict
  20, 40,                    -- 期望 score 范围
  '{"reason": "工具调用缺失，依赖关系编造"}',
  'admin',
  true
);
```

**Step 4: 验证**

```sql
SELECT skill_name, assessment_layer, expected_verdict, expected_score_min, expected_score_max
FROM skill_golden_sets
WHERE is_active = true
ORDER BY skill_name, assessment_layer;
```

### 1.3 标注目标

| Capability | Layer | 建议样本数 | 覆盖场景 |
|-----------|-------|-----------|---------|
| plan_summary | rule | 3 | pass(规则全部遵守) + warn(轻微偏差) + fail(工具调用缺失) |
| plan_summary | semantic | 3 | pass(表述清晰) + warn(部分模糊) + fail(不可读) |
| form_generation | rule | 2 | pass + fail |
| form_generation | semantic | 2 | pass + fail |
| module_skill_generation | rule | 2 | pass + fail |

---

## 二、评估 Prompt 调优

### 2.1 当前问题

L2（规则评估）当前全部 fail，均分 15-35。可能原因：
- 评估标准过于严格（要求 AI 必须调用工具，但实际流程中工具调用由 agent loop 处理，不在输出中体现）
- 规则段落提取不精确（没有 rules-begin/end 标记，传入了完整 Skill 内容包含无关部分）

### 2.2 调优方法

**方法 A: 调整评估 Skill 内容**

在 AI Config 管理页（`/global/settings/ai-configs/18/edit`），找到 Task Skill `skill_quality_rule_evaluation`，编辑其内容：

调整方向：
- 降低工具调用检查的权重（很多 Skill 的输出不包含工具调用信息）
- 区分"必须遵守的强规则"和"建议遵守的弱规则"
- 调整评分标准，让 70-89 分段有更多 case 落入

**方法 B: 给 Skill 加 rules-begin/end 标记**

编辑核心 Task Skill（如 `execute_summary_workflow`）的内容，在规则段落前后加标记：

```markdown
<!-- rules-begin -->
## 规则定义
- risk_level=high 时必须触发 requires_human_confirmation
- uncertainty.level=medium 时必须包含 reason_code
...
<!-- rules-end -->

## 其他内容（示例、注释等）
...
```

这样 L2 评估只传入规则段落，减少干扰，提高准确性。

**方法 C: 更换评估模型**

在 AI Config 管理页修改 `skill_rule_evaluation` (id=18) 或 `skill_semantic_evaluation` (id=19) 的模型：
- 当前用 Haiku 4.5（快、便宜但可能不够深入）
- 可以试 Sonnet 4.5（更强的推理能力，但慢+贵）

### 2.3 验证调优效果

调优后运行几个 task，然后查：

```sql
SELECT r.assessment_layer, r.verdict, r.score, r.assessment_confidence
FROM skill_assessment_results r
WHERE r.assessed_at > NOW() - INTERVAL '1 hour'
  AND r.assessment_layer IN ('rule', 'semantic')
ORDER BY r.assessed_at DESC;
```

如果 L2 score 分布从 15-35 提升到 50-80，说明调优有效。

---

## 三、日常监控

### 3.1 每日检查

打开 Dashboard（`/?tab=quality`），关注：
- **告警摘要**：有没有新增 P1/P2 告警
- **差评率**：是否突然上升
- **L2/L3 均分趋势**：是否在下降

### 3.2 每周检查

切到 Skill 详情页（`/?tab=quality&subtab=detail`），逐个 capability 查看：
- **版本趋势**：content_hash 是否变更，新版本质量如何
- **高频违规**：是否有反复出现的同类问题
- **盲区检测**：评估 pass 但用户差评的比例

### 3.3 关键 SQL 查询

```sql
-- 近 7 天各层评估分布
SELECT assessment_layer, verdict, count(*), round(avg(score),1)
FROM skill_assessment_results
WHERE assessed_at > NOW() - INTERVAL '7 days'
GROUP BY assessment_layer, verdict
ORDER BY assessment_layer, verdict;

-- 用户反馈与评估一致性
SELECT r.verdict, l.user_feedback, count(*)
FROM skill_assessment_results r
JOIN skill_usage_logs l ON l.id = r.usage_log_id
WHERE l.user_feedback IS NOT NULL AND r.assessment_layer = 'schema'
GROUP BY r.verdict, l.user_feedback;

-- 评估 Token 成本估算（通过耗时推算）
SELECT assessment_layer,
       count(*) as evals,
       round(avg(assessment_latency_ms)) as avg_ms,
       round(sum(assessment_latency_ms) / 1000.0 / 60, 1) as total_minutes
FROM skill_assessment_results
WHERE assessment_layer IN ('rule', 'semantic')
  AND assessed_at > NOW() - INTERVAL '7 days'
GROUP BY assessment_layer;
```

---

## 四、数据保留

系统自动清理过期数据（每日执行）：

| 数据 | 保留周期 | 到期后 |
|------|----------|--------|
| input/output snapshot | 90 天 | 字段清空，元数据保留 |
| assessment_raw_output | 30 天 | 字段清空，结构化结果保留 |
| user_modification_diff | 90 天 | 字段清空 |
| skill_content_snapshot | 永久 | 每个 hash 仅一条 |
| assessment 结构化结果 | 永久 | 用于趋势分析 |

---

## 五、后续开发路线

### Phase 4 剩余（需要 Golden Set 数据后）

- [ ] **评估器漂移告警**：每周跑 Golden Set，verdict 一致率 < 80% 时告警
- [ ] **版本对比 UI**：在 Dashboard 加版本对比 tab（API 已就绪，缺前端页面）

### Phase 5（持续迭代）

- [ ] **动态采样率**：质量稳定（L2 > 80 分）的 Skill 自动降采样，质量差的升采样
- [ ] **Skill 发布门禁**：新版本前 20 次评估均分低于阈值时阻断激活
- [ ] **评估 Prompt 自动调优**：基于 Golden Set 偏差自动建议 prompt 修改

### 前提条件

| 功能 | 前提 |
|------|------|
| 评估器漂移告警 | 至少标注 10 个 Golden Set 样本 |
| 动态采样率 | 需要 30 天以上的评估数据积累 |
| Skill 发布门禁 | 需要确认评分标准稳定（L2 调优完成后） |
| 版本对比 UI | 至少 1 个 capability 有 2 个以上 content_hash |
