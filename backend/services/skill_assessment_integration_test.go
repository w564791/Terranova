package services

import (
	"encoding/json"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSkillAssessmentIntegrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	require.NoError(t, err)

	// Manually create tables with SQLite-compatible types
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

// TestAssessment_EndToEnd_Pass verifies the full assessment flow with valid output
func TestAssessment_EndToEnd_Pass(t *testing.T) {
	// 1. Setup: in-memory SQLite DB, AutoMigrate
	db := setupSkillAssessmentIntegrationTestDB(t)

	// 2. Create a SkillSchemaValidator, register schema
	validator := NewSkillSchemaValidator()
	validator.RegisterSchema("test-task", SkillOutputSchema{
		RequiredFields: []string{"status", "config"},
		EnumFields: map[string][]string{
			"status": {"complete", "blocked"},
		},
	})

	// 3. Create an AssessmentWorker
	worker := NewAssessmentWorker(db, validator, nil, nil)

	// 4. Insert a SkillUsageLog with valid output
	validOutput := json.RawMessage(`{"status":"complete","config":{"vpc_id":"vpc-123"}}`)
	usageLog := models.SkillUsageLog{
		ID:               "e2e-test-1",
		SkillIDs:         []string{"skill-1"},
		Capability:       "test_capability",
		UserID:           "user-1",
		OutputSnapshot:   &validOutput,
		SkillContentHash: "hash-abc",
		AssessmentStatus: string(models.AssessmentStatusPending),
		CreatedAt:        time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// 5. Call worker.ProcessOne
	processed, err := worker.ProcessOne("e2e-test-1", "test-task")
	require.NoError(t, err)
	require.True(t, processed, "Expected record to be processed")

	// 6. Query SkillAssessmentResult where usage_log_id="e2e-test-1" and assessment_layer="schema"
	var result models.SkillAssessmentResult
	err = db.Where("usage_log_id = ? AND assessment_layer = ?", "e2e-test-1", models.AssessmentLayerSchema).
		First(&result).Error
	require.NoError(t, err)

	// 7. Assert: verdict=pass, score=100, schema_valid=true
	require.Equal(t, models.AssessmentVerdictPass, result.Verdict)
	require.Equal(t, int16(100), result.Score)
	require.NotNil(t, result.SchemaValid)
	require.True(t, *result.SchemaValid)
	require.Empty(t, result.MissingFields)
	require.Empty(t, result.InvalidEnumFields)

	// 8. Query SkillUsageLog, assert assessment_status="assessed"
	var updatedLog models.SkillUsageLog
	err = db.First(&updatedLog, "id = ?", "e2e-test-1").Error
	require.NoError(t, err)
	require.Equal(t, string(models.AssessmentStatusAssessed), updatedLog.AssessmentStatus)
}

// TestAssessment_EndToEnd_Fail verifies the full assessment flow with invalid output
func TestAssessment_EndToEnd_Fail(t *testing.T) {
	// 1. Setup
	db := setupSkillAssessmentIntegrationTestDB(t)

	// 2. Create validator and register schema
	validator := NewSkillSchemaValidator()
	validator.RegisterSchema("test-task", SkillOutputSchema{
		RequiredFields: []string{"status", "config"},
		EnumFields: map[string][]string{
			"status": {"complete", "blocked"},
		},
	})

	// 3. Create worker
	worker := NewAssessmentWorker(db, validator, nil, nil)

	// 4. Insert log with missing "config" field and invalid enum value
	invalidOutput := json.RawMessage(`{"status":"unknown_value"}`)
	usageLog := models.SkillUsageLog{
		ID:               "e2e-test-2",
		SkillIDs:         []string{"skill-2"},
		Capability:       "test_capability",
		UserID:           "user-2",
		OutputSnapshot:   &invalidOutput,
		SkillContentHash: "hash-def",
		AssessmentStatus: string(models.AssessmentStatusPending),
		CreatedAt:        time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// 5. Process the record
	processed, err := worker.ProcessOne("e2e-test-2", "test-task")
	require.NoError(t, err)
	require.True(t, processed)

	// 6. Query assessment result
	var result models.SkillAssessmentResult
	err = db.Where("usage_log_id = ? AND assessment_layer = ?", "e2e-test-2", models.AssessmentLayerSchema).
		First(&result).Error
	require.NoError(t, err)

	// 7. Assert: verdict=fail, score=0, schema_valid=false
	require.Equal(t, models.AssessmentVerdictFail, result.Verdict)
	require.Equal(t, int16(0), result.Score)
	require.NotNil(t, result.SchemaValid)
	require.False(t, *result.SchemaValid)

	// Should have missing "config" field
	require.Contains(t, result.MissingFields, "config")

	// Should have invalid enum value for "status"
	require.Len(t, result.InvalidEnumFields, 1)
	require.Contains(t, result.InvalidEnumFields[0], "status=unknown_value")

	// 8. Verify usage log status was updated
	var updatedLog models.SkillUsageLog
	err = db.First(&updatedLog, "id = ?", "e2e-test-2").Error
	require.NoError(t, err)
	require.Equal(t, string(models.AssessmentStatusAssessed), updatedLog.AssessmentStatus)
}

// TestAssessment_EndToEnd_InvalidJSON verifies the full assessment flow with invalid JSON
func TestAssessment_EndToEnd_InvalidJSON(t *testing.T) {
	// 1. Setup
	db := setupSkillAssessmentIntegrationTestDB(t)

	// 2. Create validator and register schema
	validator := NewSkillSchemaValidator()
	validator.RegisterSchema("test-task", SkillOutputSchema{
		RequiredFields: []string{"status", "config"},
		EnumFields: map[string][]string{
			"status": {"complete", "blocked"},
		},
	})

	// 3. Create worker
	worker := NewAssessmentWorker(db, validator, nil, nil)

	// 4. Insert log with invalid JSON
	invalidJSON := json.RawMessage(`not valid json`)
	usageLog := models.SkillUsageLog{
		ID:               "e2e-test-3",
		SkillIDs:         []string{"skill-3"},
		Capability:       "test_capability",
		UserID:           "user-3",
		OutputSnapshot:   &invalidJSON,
		SkillContentHash: "hash-ghi",
		AssessmentStatus: string(models.AssessmentStatusPending),
		CreatedAt:        time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// 5. Process the record
	processed, err := worker.ProcessOne("e2e-test-3", "test-task")
	require.NoError(t, err)
	require.True(t, processed)

	// 6. Query assessment result
	var result models.SkillAssessmentResult
	err = db.Where("usage_log_id = ? AND assessment_layer = ?", "e2e-test-3", models.AssessmentLayerSchema).
		First(&result).Error
	require.NoError(t, err)

	// 7. Assert: verdict=fail, score=0
	require.Equal(t, models.AssessmentVerdictFail, result.Verdict)
	require.Equal(t, int16(0), result.Score)
	require.NotNil(t, result.SchemaValid)
	require.False(t, *result.SchemaValid)

	// 8. Verify usage log status was updated
	var updatedLog models.SkillUsageLog
	err = db.First(&updatedLog, "id = ?", "e2e-test-3").Error
	require.NoError(t, err)
	require.Equal(t, string(models.AssessmentStatusAssessed), updatedLog.AssessmentStatus)
}
