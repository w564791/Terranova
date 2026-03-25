package services

import (
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSamplerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	require.NoError(t, err)

	err = db.Exec(`
		CREATE TABLE skill_usage_logs (
			id TEXT PRIMARY KEY,
			skill_ids TEXT NOT NULL,
			capability TEXT NOT NULL,
			workspace_id TEXT,
			user_id TEXT NOT NULL,
			module_id INTEGER,
			execution_time_ms INTEGER,
			user_feedback INTEGER,
			ai_model TEXT,
			context_summary TEXT,
			response_summary TEXT,
			created_at DATETIME,
			input_snapshot TEXT,
			output_snapshot TEXT,
			skill_content_hash TEXT,
			skill_content_snapshot TEXT,
			user_action TEXT,
			user_modification_diff TEXT,
			latency_ms INTEGER,
			assessment_status TEXT DEFAULT 'pending'
		)
	`).Error
	require.NoError(t, err)

	err = db.Exec(`
		CREATE TABLE skill_assessment_results (
			id TEXT PRIMARY KEY,
			usage_log_id TEXT,
			skill_name TEXT NOT NULL,
			skill_content_hash TEXT NOT NULL,
			assessed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			assessment_layer TEXT NOT NULL,
			verdict TEXT NOT NULL,
			score INTEGER NOT NULL,
			assessment_latency_ms INTEGER,
			schema_valid INTEGER,
			missing_fields TEXT,
			invalid_enum_fields TEXT,
			rule_violations TEXT,
			quality_issues TEXT,
			assessment_confidence TEXT,
			assessment_model TEXT,
			assessment_raw_output TEXT
		)
	`).Error
	require.NoError(t, err)

	return db
}

// Helper to insert N existing assessment results for a given content hash and layer.
func insertAssessmentResults(t *testing.T, db *gorm.DB, contentHash string, layer models.AssessmentLayer, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		err := db.Exec(`
			INSERT INTO skill_assessment_results (id, usage_log_id, skill_name, skill_content_hash, assessment_layer, verdict, score)
			VALUES (?, ?, 'test_skill', ?, ?, 'pass', 80)
		`, uuid.New().String(), uuid.New().String(), contentHash, string(layer)).Error
		require.NoError(t, err)
	}
}

func samplerStrPtr(s string) *string {
	return &s
}

func TestSampler_NewVersion(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// content_hash with 0 existing rule evals -> both true
	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "brand_new_hash",
		AssessmentStatus: "pending",
	}

	decision := sampler.Decide(log, true)
	require.True(t, decision.ShouldEvalRule, "new version should trigger rule eval")
	require.True(t, decision.ShouldEvalSemantic, "new version should trigger semantic eval")
	require.Equal(t, "new_version", decision.Reason)
}

func TestSampler_NewVersionWithSomeResults(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// Insert 19 existing results (still < 20)
	insertAssessmentResults(t, db, "partially_known_hash", models.AssessmentLayerRule, 19)

	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "partially_known_hash",
		AssessmentStatus: "pending",
	}

	decision := sampler.Decide(log, true)
	require.True(t, decision.ShouldEvalRule)
	require.True(t, decision.ShouldEvalSemantic)
	require.Equal(t, "new_version", decision.Reason)
}

func TestSampler_NotNewVersionWith20Results(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// Insert exactly 20 existing results (>= 20, no longer new)
	insertAssessmentResults(t, db, "well_known_hash", models.AssessmentLayerRule, 20)

	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "well_known_hash",
		UserAction:       samplerStrPtr("accepted"),
		AssessmentStatus: "pending",
	}

	// With 20 results and accepted action, should fall to default sampling
	decision := sampler.Decide(log, true)
	require.Equal(t, "random_sample", decision.Reason)
}

func TestSampler_SchemaFailed(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// Make it not a new version so we reach the schema_failed check
	insertAssessmentResults(t, db, "known_hash", models.AssessmentLayerRule, 25)

	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "known_hash",
		AssessmentStatus: "pending",
	}

	decision := sampler.Decide(log, false)
	require.True(t, decision.ShouldEvalRule, "schema failed should trigger rule eval")
	require.Equal(t, "schema_failed", decision.Reason)
	// L3 is sampling (random), so we just check that L2 is definitely true
}

func TestSampler_UserAborted(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// Make it not a new version
	insertAssessmentResults(t, db, "known_hash", models.AssessmentLayerRule, 25)

	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "known_hash",
		UserAction:       samplerStrPtr("aborted"),
		AssessmentStatus: "pending",
	}

	decision := sampler.Decide(log, true)
	require.True(t, decision.ShouldEvalRule, "aborted should trigger rule eval")
	require.True(t, decision.ShouldEvalSemantic, "aborted should trigger semantic eval")
	require.Equal(t, "user_aborted", decision.Reason)
}

func TestSampler_UserModified(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// Make it not a new version
	insertAssessmentResults(t, db, "known_hash", models.AssessmentLayerRule, 25)

	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "known_hash",
		UserAction:       samplerStrPtr("modified"),
		AssessmentStatus: "pending",
	}

	decision := sampler.Decide(log, true)
	require.True(t, decision.ShouldEvalSemantic, "modified should trigger semantic eval")
	require.Equal(t, "user_modified", decision.Reason)
	// L2 is sampling (random), so we just check that L3 is definitely true
}

func TestSampler_DefaultSampling(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// Make it not a new version, schema valid, accepted action
	insertAssessmentResults(t, db, "stable_hash", models.AssessmentLayerRule, 30)

	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "stable_hash",
		UserAction:       samplerStrPtr("accepted"),
		AssessmentStatus: "pending",
	}

	// Run many times and verify random behavior
	ruleCount := 0
	semanticCount := 0
	runs := 10000

	for i := 0; i < runs; i++ {
		decision := sampler.Decide(log, true)
		require.Equal(t, "random_sample", decision.Reason)
		if decision.ShouldEvalRule {
			ruleCount++
		}
		if decision.ShouldEvalSemantic {
			semanticCount++
		}
	}

	// L2 should be ~20% (allow 15%-25%)
	ruleRate := float64(ruleCount) / float64(runs)
	require.InDelta(t, 0.20, ruleRate, 0.05, "rule sampling rate should be ~20%%")

	// L3 should be ~5% (allow 2%-8%)
	semanticRate := float64(semanticCount) / float64(runs)
	require.InDelta(t, 0.05, semanticRate, 0.03, "semantic sampling rate should be ~5%%")
}

func TestSampler_NilUserAction(t *testing.T) {
	db := setupSamplerTestDB(t)
	sampler := NewAssessmentSampler(db)

	// Make it not a new version
	insertAssessmentResults(t, db, "nil_action_hash", models.AssessmentLayerRule, 25)

	log := &models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillContentHash: "nil_action_hash",
		UserAction:       nil, // nil user action
		AssessmentStatus: "pending",
	}

	// Should fall to default sampling without panic
	decision := sampler.Decide(log, true)
	require.Equal(t, "random_sample", decision.Reason)
}
