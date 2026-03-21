package services

import (
	"iac-platform/internal/models"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSummaryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Migrate tables
	db.AutoMigrate(&models.AIConfig{}, &models.WorkspaceTask{}, &models.AIPlanSummary{}, &models.AIApplySummary{})
	return db
}

func TestGeneratePlanSummary_NoAIConfig(t *testing.T) {
	db := setupSummaryTestDB(t)

	// Create a task with changes but NO AI config
	task := models.WorkspaceTask{
		WorkspaceID:    "ws-test",
		Status:         "success",
		ChangesAdd:     2,
		ChangesChange:  1,
		ChangesDestroy: 0,
	}
	db.Create(&task)

	svc := NewAISummaryService(db)
	svc.GeneratePlanSummary(task.ID)

	// Verify no record created
	var count int64
	db.Model(&models.AIPlanSummary{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no plan summary records, got %d", count)
	}
}

func TestGeneratePlanSummary_NoChanges(t *testing.T) {
	db := setupSummaryTestDB(t)

	// Create AI config
	db.Create(&models.AIConfig{
		ServiceType:  "bedrock",
		ModelID:      "test-model",
		AWSRegion:    "us-west-2",
		Enabled:      false,
		Capabilities: models.StringArray{"summary"},
	})

	// Create task with zero changes
	task := models.WorkspaceTask{
		WorkspaceID:    "ws-test",
		Status:         "success",
		ChangesAdd:     0,
		ChangesChange:  0,
		ChangesDestroy: 0,
	}
	db.Create(&task)

	svc := NewAISummaryService(db)
	svc.GeneratePlanSummary(task.ID)

	var count int64
	db.Model(&models.AIPlanSummary{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no plan summary records, got %d", count)
	}
}

func TestGeneratePlanSummary_DuplicatePrevention(t *testing.T) {
	db := setupSummaryTestDB(t)

	// Create AI config
	db.Create(&models.AIConfig{
		ServiceType:  "bedrock",
		ModelID:      "test-model",
		AWSRegion:    "us-west-2",
		Enabled:      false,
		Capabilities: models.StringArray{"summary"},
	})

	// Create task
	task := models.WorkspaceTask{
		WorkspaceID:    "ws-test",
		Status:         "success",
		ChangesAdd:     3,
		ChangesChange:  0,
		ChangesDestroy: 0,
	}
	db.Create(&task)

	// Pre-create existing summary
	db.Create(&models.AIPlanSummary{
		ID:          "plsm-existing12345678",
		TaskID:      task.ID,
		WorkspaceID: "ws-test",
		Status:      "completed",
		CreatedAt:   time.Now(),
	})

	svc := NewAISummaryService(db)
	svc.GeneratePlanSummary(task.ID)

	// Should still be just 1 record
	var count int64
	db.Model(&models.AIPlanSummary{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 plan summary record (existing), got %d", count)
	}
}

func TestGetPlanSummary(t *testing.T) {
	db := setupSummaryTestDB(t)

	db.Create(&models.AIPlanSummary{
		ID:              "plsm-test1234567890",
		TaskID:          42,
		WorkspaceID:     "ws-test",
		Status:          "completed",
		ChangesOverview: "test overview",
		CreatedAt:       time.Now(),
	})

	svc := NewAISummaryService(db)

	// Found
	result := svc.GetPlanSummary(42)
	if result == nil {
		t.Fatal("expected plan summary, got nil")
	}
	if result.ChangesOverview != "test overview" {
		t.Errorf("wrong overview: %s", result.ChangesOverview)
	}

	// Not found
	result = svc.GetPlanSummary(999)
	if result != nil {
		t.Error("expected nil for non-existent task")
	}
}

func TestRetryPlanSummary_OnlyFailedAllowed(t *testing.T) {
	db := setupSummaryTestDB(t)

	db.Create(&models.AIPlanSummary{
		ID:          "plsm-completed123456",
		TaskID:      42,
		WorkspaceID: "ws-test",
		Status:      "completed",
		CreatedAt:   time.Now(),
	})

	svc := NewAISummaryService(db)
	err := svc.RetryPlanSummary(42)
	if err == nil {
		t.Error("expected error when retrying completed summary")
	}
}
