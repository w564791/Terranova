CREATE TABLE IF NOT EXISTS skill_golden_sets (
  id                  VARCHAR(36) PRIMARY KEY,
  skill_name          VARCHAR(128) NOT NULL,
  assessment_layer    VARCHAR(16)  NOT NULL,  -- rule | semantic
  input_snapshot      JSONB        NOT NULL,
  output_snapshot     JSONB        NOT NULL,
  expected_verdict    VARCHAR(16)  NOT NULL,  -- pass | warn | fail
  expected_score_min  SMALLINT     NOT NULL,
  expected_score_max  SMALLINT     NOT NULL,
  annotations         JSONB,
  created_by          VARCHAR(128),
  created_at          TIMESTAMPTZ  DEFAULT NOW(),
  is_active           BOOLEAN      DEFAULT true
);

CREATE INDEX IF NOT EXISTS idx_golden_skill_layer ON skill_golden_sets (skill_name, assessment_layer) WHERE is_active = true;
