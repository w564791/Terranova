# Skill 质量监控系统 — 完整设计方案

## 一、系统全景

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Skill 调用链路                                 │
│                                                                     │
│  业务触发 → Skill 执行 → 输出落库 → [采样决策] → 分层评估 → 评估落库  │
│                              ↑                       ↓              │
│                         用户反馈信号            Dashboard + 告警     │
│                                                      ↓              │
│                                                Golden Set 校准      │
└─────────────────────────────────────────────────────────────────────┘
```

核心思路：

1. **不改变现有调用链路**，在落库后异步触发评估
2. **分层评估**：结构校验（纯代码）→ 规则一致性 → 语义质量（LLM），逐层递进、独立打分
3. **多信号融合**：LLM-as-Judge + 用户反馈 + Golden Set 校准，避免单一评估手段的盲区
4. Skill 自身定义即评估标准，不需要为每个 skill 手写断言
5. **评估对象为 Task Skill**：一次 Skill 执行可能组合多个 Skill（Foundation + Domain + Task），评估以 Task Skill 为准——它定义了输出结构和业务规则，是评估标准的来源。`skill_assessment_results.skill_name` 记录的是 Task Skill 的名称

---

## 二、数据库设计

### 2.1 现有表扩展（skill_usage_logs）

在现有 `skill_usage_logs` 结构上，补充以下字段：

```sql
ALTER TABLE skill_usage_logs ADD COLUMN IF NOT EXISTS
  input_snapshot         JSONB,          -- 调用时的完整输入
  output_snapshot        JSONB,          -- 完整输出 JSON
  skill_content_hash     VARCHAR(64),    -- skill content 的 SHA256（用于版本追溯）
  skill_content_snapshot TEXT,           -- skill content 完整快照（仅 hash 首次出现时写入）
  user_action            VARCHAR(16),    -- accepted | modified | aborted（用户对输出的操作）
  user_modification_diff TEXT,           -- 用户修改的 diff（user_action=modified 时记录）
  latency_ms             INTEGER,        -- 调用耗时
  assessment_status      VARCHAR(16)     -- pending | assessed
                         DEFAULT 'pending';
```

> **为什么用 content_hash 而不是 version 字段？** 现有 `Skill` model 的 `Version` 是手动维护的普通字段，没有发布流程保障。用 content 的 SHA256 hash 做版本标识：
> - 自动追踪每次内容变更，无需依赖人工打版本
> - 避免"改了内容忘记改版本号"导致评估失真
> - 需要回溯时，从历史记录中按 hash 聚合即可
>
> **content_snapshot 去重写入策略**：`skill_content_snapshot` 仅在该 `skill_content_hash` 首次出现时写入完整 Skill 内容，后续相同 hash 的记录该字段留空。这样既保留了回溯到任意历史版本内容的能力，又避免了重复存储大文本的开销。查询时按 `(skill_name, skill_content_hash)` 找到最早的非空记录即可。

### 2.2 新增评估结果表（skill_assessment_results）

```sql
CREATE TABLE skill_assessment_results (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  usage_log_id        UUID REFERENCES skill_usage_logs(id),
  skill_name          VARCHAR(128)   NOT NULL,
  skill_content_hash  VARCHAR(64)    NOT NULL,
  assessed_at         TIMESTAMPTZ    DEFAULT NOW(),

  -- 评估层级标识
  assessment_layer    VARCHAR(16)    NOT NULL,  -- schema | rule | semantic

  -- 核心评估结果
  verdict             VARCHAR(16)    NOT NULL,  -- pass | warn | fail
  score               SMALLINT       NOT NULL,  -- 0-100
  assessment_latency_ms INTEGER,                -- 评估本身耗时

  -- Layer 1: 结构合规（schema 层）
  schema_valid        BOOLEAN,
  missing_fields      TEXT[],                   -- 缺失的必填字段列表
  invalid_enum_fields TEXT[],                   -- 枚举值非法的字段列表

  -- Layer 2: 逻辑一致性（rule 层）
  rule_violations     JSONB,
  -- 示例：
  -- [
  --   { "rule": "requires_human_confirmation触发条件", "detail": "risk_level=high 但未触发确认" },
  --   { "rule": "uncertainty.reason_code", "detail": "level=medium 但缺少 reason_code" }
  -- ]

  -- Layer 3: 语义质量（semantic 层）
  quality_issues      JSONB,
  -- 示例：
  -- [
  --   { "field": "decision_hints.title", "issue": "使用了模糊指代，未包含具体资源标识", "severity": "high" },
  --   { "field": "risk_highlights", "issue": "内容与 title 重复，信息量低", "severity": "medium" }
  -- ]

  -- 评估置信度
  assessment_confidence VARCHAR(16),   -- high | medium | low
  assessment_model    VARCHAR(64),     -- 使用的评估模型（如 haiku / sonnet）
  assessment_raw_output TEXT           -- 评估器的完整原始输出（用于 debug）
);

