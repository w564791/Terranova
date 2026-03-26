-- Create evaluation Skills for Layer 2 (rule) and Layer 3 (semantic)

-- Layer 2: Rule consistency evaluation prompt
INSERT INTO skills (id, name, display_name, description, layer, content, version, is_active, priority, source_type, metadata, created_by, created_at, updated_at)
VALUES (
  'skill-quality-rule-eval',
  'skill_quality_rule_evaluation',
  'Skill 质量 - 规则一致性评估',
  '对照 Skill 定义中的规则，检查 AI 输出是否违反条件逻辑和业务规则',
  'task',
  '你是一个 Skill 输出质量的规则一致性检查器。

## 你的职责
检查 AI 的最终输出 JSON 是否违反了 Skill 定义中关于**输出内容**的规则。

## 重要：检查范围限定
你只检查**输出内容本身**的规则，包括：
- 输出 JSON 的字段值是否符合规则定义的约束（如枚举值、条件触发）
- 字段之间的逻辑一致性（如 risk_level=high 时 requires_confirmation 是否为 true）
- 数值计算规则（如 score 计算逻辑）
- 输出格式规范（如 recommended_actions 的结构要求）

你**不检查**以下内容（这些是 Agent 流程层面的，不是输出内容的问题）：
- 是否调用了特定工具（如 query_cmdb_dependencies）
- 阶段识别（stage 字段由系统注入，不是 AI 输出的一部分）
- 工具调用顺序或次数
- Agent loop 的执行流程

## Skill 定义（仅规则部分）
{skill_rules_section}

## 本次调用输入
{input}

## 本次调用输出
{output}

---

仅检查输出内容的规则一致性。按以下 JSON 格式输出：

{
  "verdict": "pass | warn | fail",
  "score": 0-100的整数,
  "rule_violations": [
    {
      "rule": "规则简短名称",
      "detail": "具体违反内容，引用输出中的实际值"
    }
  ],
  "assessment_confidence": "high | medium | low"
}

评分参考：
- 90-100：输出完全符合所有内容规则
- 70-89 ：轻微偏差（如枚举值可接受但非最优、格式小瑕疵）
- 50-69 ：存在规则违反，但核心字段正确
- 30-49 ：多处规则违反，影响输出可用性
- 0-29  ：关键规则违反（如 risk_level 与实际风险严重不匹配）

注意：
- 如果规则涉及工具调用或流程步骤，跳过该规则（不算违反）
- rule_violations 必须引用输出中的具体值
- 只输出 JSON，不要有额外文字',
  '1.0.0', true, 0, 'manual',
  '{"tags": ["quality", "evaluation", "rule"], "description": "Layer 2 规则一致性评估 prompt"}',
  'system', NOW(), NOW()
) ON CONFLICT (name) DO UPDATE SET content = EXCLUDED.content, updated_at = NOW();

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
) ON CONFLICT (name) DO UPDATE SET content = EXCLUDED.content, updated_at = NOW();
