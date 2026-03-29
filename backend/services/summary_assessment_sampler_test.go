package services

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSummarySamplerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time { return time.Now() },
	})
	if err != nil {
		t.Fatal(err)
	}
	// Use raw SQL to create a simplified table for testing
	db.Exec(`CREATE TABLE IF NOT EXISTS skill_assessment_results (
		id VARCHAR(36) PRIMARY KEY,
		usage_log_id VARCHAR(36) DEFAULT '',
		skill_name VARCHAR(128) NOT NULL,
		skill_content_hash VARCHAR(64) NOT NULL,
		assessed_at TEXT DEFAULT '',
		assessment_layer VARCHAR(16) NOT NULL,
		verdict VARCHAR(16) NOT NULL,
		score SMALLINT NOT NULL DEFAULT 0,
		source_type VARCHAR(16) DEFAULT 'skill'
	)`)
	return db
}

func TestSummarySampler_L1Fail_FullEval(t *testing.T) {
	db := setupSummarySamplerTestDB(t)
	s := NewSummaryAssessmentSampler(db)

	decision := s.Decide("aws_instance", false, false)
	if !decision.ShouldEvalRule || !decision.ShouldEvalSemantic {
		t.Error("L1 fail should trigger full L2+L3 evaluation")
	}
}

func TestSummarySampler_NewResourceType_FullEval(t *testing.T) {
	db := setupSummarySamplerTestDB(t)
	s := NewSummaryAssessmentSampler(db)

	decision := s.Decide("aws_new_resource_type", true, false)
	if !decision.ShouldEvalRule || !decision.ShouldEvalSemantic {
		t.Error("new resource type should trigger full L2+L3 evaluation")
	}
}

func TestSummarySampler_ExistingType_DefaultSampling(t *testing.T) {
	db := setupSummarySamplerTestDB(t)
	db.Exec(`INSERT INTO skill_assessment_results (id, skill_name, skill_content_hash, assessment_layer, verdict, score, source_type) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"prior-1", "aws_instance", "h1", "schema", "pass", 100, "summary")

	s := NewSummaryAssessmentSampler(db)
	ruleCount, semanticCount := 0, 0
	for i := 0; i < 100; i++ {
		d := s.Decide("aws_instance", true, false)
		if d.ShouldEvalRule {
			ruleCount++
		}
		if d.ShouldEvalSemantic {
			semanticCount++
		}
	}
	// Default: L2=10%, L3=5%
	if ruleCount == 0 || ruleCount == 100 {
		t.Errorf("rule sampling looks wrong: %d/100 (expected ~10%%)", ruleCount)
	}
	if semanticCount == 100 {
		t.Errorf("semantic sampling looks wrong: %d/100 (expected ~5%%)", semanticCount)
	}
}

func TestSummarySampler_ConfigChanged_HigherRate(t *testing.T) {
	db := setupSummarySamplerTestDB(t)
	db.Exec(`INSERT INTO skill_assessment_results (id, skill_name, skill_content_hash, assessment_layer, verdict, score, source_type) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"prior-1", "aws_instance", "h1", "schema", "pass", 100, "summary")

	s := NewSummaryAssessmentSampler(db)
	ruleCount := 0
	for i := 0; i < 100; i++ {
		d := s.Decide("aws_instance", true, true)
		if d.ShouldEvalRule {
			ruleCount++
		}
	}
	// Config changed: L2=50%
	if ruleCount < 20 {
		t.Errorf("config changed rule sampling too low: %d/100 (expected ~50%%)", ruleCount)
	}
}