-- 查询索引
CREATE INDEX idx_assessment_skill_hash ON skill_assessment_results (skill_name, skill_content_hash);
CREATE INDEX idx_assessment_usage_layer ON skill_assessment_results (usage_log_id, assessment_layer);  -- 延迟补评查询
CREATE INDEX idx_assessment_verdict ON skill_assessment_results (verdict);
CREATE INDEX idx_assessment_at ON skill_assessment_results (assessed_at DESC);
```

### 2.3 Golden Set 表（skill_golden_sets）

```sql
CREATE TABLE skill_golden_sets (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  skill_name          VARCHAR(128)   NOT NULL,
  assessment_layer    VARCHAR(16)    NOT NULL,  -- rule | semantic（Layer 1 是纯代码校验，不需要 Golden Set）
  input_snapshot      JSONB          NOT NULL,  -- 标注的输入
  output_snapshot     JSONB          NOT NULL,  -- 标注的输出
  expected_verdict    VARCHAR(16)    NOT NULL,  -- pass | warn | fail
  expected_score_min  SMALLINT       NOT NULL,  -- 期望分数下限
  expected_score_max  SMALLINT       NOT NULL,  -- 期望分数上限
  annotations         JSONB,                    -- 人工标注的具体问题说明
  created_by          VARCHAR(128),
  created_at          TIMESTAMPTZ    DEFAULT NOW(),
  is_active           BOOLEAN        DEFAULT true
);

