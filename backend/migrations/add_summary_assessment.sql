-- Summary Assessment Migration
-- Extends skill_assessment_results for summary evaluation
-- Extends resource_index with assessment tracking

-- 0. Allow NULL usage_log_id (summary assessments have no usage log)
ALTER TABLE skill_assessment_results DROP CONSTRAINT IF EXISTS skill_assessment_results_usage_log_id_fkey;
ALTER TABLE skill_assessment_results ALTER COLUMN usage_log_id DROP NOT NULL;

-- 1. Extend skill_assessment_results table
ALTER TABLE skill_assessment_results
ADD COLUMN IF NOT EXISTS source_type VARCHAR(16) DEFAULT 'skill';

ALTER TABLE skill_assessment_results
ADD COLUMN IF NOT EXISTS resource_id INTEGER;

ALTER TABLE skill_assessment_results
ADD COLUMN IF NOT EXISTS format_violations TEXT[];

ALTER TABLE skill_assessment_results
ADD COLUMN IF NOT EXISTS security_tag_misses JSONB;

ALTER TABLE skill_assessment_results
ADD COLUMN IF NOT EXISTS hallucination_suspects TEXT[];

COMMENT ON COLUMN skill_assessment_results.source_type IS '评估来源：skill | summary';
COMMENT ON COLUMN skill_assessment_results.resource_id IS '关联 resource_index.id（summary 评估时使用）';
COMMENT ON COLUMN skill_assessment_results.format_violations IS 'L1: 检测到的格式问题列表';
COMMENT ON COLUMN skill_assessment_results.security_tag_misses IS 'L1: 应标注但缺失的安全标签';
COMMENT ON COLUMN skill_assessment_results.hallucination_suspects IS 'L1: 摘要中无法在原始属性中溯源的值';

-- 2. Indexes
CREATE INDEX IF NOT EXISTS idx_assessment_source_type
ON skill_assessment_results (source_type);

CREATE INDEX IF NOT EXISTS idx_assessment_resource_id
ON skill_assessment_results (resource_id) WHERE resource_id IS NOT NULL;

-- 3. Extend resource_index table
ALTER TABLE resource_index
ADD COLUMN IF NOT EXISTS summary_assessment_status VARCHAR(16) DEFAULT '';

COMMENT ON COLUMN resource_index.summary_assessment_status IS '摘要评估状态：空 | pending | assessed';

ALTER TABLE resource_index
ADD COLUMN IF NOT EXISTS summary_regeneration_hint TEXT DEFAULT '';

COMMENT ON COLUMN resource_index.summary_regeneration_hint IS '重新生成时的质量反馈提示（评估问题摘要）';
