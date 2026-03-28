package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAssessmentTestDB(t *testing.T) *gorm.DB {
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

func TestAssessmentWorker_ProcessesPendingRecord(t *testing.T) {
	db := setupAssessmentTestDB(t)

	// Create validator and register schema
	validator := NewSkillSchemaValidator()
	validator.RegisterSchema("test_skill", SkillOutputSchema{
		RequiredFields: []string{"status", "result"},
		EnumFields: map[string][]string{
			"status": {"success", "failure", "pending"},
		},
	})

	// Create worker
	worker := NewAssessmentWorker(db, validator, nil, nil)

	// Create a pending usage log with valid output
	validOutput := json.RawMessage(`{"status": "success", "result": "Operation completed"}`)
	usageLog := models.SkillUsageLog{
		ID:                  uuid.New().String(),
		SkillIDs:            []string{"skill-1"},
		Capability:          "test_capability",
		UserID:              "user-1",
		OutputSnapshot:      &validOutput,
		SkillContentHash:    "abc123",
		AssessmentStatus:    "pending",
		CreatedAt:           time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// Process the record
	success, err := worker.ProcessOne(usageLog.ID, "test_skill")
	require.NoError(t, err)
	require.True(t, success)

	// Verify assessment result was created
	var result models.SkillAssessmentResult
	err = db.Where("usage_log_id = ?", usageLog.ID).First(&result).Error
	require.NoError(t, err)

	require.Equal(t, "test_skill", result.SkillName)
	require.Equal(t, "abc123", result.SkillContentHash)
	require.Equal(t, models.AssessmentLayerSchema, result.AssessmentLayer)
	require.Equal(t, models.AssessmentVerdictPass, result.Verdict)
	require.Equal(t, int16(100), result.Score)
	require.NotNil(t, result.SchemaValid)
	require.True(t, *result.SchemaValid)
	require.Empty(t, result.MissingFields)
	require.Empty(t, result.InvalidEnumFields)
	require.NotNil(t, result.AssessmentLatencyMs)

	// Verify usage log status was updated
	var updatedLog models.SkillUsageLog
	err = db.First(&updatedLog, "id = ?", usageLog.ID).Error
	require.NoError(t, err)
	require.Equal(t, string(models.AssessmentStatusAssessed), updatedLog.AssessmentStatus)
}

func TestAssessmentWorker_SchemaFail(t *testing.T) {
	db := setupAssessmentTestDB(t)

	// Create validator and register schema
	validator := NewSkillSchemaValidator()
	validator.RegisterSchema("test_skill", SkillOutputSchema{
		RequiredFields: []string{"status", "result"},
		EnumFields:     map[string][]string{},
	})

	// Create worker
	worker := NewAssessmentWorker(db, validator, nil, nil)

	// Create a pending usage log with missing required field
	invalidOutput := json.RawMessage(`{"status": "success"}`)
	usageLog := models.SkillUsageLog{
		ID:                  uuid.New().String(),
		SkillIDs:            []string{"skill-1"},
		Capability:          "test_capability",
		UserID:              "user-1",
		OutputSnapshot:      &invalidOutput,
		SkillContentHash:    "def456",
		AssessmentStatus:    "pending",
		CreatedAt:           time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// Process the record
	success, err := worker.ProcessOne(usageLog.ID, "test_skill")
	require.NoError(t, err)
	require.True(t, success)

	// Verify assessment result was created with fail verdict
	var result models.SkillAssessmentResult
	err = db.Where("usage_log_id = ?", usageLog.ID).First(&result).Error
	require.NoError(t, err)

	require.Equal(t, models.AssessmentVerdictFail, result.Verdict)
	require.Equal(t, int16(0), result.Score)
	require.NotNil(t, result.SchemaValid)
	require.False(t, *result.SchemaValid)
	require.Contains(t, result.MissingFields, "result")

	// Verify usage log status was updated
	var updatedLog models.SkillUsageLog
	err = db.First(&updatedLog, "id = ?", usageLog.ID).Error
	require.NoError(t, err)
	require.Equal(t, string(models.AssessmentStatusAssessed), updatedLog.AssessmentStatus)
}

func TestAssessmentWorker_AlreadyAssessed(t *testing.T) {
	db := setupAssessmentTestDB(t)

	validator := NewSkillSchemaValidator()
	worker := NewAssessmentWorker(db, validator, nil, nil)

	// Create a usage log that's already assessed
	validOutput := json.RawMessage(`{"status": "success"}`)
	usageLog := models.SkillUsageLog{
		ID:                  uuid.New().String(),
		SkillIDs:            []string{"skill-1"},
		Capability:          "test_capability",
		UserID:              "user-1",
		OutputSnapshot:      &validOutput,
		SkillContentHash:    "ghi789",
		AssessmentStatus:    "assessed",
		CreatedAt:           time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// Try to process the record
	success, err := worker.ProcessOne(usageLog.ID, "test_skill")
	require.NoError(t, err)
	require.False(t, success) // Should return false since already assessed

	// Verify no new assessment result was created
	var count int64
	db.Model(&models.SkillAssessmentResult{}).Where("usage_log_id = ?", usageLog.ID).Count(&count)
	require.Equal(t, int64(0), count)
}

func TestAssessmentWorker_NullOutput(t *testing.T) {
	db := setupAssessmentTestDB(t)

	validator := NewSkillSchemaValidator()
	validator.RegisterSchema("test_skill", SkillOutputSchema{
		RequiredFields: []string{"status"},
	})

	worker := NewAssessmentWorker(db, validator, nil, nil)

	// Create a usage log with nil output (will use default "null")
	usageLog := models.SkillUsageLog{
		ID:                  uuid.New().String(),
		SkillIDs:            []string{"skill-1"},
		Capability:          "test_capability",
		UserID:              "user-1",
		OutputSnapshot:      nil,
		SkillContentHash:    "null123",
		AssessmentStatus:    "pending",
		CreatedAt:           time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// Process the record
	success, err := worker.ProcessOne(usageLog.ID, "test_skill")
	require.NoError(t, err)
	require.True(t, success)

	// Verify assessment result shows fail for null output
	var result models.SkillAssessmentResult
	err = db.Where("usage_log_id = ?", usageLog.ID).First(&result).Error
	require.NoError(t, err)

	require.Equal(t, models.AssessmentVerdictFail, result.Verdict)
	require.Equal(t, int16(0), result.Score)
}

func TestAssessmentWorker_Submit(t *testing.T) {
	db := setupAssessmentTestDB(t)

	validator := NewSkillSchemaValidator()
	worker := NewAssessmentWorker(db, validator, nil, nil)

	// Submit should not block even if channel is full
	// Test with many submissions
	for i := 0; i < 200; i++ {
		worker.Submit(uuid.New().String(), "test_skill")
	}

	// Should not panic or block
	require.True(t, true)
}

func TestAssessmentWorker_StartStop(t *testing.T) {
	db := setupAssessmentTestDB(t)

	validator := NewSkillSchemaValidator()
	worker := NewAssessmentWorker(db, validator, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start worker
	go worker.Start(ctx)

	// Give it a moment to start
	time.Sleep(100 * time.Millisecond)

	// Create a pending usage log
	validOutput := json.RawMessage(`{"status": "success"}`)
	usageLog := models.SkillUsageLog{
		ID:                  uuid.New().String(),
		SkillIDs:            []string{"skill-1"},
		Capability:          "test_capability",
		UserID:              "user-1",
		OutputSnapshot:      &validOutput,
		SkillContentHash:    "start123",
		AssessmentStatus:    "pending",
		CreatedAt:           time.Now(),
	}
	err := db.Create(&usageLog).Error
	require.NoError(t, err)

	// Wait for scanner to pick it up (scanner runs every 30s, but we'll use a short timeout)
	time.Sleep(500 * time.Millisecond)

	// Stop worker
	worker.Stop()

	// Wait a bit for it to fully stop
	time.Sleep(100 * time.Millisecond)

	// Worker should have stopped
	require.False(t, worker.IsRunning())
}

func TestAssessmentWorker_UsesCapabilityThenHashAsFallback(t *testing.T) {
	db := setupAssessmentTestDB(t)

	validator := NewSkillSchemaValidator()
	// 注册 capability 名作为 schema key
	validator.RegisterSchema("test_capability", SkillOutputSchema{
		RequiredFields: []string{"data"},
	})

	worker := NewAssessmentWorker(db, validator, nil, nil)

	// Case 1: 有 capability → 用 capability 作为 schema key
	validOutput := json.RawMessage(`{"data": "test"}`)
	log1 := models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillIDs:         []string{"skill-1"},
		Capability:       "test_capability",
		UserID:           "user-1",
		OutputSnapshot:   &validOutput,
		SkillContentHash: "some_hash",
		AssessmentStatus: "pending",
		CreatedAt:        time.Now(),
	}
	require.NoError(t, db.Create(&log1).Error)

	success, err := worker.ProcessOne(log1.ID, "")
	require.NoError(t, err)
	require.True(t, success)

	var result1 models.SkillAssessmentResult
	require.NoError(t, db.Where("usage_log_id = ?", log1.ID).First(&result1).Error)
	require.Equal(t, "test_capability", result1.SkillName) // capability 优先
	require.Equal(t, models.AssessmentVerdictPass, result1.Verdict)

	// Case 2: capability 为空 → fallback 到 content_hash
	validator.RegisterSchema("fallback_hash", SkillOutputSchema{
		RequiredFields: []string{"data"},
	})
	log2 := models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillIDs:         []string{"skill-1"},
		Capability:       "",
		UserID:           "user-1",
		OutputSnapshot:   &validOutput,
		SkillContentHash: "fallback_hash",
		AssessmentStatus: "pending",
		CreatedAt:        time.Now(),
	}
	require.NoError(t, db.Create(&log2).Error)

	success, err = worker.ProcessOne(log2.ID, "")
	require.NoError(t, err)
	require.True(t, success)

	var result2 models.SkillAssessmentResult
	require.NoError(t, db.Where("usage_log_id = ?", log2.ID).First(&result2).Error)
	require.Equal(t, "fallback_hash", result2.SkillName) // hash fallback
}

func TestAssessmentWorker_Idempotent(t *testing.T) {
	db := setupAssessmentTestDB(t)

	validator := NewSkillSchemaValidator()
	validator.RegisterSchema("test_skill", SkillOutputSchema{
		RequiredFields: []string{"status"},
	})

	worker := NewAssessmentWorker(db, validator, nil, nil)

	validOutput := json.RawMessage(`{"status": "ok"}`)
	usageLog := models.SkillUsageLog{
		ID:               uuid.New().String(),
		SkillIDs:         []string{"skill-1"},
		Capability:       "test_capability",
		UserID:           "user-1",
		OutputSnapshot:   &validOutput,
		SkillContentHash: "idem123",
		AssessmentStatus: "pending",
		CreatedAt:        time.Now(),
	}
	require.NoError(t, db.Create(&usageLog).Error)

	// First call succeeds
	success1, err := worker.ProcessOne(usageLog.ID, "test_skill")
	require.NoError(t, err)
	require.True(t, success1)

	// Second call on the same log should be a no-op (CAS fails)
	success2, err := worker.ProcessOne(usageLog.ID, "test_skill")
	require.NoError(t, err)
	require.False(t, success2)

	// Only one schema assessment record should exist
	var count int64
	db.Model(&models.SkillAssessmentResult{}).
		Where("usage_log_id = ? AND assessment_layer = 'schema'", usageLog.ID).
		Count(&count)
	require.Equal(t, int64(1), count)
}
