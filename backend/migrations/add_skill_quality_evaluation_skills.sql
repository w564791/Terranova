-- Create evaluation Skills for Layer 2 (rule) and Layer 3 (semantic)

-- Layer 2: Rule consistency evaluation prompt
INSERT INTO skills (id, name, display_name, description, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
  'skill-quality-rule-eval',
  'skill_quality_rule_evaluation',
  'Skill 质量 - 规则一致性评估',
  '对照 Skill 定义中的规则，检查 AI 输出是否违反条件逻辑和业务规则',
  'task',
  '你是一个 Skill 规则一致性检查器。你的工作是检查输出是否违反了 skill 定义中的条件逻辑和业务规则。

## Skill 定义（仅规则部分）
{skill_rules_section}

## 本次调用输入
{input}

## 本次调用输出
{output}

---

仅检查规则一致性，不评价语义质量。按以下 JSON 格式输出：

{
  "verdict": "pass | warn | fail",
  "score": 0-100的整数,
  "rule_violations": [
    {
      "rule": "skill 中对应规则的简短名称",
      "detail": "具体说明哪里违反了，必须引用实际输出中的值"
    }
  ],
  "assessment_confidence": "high | medium | low"
}

评分参考：
- 90-100：所有条件逻辑和规则完全符合
- 70-89 ：轻微规则偏差，不影响核心逻辑
- 50-69 ：存在规则违反，但不影响安全性
- 0-49  ：关键规则违反（如安全相关条件未触发）

注意：
- rule_violations 必须引用 skill 中的具体规则条目，禁止泛化描述
- 如果 output 不是合法 JSON，verdict 直接为 fail，score 为 0
- 只输出 JSON，不要有任何额外文字',
  '1.0.0', true, 0, 'manual',
  '{"tags": ["quality", "evaluation", "rule"], "description": "Layer 2 规则一致性评估 prompt"}',
  'system', NOW(), NOW()
) ON CONFLICT (name) DO NOTHING;

-- Layer 3: Semantic quality evaluation prompt
INSERT INTO skills (id, name, display_name, description, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
  'skill-quality-semantic-eval',
  'skill_quality_semantic_evaluation',
  'Skill 质量 - 语义质量评估',
  '评估 AI 输出的表述质量、信息量和用户可读性',
  'task',
  '你是一个 Skill 语义质量评估器。你只需关注输出的语义质量，不要检查 JSON 结构合规性或规则一致性（这些由其他评估层负责）。

## Skill 定义
{skill_md}

## 本次调用输入
{input}

## 本次调用输出
{output}

---

仅评估语义质量，不检查结构和规则。按以下 JSON 格式输出：

{
  "verdict": "pass | warn | fail",
  "score": 0-100的整数,
  "quality_issues": [
    {
      "field": "有问题的字段路径",
      "issue": "具体问题描述，禁止使用信息不足等模糊表述",
      "severity": "high | medium | low"
    }
  ],
  "highlights": ["做得好的地方，1-3条"],
  "assessment_confidence": "high | medium | low"
}

评分参考：
- 90-100：表述精准具体、信息量充分、用户可直接理解
- 70-89 ：基本清晰，有轻微含糊或冗余
- 50-69 ：多处表述模糊或信息量不足
- 0-49  ：严重的语义问题，用户无法从输出中获取有效信息

注意：
- quality_issues 每条必须基于实际输出内容，禁止假设
- 引用实际输出中的具体文本来说明问题
- 只输出 JSON，不要有任何额外文字',
  '1.0.0', true, 0, 'manual',
  '{"tags": ["quality", "evaluation", "semantic"], "description": "Layer 3 语义质量评估 prompt"}',
  'system', NOW(), NOW()
) ON CONFLICT (name) DO NOTHING;

-- Update skill_quality_assessment AI Config with skill_composition
UPDATE ai_configs
SET mode = 'prompt',
    skill_composition = '{
      "task_skill": "",
      "foundation_skills": [],
      "domain_skills": [],
      "auto_load_module_skill": false
    }'::jsonb
WHERE capabilities @> '["skill_quality_assessment"]'::jsonb;
