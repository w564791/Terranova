-- Skill Assessment Migration
-- Extends skill_usage_logs and adds skill_assessment_results table

-- 1. Extend skill_usage_logs table with assessment-related columns
ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS input_snapshot JSONB;

ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS output_snapshot JSONB;

ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS skill_content_hash VARCHAR(64);

ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS skill_content_snapshot TEXT;

ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS user_action VARCHAR(16);

ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS user_modification_diff TEXT;

ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS latency_ms INTEGER;

ALTER TABLE skill_usage_logs
ADD COLUMN IF NOT EXISTS assessment_status VARCHAR(16) DEFAULT 'pending';

COMMENT ON COLUMN skill_usage_logs.input_snapshot IS '调用时的完整输入';
COMMENT ON COLUMN skill_usage_logs.output_snapshot IS '完整输出 JSON';
COMMENT ON COLUMN skill_usage_logs.skill_content_hash IS 'skill content 的 SHA256';
COMMENT ON COLUMN skill_usage_logs.skill_content_snapshot IS 'skill content 完整快照（仅 hash 首次出现时写入）';
COMMENT ON COLUMN skill_usage_logs.user_action IS '用户行为：accepted | modified | aborted';
COMMENT ON COLUMN skill_usage_logs.user_modification_diff IS '用户修改的 diff';
COMMENT ON COLUMN skill_usage_logs.latency_ms IS '调用耗时（毫秒）';
COMMENT ON COLUMN skill_usage_logs.assessment_status IS '评估状态：pending | assessed';

-- 2. Create skill_assessment_results table
CREATE TABLE IF NOT EXISTS skill_assessment_results (
    id VARCHAR(36) PRIMARY KEY,
    usage_log_id VARCHAR(36) NOT NULL REFERENCES skill_usage_logs(id),
    skill_name VARCHAR(128) NOT NULL,
    skill_content_hash VARCHAR(64) NOT NULL,
    assessed_at TIMESTAMPTZ DEFAULT NOW(),
    assessment_layer VARCHAR(16) NOT NULL,
    verdict VARCHAR(16) NOT NULL,
    score SMALLINT NOT NULL,
    assessment_latency_ms INTEGER,
    schema_valid BOOLEAN,
    missing_fields TEXT[],
    invalid_enum_fields TEXT[],
    rule_violations JSONB,
    quality_issues JSONB,
    assessment_confidence VARCHAR(16),
    assessment_model VARCHAR(64),
    assessment_raw_output TEXT
);

COMMENT ON TABLE skill_assessment_results IS 'Skill 质量评估结果记录';
COMMENT ON COLUMN skill_assessment_results.assessment_layer IS '评估层级：schema | rule | semantic';
COMMENT ON COLUMN skill_assessment_results.verdict IS '评估结果：pass | warn | fail';
COMMENT ON COLUMN skill_assessment_results.score IS '评分：0-100';
COMMENT ON COLUMN skill_assessment_results.assessment_latency_ms IS '评估耗时（毫秒）';
COMMENT ON COLUMN skill_assessment_results.schema_valid IS 'Schema 校验是否通过';
COMMENT ON COLUMN skill_assessment_results.missing_fields IS '缺失的必需字段';
COMMENT ON COLUMN skill_assessment_results.invalid_enum_fields IS '枚举值无效的字段';
COMMENT ON COLUMN skill_assessment_results.rule_violations IS '规则违反详情';
COMMENT ON COLUMN skill_assessment_results.quality_issues IS '质量问题详情';
COMMENT ON COLUMN skill_assessment_results.assessment_confidence IS '评估置信度';
COMMENT ON COLUMN skill_assessment_results.assessment_model IS '评估使用的 AI 模型';
COMMENT ON COLUMN skill_assessment_results.assessment_raw_output IS '评估原始输出';

-- 3. Create indexes for skill_assessment_results
CREATE INDEX IF NOT EXISTS idx_assessment_skill_hash
ON skill_assessment_results (skill_name, skill_content_hash);

CREATE INDEX IF NOT EXISTS idx_assessment_usage_layer
ON skill_assessment_results (usage_log_id, assessment_layer);

CREATE INDEX IF NOT EXISTS idx_assessment_verdict
ON skill_assessment_results (verdict);

CREATE INDEX IF NOT EXISTS idx_assessment_at
ON skill_assessment_results (assessed_at DESC);