CREATE INDEX idx_golden_skill_layer ON skill_golden_sets (skill_name, assessment_layer) WHERE is_active = true;
```

> **Golden Set 的作用**：每个 Skill 维护 3-5 个人工标注的 input/output 对，**按 Layer 分别标注**（同一个 input/output 可以有 rule 层和 semantic 层两条标注记录）。定期用评估器跑这些数据，对比评估结果和人工标注。如果偏差大，说明评估 prompt 或模型需要调优。这是整个评估系统的可信度基准。

---

## 三、分层评估架构

不同于单一 LLM 评估，采用三层流水线，每层独立打分、独立存储、独立告警：

```
┌──────────────────────────────────────────────────────────────┐
│                      分层评估流水线                             │
│                                                              │
│  三层独立评估，互不门控。Layer 1 失败不阻止 Layer 2/3 执行。    │
│  是否执行某层由采样策略决定（见第四章）。                        │
│                                                              │
│  Layer 1: Schema 校验                                        │
│    方式: 纯代码校验（JSON Schema / Go struct 校验）              │
│    成本: 零                                                   │
│    覆盖: 100%（全量，每次必跑）                                 │
│    检查: 必填字段、枚举合法性、类型匹配、JSON 格式               │
│                                                              │
│  Layer 2: 规则一致性校验                                       │
│    方式: 轻量 LLM（Haiku）或可编程规则                          │
│    成本: 低                                                   │
│    覆盖: 20-50%（按采样策略）                                   │
│    检查: 条件触发逻辑、字段间依赖关系、业务规则合规               │
│                                                              │
│  Layer 3: 语义质量评估                                        │
│    方式: LLM-as-Judge（Sonnet/Haiku）                         │
│    成本: 中                                                   │
│    覆盖: 5-15%（低采样率）                                     │
│    检查: 表述质量、信息量、具体性、用户可读性                    │
└──────────────────────────────────────────────────────────────┘
```

**为什么要解耦？**

- **独立迭代**：schema 规则变了不影响语义评估，反之亦然
- **成本精确控制**：Layer 1 零成本全量跑，Layer 3 可以大幅降采样
- **问题定位更清晰**：结构错误和语义问题不再混在一个分数里
- **避免互相干扰**：结构完美但语义差的 case，不会因为 schema pass 拉高总分

---

## 四、采样策略

### 4.1 采样决策树

```
┌──────────────────────────────────────────────────────────────┐
│                     采样决策树                                 │
│                                                              │
│  Layer 1 (Schema): 全量，100%，无需采样                       │
│                                                              │
│  Layer 2 & 3 共用以下决策树:                                   │
│                                                              │
│  skill content_hash 首次出现？                                │
│    YES → 前 20 次全量评估（Layer 2 + 3）                      │
│    NO  ↓                                                     │
│                                                              │
│  Layer 1 校验失败（schema_valid=false）？                      │
│    YES → 必评估 Layer 2（100%），Layer 3 按正常采样率          │
│    NO  ↓                                                     │
│                                                              │
│  即时信号：user_action = aborted？                            │
│    YES → 必评估 Layer 2 + 3（100%）                          │
│    NO  ↓                                                     │
│                                                              │
│  即时信号：user_action = modified？                            │
│    YES → Layer 2 按正常采样率，必评估 Layer 3（100%）          │
│    NO  ↓                                                     │
│                                                              │
│  按 skill 滚动采样率:                                         │
│    Layer 2: 默认 20%，高风险 skill 50%                        │
│    Layer 3: 默认 5%，高风险 skill 15%                         │
└──────────────────────────────────────────────────────────────┘
```

### 4.2 用户信号接入（双时机触发）

用户信号分两类，触发时机不同：

**即时信号**（Skill 执行完成时可获得，纳入首次采样决策）：

| 信号 | 触发行为 |
|------|----------|
| `user_action = aborted` | 强制进入 Layer 2 + 3 评估 |
| `user_action = modified` | 记录修改 diff，强制进入 Layer 3 评估 |
| `user_action = accepted` | 按正常采样率 |

**延迟信号**（用户事后评分，异步补评）：

| 信号 | 触发行为 |
|------|----------|
| `user_feedback <= 2`（差评） | 检查该记录是否已完成 Layer 2+3 评估，未评估则补评 |
| `user_feedback >= 4`（好评） | 不触发额外评估 |

> **为什么要分两个时机？** `user_action` 是执行时的即时行为，评估触发时已可获得。`user_feedback` 是事后评分，可能在评估完成后数小时才到达。如果只依赖 `user_feedback` 做触发条件，评估时该字段大概率还不存在，条件永远不会命中。

**关联分析**：评估 pass 但用户给差评的 case 需要重点关注——说明评估标准存在盲区，应反向优化评估 prompt。

---

## 五、评估器实现

### 5.1 Layer 1: Schema 校验器（纯代码）

```go
// SkillSchemaValidator 纯代码校验，零 LLM 成本
type SkillSchemaValidator struct {
    // 每个 skill 注册自己的 JSON Schema 或校验规则
    schemas map[string]*jsonschema.Schema
}

type SchemaValidationResult struct {
    Valid           bool     `json:"schema_valid"`
    MissingFields   []string `json:"missing_fields"`
    InvalidEnums    []string `json:"invalid_enum_fields"`
    Score           int      `json:"score"` // 100 或 0
}
```

校验内容：
- JSON 格式是否合法
- 必填字段是否存在且非空
- 枚举字段值是否在允许范围内
- 数据类型是否匹配

### 5.2 Layer 2: 规则一致性评估 Prompt

```
你是一个 Skill 规则一致性检查器。你的工作是检查输出是否违反了 skill 定义中的条件逻辑和业务规则。

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
  "score": 0-100,
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
```

> **成本优化**：仅传入 skill 的规则部分（去掉示例、注释、背景说明），可减少约 40% tokens。推荐使用 Haiku 级别模型。
>
> **规则段提取约定**：Layer 2 依赖从 Skill content 中提取规则段落。由于 Skill content 是自由格式 Markdown，需要约定标准标记，让 `extract_rules_section` 做简单文本截取：
>
> ```markdown
> <!-- rules-begin -->
> ## 规则定义
> - risk_level=high 时必须触发 requires_human_confirmation
> - uncertainty.level=medium 时必须包含 reason_code
> ...
> <!-- rules-end -->
> ```
>
> 未包含标记的 Skill 回退为传入完整 content（牺牲成本换取兼容性）。新建或修改 Skill 时应逐步加上标记。

### 5.3 Layer 3: 语义质量评估 Prompt

```
你是一个 Skill 语义质量评估器。你只需关注输出的语义质量，不要检查 JSON 结构合规性或规则一致性（这些由其他评估层负责）。

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
  "score": 0-100,
  "quality_issues": [
    {
      "field": "有问题的字段路径",
      "issue": "具体问题描述，禁止使用'信息不足'等模糊表述",
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
```

### 5.4 评估触发代码（伪代码）

```python
# ===== 即时触发：Skill 执行完成后调用 =====
async def trigger_assessment(usage_log_id: str):
    log = db.get(usage_log_id)

    # ===== Layer 1: Schema 校验（全量，纯代码）=====
    schema_result = schema_validator.validate(log.skill_name, log.output_snapshot)
    db.insert('skill_assessment_results', {
        'usage_log_id': usage_log_id,
        'assessment_layer': 'schema',
        **schema_result
    })

    # 即时采样决策（基于 user_action + content_hash + schema 结果）
    sample_decision = decide_sampling(log)

    # ===== Layer 2: 规则一致性 =====
    if sample_decision.should_check_rules:
        skill_rules = extract_rules_section(log.skill_name, log.skill_content_hash)
        rule_result = await call_llm_judge(
            model='haiku',
            prompt=RULE_CHECK_PROMPT,
            skill_rules=skill_rules,
            input=log.input_snapshot,
            output=log.output_snapshot
        )
        db.insert('skill_assessment_results', {
            'usage_log_id': usage_log_id,
            'assessment_layer': 'rule',
            **rule_result
        })

    # ===== Layer 3: 语义质量 =====
    if sample_decision.should_check_semantic:
        skill_md = get_skill_content_by_hash(log.skill_name, log.skill_content_hash)
        semantic_result = await call_llm_judge(
            model='haiku',  # 或 sonnet（视成本预算）
            prompt=SEMANTIC_CHECK_PROMPT,
            skill_md=skill_md,
            input=log.input_snapshot,
            output=log.output_snapshot
        )
        db.insert('skill_assessment_results', {
            'usage_log_id': usage_log_id,
            'assessment_layer': 'semantic',
            **semantic_result
        })

    db.update(usage_log_id, assessment_status='assessed')


# ===== 延迟触发：用户反馈到达时调用 =====
async def on_user_feedback(usage_log_id: str, feedback_score: int):
    if feedback_score > 2:
        return  # 非差评，不补评

    # 分别检查每层是否已评估，独立补评缺失的层
    has_rule = db.exists('skill_assessment_results',
        usage_log_id=usage_log_id, assessment_layer='rule')
    has_semantic = db.exists('skill_assessment_results',
        usage_log_id=usage_log_id, assessment_layer='semantic')

    if not has_rule:
        await trigger_layer_2(usage_log_id)
    if not has_semantic:
        await trigger_layer_3(usage_log_id)
```

### 5.5 异步执行技术选型

评估任务必须异步执行，不阻塞 Skill 调用链路。推荐方案（按优先级排序）：

| 方案 | 适用阶段 | 优点 | 缺点 |
|------|----------|------|------|
| **goroutine pool + 数据库轮询** | Phase 1-2 | 零外部依赖，实现简单 | 重启丢任务，无重试保障 |
| **Redis 队列（已有依赖）** | Phase 3+ | 可靠投递，支持重试和延迟 | 需要 worker 管理 |
| **独立 CronJob 扫描 pending** | 兜底 | 最简单，天然幂等 | 延迟高（分钟级） |

**建议路径**：Phase 1 用 goroutine pool + 定时扫描 pending 记录兜底（简单可靠），Phase 3 引入 LLM 评估后如果吞吐不够再切换到 Redis 队列。

```go
// Phase 1 推荐实现：后台 worker pool
type AssessmentWorker struct {
    db       *gorm.DB
    poolSize int           // 并发 goroutine 数，建议 3-5
    interval time.Duration // pending 扫描间隔，建议 30s
}

// Skill 调用完成后投递（fire-and-forget）
func (w *AssessmentWorker) Submit(usageLogID string) {
    select {
    case w.taskCh <- usageLogID:
    default:
        // channel 满时标记 pending，由定时扫描兜底
        log.Warn("assessment queue full, will be picked up by scanner")
    }
}
```

---

## 六、Golden Set 校准机制

### 6.1 目的

LLM 评估 LLM 输出，本身存在漂移风险。Golden Set 是评估器的"单元测试"，确保评估器自身的准确性。

### 6.2 工作方式

```
┌────────────────────────────────────────────────────┐
│              Golden Set 校准流程                     │
│                                                    │
│  1. 每个 Skill 维护 3-5 个标注样本                   │
│     - 人工标注 input/output + 期望 verdict/score    │
│     - 涵盖 pass / warn / fail 各类场景             │
│                                                    │
│  2. 定期校准（建议每周 / 评估 prompt 变更后）         │
│     - 用当前评估器跑所有 Golden Set                  │
│     - 对比评估结果和人工标注                         │
│                                                    │
│  3. 计算偏差指标                                    │
│     - verdict 一致率（期望 > 85%）                  │
│     - score 偏差均值（期望 < 10分）                  │
│                                                    │
│  4. 偏差过大时                                      │
│     - 触发告警，暂停该 skill 的 LLM 评估            │
│     - 人工检查并调优评估 prompt                      │
└────────────────────────────────────────────────────┘
```

### 6.3 Golden Set 数据来源

- **初始标注**：从生产数据中挑选典型 case，人工标注
- **持续补充**：评估 pass 但用户差评的 case → 人工复核后加入 Golden Set
- **版本更新**：Skill 定义大幅变更时，需要同步更新 Golden Set

---

## 七、Dashboard 核心指标设计

### 7.1 全局概览页

**技术指标：**

| 指标 | 计算方式 | 刷新频率 |
|------|----------|----------|
| Schema 通过率（7天） | Layer 1 pass / total | 实时 |
| 规则通过率（7天） | Layer 2 pass / (pass+warn+fail) | 每小时 |
| 语义质量均分（7天） | Layer 3 avg(score) | 每小时 |
| 活跃 skill 数 | 近7天有调用的 skill 数 | 每日 |
| 高风险 skill 数 | 任一层 fail率 > 20% 的 skill | 每小时 |

**业务指标：**

| 指标 | 计算方式 | 意义 |
|------|----------|------|
| 用户采纳率 | 确认执行 / 总执行次数 | Skill 输出是否真的有用 |
| 二次修改率 | 手动修改后执行 / 总执行次数 | Skill 输出离用户期望的差距 |
| 负面反馈率 | feedback<=2 / 有反馈的总数 | 用户直接不满意的比例 |
| 评估-反馈一致率 | 评估pass且用户好评 / 同时有评估结果和用户反馈的记录数 | 评估器本身的可信度 |

> **为什么需要业务指标？** 技术指标只能告诉你 Skill 输出是否"合规"，但"合规"不等于"好用"。用户采纳率和二次修改率不需要 LLM 评估，从行为数据直接计算，且更能反映真实质量。
>
> **数据源依赖**：业务指标依赖 `skill_usage_logs` 中新增的 `user_action` 字段（accepted / modified / aborted）。现有 `user_feedback` 是事后评分，`user_action` 是实时行为记录。前端在用户确认执行、手动修改后执行、放弃操作时，需要分别上报对应的 action 值。这是 Phase 1 数据采集的一部分，必须在 Phase 2 Dashboard 之前完成。

### 7.2 单 Skill 详情页

**分层质量趋势（按 content_hash 聚合）**

```
  ↑ 通过率/均分
  |     hash:a3f2.. ●━━━━━━━
  |                  Layer1: 100%  Layer2: 92  Layer3: 85
  |  hash:7b1c.. ●━━━
  |               Layer1: 98%   Layer2: 78  Layer3: 71
  └──────────────────────────────────────────→ 时间

  每次 content_hash 变更后，前20次调用的各层 score 均值作为基准
```

**违规分布图（按 Layer 分组）**

```
Layer 2 - 规则违规:
  requires_human_confirmation 漏触发  ████████ 12次
  uncertainty.reason_code 缺失        █████    7次

Layer 3 - 语义问题:
  decision_hints.title 过于模糊       ███      4次
  risk_highlights 与 title 重复       ██       2次
```

**用户行为对照**

```
评估结果 vs 用户反馈（散点图/矩阵）：
                 用户好评    用户差评
  评估 pass:      85%         8%  ← 关注：评估盲区
  评估 warn:      10%         5%
  评估 fail:       2%        15%  ← 符合预期
```

### 7.3 版本对比页

选择同一 skill 的两个 content_hash，对比：

- 各层通过率/均分变化（+/-）
- 违规类型变化（新增/消除了哪些）
- 用户采纳率变化
- 二次修改率变化

### 7.4 Golden Set 校准报告页

- 各 Skill 的 verdict 一致率
- Score 偏差分布
- 偏差异常的 Skill 标红

### 7.5 告警规则

| 告警 | 触发条件 | 级别 |
|------|----------|------|
| 新版本质量下降 | 新 hash 前20次均分比上一版低 10分+ | P1 |
| Schema 错误率突增 | 某 skill 近1小时 schema_valid=false 率 > 10% | P1 |
| 单次 Schema 错误 | 任何 schema_valid=false（单条记录） | P2 |
| fail 率突增 | 某 skill 近1小时 Layer 2 fail率 > 30% | P1 |
| 评估器漂移 | Golden Set verdict 一致率 < 80% | P2 |
| 用户差评激增 | 某 skill 近24小时负面反馈率 > 25% | P2 |
| 评估-反馈不一致 | 评估 pass 但用户差评占比 > 15% | P2 |
| 评估积压 | assessment_status=pending 超1小时 > 50条 | P3 |

---

## 八、成本控制

### 8.1 分层成本估算

```
Layer 1 (Schema):
  纯代码校验 → 零 LLM 成本，100% 覆盖

Layer 2 (规则一致性):
  输入:
    skill_rules:  ~1000 tokens（仅规则部分，去掉示例和注释）
    input:        ~500  tokens
    output:       ~1000 tokens
    prompt:       ~300  tokens
  输出:           ~300  tokens（评估结果 JSON）
  ──────────────────────────
  合计:         ~3100 tokens/次（输入 2800 + 输出 300）

  按 20% 采样率，日均1000次调用：
  200次 x 3100 tokens = 62万 tokens/天（Haiku 价格极低）

Layer 3 (语义质量):
  输入:
    skill_md:     ~3000 tokens
    input:        ~500  tokens
    output:       ~1000 tokens
    prompt:       ~400  tokens
  输出:           ~500  tokens（评估结果 JSON，含 highlights）
  ──────────────────────────
  合计:         ~5400 tokens/次（输入 4900 + 输出 500）

  按 5% 采样率，日均1000次调用：
  50次 x 5400 tokens = 27万 tokens/天

Golden Set 校准:
  按每周一次，20个 Skill x 5个样本 x 2层（rule + semantic）:
  200次 x ~4250 tokens（两层均值）= 85万 tokens/周 = 约12万 tokens/天

总计: 约 101万 tokens/天
  其中输出 tokens 约占 10%，但单价约为输入的 3-5 倍
  折算实际费用时需按 input/output 分别计价
```

### 8.2 成本优化手段

- **分层本身就是最大的优化**：Layer 1 零成本覆盖 100%，拦截大部分结构问题
- **skill_md 压缩**：Layer 2 只传规则部分，减少 60%+ tokens
- **模型选择**：Layer 2 用 Haiku，Layer 3 视预算选 Haiku 或 Sonnet
- **缓存相似输入**：输入结构高度相似时，复用最近一次评估结果
- **动态采样率**：质量稳定的 Skill 自动降低采样率

### 8.3 数据保留策略

`skill_usage_logs` 带完整 snapshot、`skill_assessment_results` 带 raw_output，数据量会快速增长，需要明确保留周期：

| 数据 | 保留周期 | 到期处理 |
|------|----------|----------|
| `input_snapshot` / `output_snapshot` | 90 天 | 清空字段内容，保留其他元数据（hash、score、verdict） |
| `skill_content_snapshot` | 永久 | 每个 hash 仅一条非空记录，增长量极小 |
| `assessment_raw_output` | 30 天 | 清空字段，保留结构化评估结果 |
| `skill_assessment_results`（结构化字段） | 永久 | 用于趋势分析和版本对比 |
| `user_modification_diff` | 90 天 | 清空字段 |

> 建议通过 pg_cron 或应用层定时任务实现自动清理。清理前先确认对应的聚合统计已生成。

---

## 九、落地路线图

### Phase 1（1-2周）：数据基础 + Schema 校验

- [ ] 扩展 `skill_usage_logs` 表，补充 input_snapshot、output_snapshot、skill_content_hash、skill_content_snapshot、user_action、user_modification_diff 字段
- [ ] 在 Skill 调用链路中落库完整 input/output 和 content hash（content_snapshot 仅 hash 首次出现时写入）
- [ ] 前端上报 user_action（accepted / modified / aborted）和修改 diff
- [ ] 建立 `skill_assessment_results` 表
- [ ] 实现 Layer 1 Schema 校验器（纯代码，全量覆盖）
- [ ] 实现后台 AssessmentWorker（goroutine pool + pending 扫描兜底）
- [ ] Schema 校验结果自动落库

> **Phase 1 交付价值**：零 LLM 成本，即刻发现所有结构类问题，用户行为数据开始积累。

### Phase 2（2-3周）：Dashboard + 业务指标

- [ ] 全局概览页（技术指标 + 业务指标）
- [ ] 单 Skill 详情页（Schema 通过率趋势、用户采纳率/二次修改率）
- [ ] 基础告警（Schema 错误、用户差评激增）

> **Phase 2 交付价值**：仅凭 Schema 校验数据 + 用户行为数据，已可提供有价值的 Dashboard。

### Phase 3（3-4周）：LLM 评估引擎

- [ ] 实现采样决策逻辑（含用户反馈信号触发）
- [ ] 实现 Layer 2 规则一致性评估器（Haiku）
- [ ] 实现 Layer 3 语义质量评估器
- [ ] 评估结果落库 + Dashboard 集成
- [ ] 建立 `skill_golden_sets` 表
- [ ] 每个核心 Skill 初始标注 3-5 个 Golden Set 样本

### Phase 4（4-5周）：校准 + 闭环

- [ ] Golden Set 定期校准任务
- [ ] 评估器漂移告警
- [ ] 评估-反馈一致性分析面板
- [ ] 版本对比页
- [ ] 根据高频违规反向优化 Skill 定义
- [ ] 评估 pass 但用户差评的 case → 自动纳入 Golden Set 候选

### Phase 5（持续迭代）：智能化

- [ ] 动态采样率：质量稳定的 Skill 自动降低采样率
- [ ] Skill 发布门禁：新版本评估分低于阈值时阻断发布
- [ ] 评估 prompt 自动调优：基于 Golden Set 偏差自动建议 prompt 修改

---

## 十、关键设计决策说明

**为什么采用三层评估而不是单一 LLM 评估？**
单一 LLM prompt 同时评估结构、规则、语义，会导致维度混淆（结构完美但语义差，分数难以表达）、无法独立迭代、成本无法精细控制。分层后，每层职责清晰、可独立调优和扩缩。

**为什么用 content_hash + 去重快照而不是独立版本快照表？**
现有 Skill model 没有发布流程，version 字段是手动维护的。content hash 自动追踪变更，消除了"忘记打版本/快照"的风险。回溯 Skill 内容时，通过 `skill_content_snapshot` 字段的去重写入（仅 hash 首次出现时存完整内容）实现，既不丢失历史内容，又避免了独立快照表的维护成本和写入遗漏风险。

**为什么要接入用户反馈信号？**
LLM 评估 LLM 的输出，置信度天然有限。用户反馈是更可靠的质量信号。评估 pass 但用户差评的 case 暴露评估标准的盲区，比 LLM 自说自话更有校准价值。

**为什么需要 Golden Set 校准？**
评估器本身也是 LLM，存在漂移风险。Golden Set 相当于评估器的"单元测试"，提供可量化的准确性基准。没有校准机制的评估系统，时间一长就不知道评估结果本身是否可信。

**为什么评估结果不直接影响线上决策？**
评估是观测手段，不是防火墙。错误的评估结果（LLM judge 也会出错）不应阻断业务。质量问题通过迭代 skill 定义来修复，而不是在运行时拦截。

**为什么 Dashboard 要加业务指标？**
技术指标（通过率、fail率）只反映"合不合规"，不反映"好不好用"。用户采纳率和二次修改率从行为数据直接计算，无需 LLM，是最真实的质量信号。
