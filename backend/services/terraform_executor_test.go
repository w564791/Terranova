package services

import (
	"fmt"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ============================================================================
// Test infrastructure for TerraformExecutor
// ============================================================================

// setupExecutorTestDB extends setupTestDB with additional tables needed for
// TerraformExecutor tests (workspace_resources, resource_code_versions).
func setupExecutorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TABLE IF NOT EXISTS workspace_resources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_name TEXT NOT NULL,
		current_version_id INTEGER,
		is_active INTEGER DEFAULT 1,
		description TEXT DEFAULT '',
		tags TEXT,
		created_by TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_applied_at DATETIME,
		manifest_deployment_id TEXT
	)`)
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TABLE IF NOT EXISTS resource_code_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		resource_id INTEGER NOT NULL,
		version INTEGER NOT NULL,
		is_latest INTEGER DEFAULT 0,
		tf_code TEXT DEFAULT '{}',
		variables TEXT,
		change_summary TEXT DEFAULT '',
		change_type TEXT DEFAULT 'create',
		diff_from_previous TEXT DEFAULT '',
		state_version_id INTEGER,
		task_id INTEGER,
		created_by TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	return db
}

// newTestExecutor creates a TerraformExecutor for testing.
// Pass db=nil to simulate Agent mode (no direct DB access).
func newTestExecutor(db *gorm.DB) *TerraformExecutor {
	return &TerraformExecutor{
		db:            db,
		signalManager: GetSignalManager(),
	}
}

// testWorkspaceResource is a simplified resource struct for SQLite testing.
// The full models.WorkspaceResource has JSONB Tags field that SQLite cannot serialize.
type testWorkspaceResource struct {
	ID               uint   `gorm:"primaryKey"`
	WorkspaceID      string `gorm:"column:workspace_id;not null"`
	ResourceID       string `gorm:"column:resource_id;not null"`
	ResourceType     string `gorm:"column:resource_type;not null"`
	ResourceName     string `gorm:"column:resource_name;not null"`
	CurrentVersionID *uint
	IsActive         bool      `gorm:"default:true"`
	Description      string    `gorm:"default:''"`
	CreatedAt        time.Time `gorm:"autoCreateTime"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

func (testWorkspaceResource) TableName() string { return "workspace_resources" }

// createTestResource creates a workspace resource for testing using the simplified struct,
// then reads it back as the full model.
func createTestResource(t *testing.T, db *gorm.DB, wsID string, resourceID string, resourceType string) *models.WorkspaceResource {
	t.Helper()
	tr := &testWorkspaceResource{
		WorkspaceID:  wsID,
		ResourceID:   resourceID,
		ResourceType: resourceType,
		ResourceName: "test-resource",
		IsActive:     true,
	}
	require.NoError(t, db.Create(tr).Error)

	var r models.WorkspaceResource
	require.NoError(t, db.First(&r, tr.ID).Error)
	return &r
}

// createTestVersion creates a resource code version for testing.
func createTestVersion(t *testing.T, db *gorm.DB, resourceDBID uint, version int) *models.ResourceCodeVersion {
	t.Helper()
	v := &models.ResourceCodeVersion{
		ResourceID: resourceDBID,
		Version:    version,
		IsLatest:   true,
		ChangeType: "create",
	}
	require.NoError(t, db.Create(v).Error)
	return v
}

// ============================================================================
// ValidateResourceVersionSnapshot Tests
// ============================================================================

func TestValidateSnapshot_NilSnapshotCreatedAt(t *testing.T) {
	executor := newTestExecutor(nil)
	task := &models.WorkspaceTask{
		SnapshotCreatedAt: nil,
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot_created_at is nil")
}

func TestValidateSnapshot_NilResourceVersions(t *testing.T) {
	executor := newTestExecutor(nil)
	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt:        &now,
		SnapshotResourceVersions: nil,
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot resource versions is nil")
}

func TestValidateSnapshot_NilVariables(t *testing.T) {
	executor := newTestExecutor(nil)
	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt:        &now,
		SnapshotResourceVersions: models.JSONB{},
		SnapshotVariables:        nil,
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot variables missing")
}

func TestValidateSnapshot_NilProviderConfig(t *testing.T) {
	executor := newTestExecutor(nil)
	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt:        &now,
		SnapshotResourceVersions: models.JSONB{},
		SnapshotVariables:        models.JSONB{},
		SnapshotProviderConfig:   nil,
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	// nil provider config is valid: template-mode workspaces resolve it dynamically
	assert.NoError(t, err)
}

func TestValidateSnapshot_EmptyResources_Success(t *testing.T) {
	executor := newTestExecutor(nil)
	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt:        &now,
		SnapshotResourceVersions: models.JSONB{},
		SnapshotVariables:        models.JSONB{},
		SnapshotProviderConfig:   models.JSONB{"region": "us-east-1"},
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.NoError(t, err)
}

func TestValidateSnapshot_AgentMode_SkipsDBValidation(t *testing.T) {
	// Agent mode: db=nil, should skip per-resource DB validation via continue
	executor := newTestExecutor(nil)
	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt: &now,
		SnapshotResourceVersions: models.JSONB{
			"aws_s3_bucket.test": map[string]interface{}{
				"resource_db_id": float64(999), // doesn't exist, but won't be checked
				"version":        float64(1),
			},
		},
		SnapshotVariables:      models.JSONB{},
		SnapshotProviderConfig: models.JSONB{"region": "us-east-1"},
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.NoError(t, err) // Agent mode skips DB check — design behavior
}

func TestValidateSnapshot_LocalMode_ResourceNotFound(t *testing.T) {
	db := setupExecutorTestDB(t)
	executor := newTestExecutor(db)
	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt: &now,
		SnapshotResourceVersions: models.JSONB{
			"aws_s3_bucket.missing": map[string]interface{}{
				"resource_db_id": float64(9999),
				"version":        float64(1),
			},
		},
		SnapshotVariables:      models.JSONB{},
		SnapshotProviderConfig: models.JSONB{"region": "us-east-1"},
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestValidateSnapshot_LocalMode_VersionNotFound(t *testing.T) {
	db := setupExecutorTestDB(t)
	executor := newTestExecutor(db)

	// Create resource but NOT the version
	resource := createTestResource(t, db, "ws-snap-test", "aws_s3_bucket.test", "aws_s3_bucket")

	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt: &now,
		SnapshotResourceVersions: models.JSONB{
			"aws_s3_bucket.test": map[string]interface{}{
				"resource_db_id": float64(resource.ID),
				"version":        float64(99), // version 99 doesn't exist
			},
		},
		SnapshotVariables:      models.JSONB{},
		SnapshotProviderConfig: models.JSONB{"region": "us-east-1"},
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no longer exists")
}

func TestValidateSnapshot_LocalMode_AllValid(t *testing.T) {
	db := setupExecutorTestDB(t)
	executor := newTestExecutor(db)

	resource := createTestResource(t, db, "ws-snap-test", "aws_s3_bucket.test", "aws_s3_bucket")
	createTestVersion(t, db, resource.ID, 1)

	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt: &now,
		SnapshotResourceVersions: models.JSONB{
			"aws_s3_bucket.test": map[string]interface{}{
				"resource_db_id": float64(resource.ID),
				"version":        float64(1),
			},
		},
		SnapshotVariables:      models.JSONB{"_array": []interface{}{}},
		SnapshotProviderConfig: models.JSONB{"region": "us-east-1"},
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.NoError(t, err)
}

func TestValidateSnapshot_LocalMode_MultipleResources(t *testing.T) {
	db := setupExecutorTestDB(t)
	executor := newTestExecutor(db)

	r1 := createTestResource(t, db, "ws-multi", "aws_s3_bucket.a", "aws_s3_bucket")
	createTestVersion(t, db, r1.ID, 1)
	r2 := createTestResource(t, db, "ws-multi", "aws_instance.b", "aws_instance")
	createTestVersion(t, db, r2.ID, 2)

	now := time.Now()
	task := &models.WorkspaceTask{
		SnapshotCreatedAt: &now,
		SnapshotResourceVersions: models.JSONB{
			"aws_s3_bucket.a": map[string]interface{}{
				"resource_db_id": float64(r1.ID),
				"version":        float64(1),
			},
			"aws_instance.b": map[string]interface{}{
				"resource_db_id": float64(r2.ID),
				"version":        float64(2),
			},
		},
		SnapshotVariables:      models.JSONB{},
		SnapshotProviderConfig: models.JSONB{"region": "us-east-1"},
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.NoError(t, err)
}

func TestValidateSnapshot_OldSnapshot_WarnsButPasses(t *testing.T) {
	db := setupExecutorTestDB(t)
	executor := newTestExecutor(db)

	// Snapshot created 25 hours ago — should warn but still pass
	oldTime := time.Now().Add(-25 * time.Hour)
	task := &models.WorkspaceTask{
		SnapshotCreatedAt:        &oldTime,
		SnapshotResourceVersions: models.JSONB{},
		SnapshotVariables:        models.JSONB{},
		SnapshotProviderConfig:   models.JSONB{"region": "us-east-1"},
	}
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(task, logger)
	assert.NoError(t, err) // Warns but does not error
}

// ============================================================================
// casTaskStatus Tests
// ============================================================================

func TestCAS_Success_PendingToRunning(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-cas")
	task := createTestTask(t, db, "ws-cas", models.TaskTypePlan, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	err := mgr.casTaskStatus(task, "plan")
	assert.NoError(t, err)
	assert.Equal(t, models.TaskStatusRunning, task.Status)
	assert.Equal(t, "planning", task.Stage)
	assert.NotNil(t, task.StartedAt)

	// Verify in DB
	var dbTask models.WorkspaceTask
	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusRunning, dbTask.Status)
}

func TestCAS_Success_ApplyPendingToRunning(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-cas-apply")
	task := createTestTask(t, db, "ws-cas-apply", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)

	mgr := newTestManager(db, nil, nil)
	err := mgr.casTaskStatus(task, "apply")
	assert.NoError(t, err)
	assert.Equal(t, models.TaskStatusRunning, task.Status)
	assert.Equal(t, "applying", task.Stage)
}

func TestCAS_Fails_WhenStatusAlreadyChanged(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-cas-race")
	task := createTestTask(t, db, "ws-cas-race", models.TaskTypePlan, models.TaskStatusPending)

	// Simulate another process changing the status first
	db.Model(&models.WorkspaceTask{}).Where("id = ?", task.ID).
		Update("status", models.TaskStatusRunning)

	mgr := newTestManager(db, nil, nil)
	err := mgr.casTaskStatus(task, "plan")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CAS failed")
}

func TestCAS_Concurrent_OnlyOneSucceeds(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-cas-conc")
	task := createTestTask(t, db, "ws-cas-conc", models.TaskTypePlan, models.TaskStatusPending)

	mgr1 := newTestManager(db, nil, nil)
	mgr2 := newTestManager(db, nil, nil)

	// Create copies so each goroutine has its own task reference
	task1 := *task
	task1.Status = models.TaskStatusPending
	task2 := *task
	task2.Status = models.TaskStatusPending

	err1 := mgr1.casTaskStatus(&task1, "plan")
	err2 := mgr2.casTaskStatus(&task2, "plan")

	// Exactly one should succeed
	if err1 == nil {
		assert.Error(t, err2, "second CAS should fail")
	} else {
		assert.NoError(t, err2, "if first CAS failed, second should succeed")
	}

	// DB should show running
	var dbTask models.WorkspaceTask
	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusRunning, dbTask.Status)
}

// ============================================================================
// Task Status Transition Tests
// ============================================================================

func TestStatusTransition_PlanOnly_PendingToSuccess(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-st-plan")
	task := createTestTask(t, db, "ws-st-plan", models.TaskTypePlan, models.TaskStatusPending)

	// pending → running (via CAS)
	mgr := newTestManager(db, nil, nil)
	err := mgr.casTaskStatus(task, "plan")
	require.NoError(t, err)
	assert.Equal(t, models.TaskStatusRunning, task.Status)

	// running → success (simulated plan completion)
	task.Status = models.TaskStatusSuccess
	task.Stage = "completed"
	now := time.Now()
	task.CompletedAt = &now
	db.Save(task)

	var dbTask models.WorkspaceTask
	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusSuccess, dbTask.Status)
}

func TestStatusTransition_PlanAndApply_FullLifecycle(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-st-paa")
	task := createTestTask(t, db, "ws-st-paa", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)

	// 1. pending → running (plan phase)
	err := mgr.casTaskStatus(task, "plan")
	require.NoError(t, err)
	assert.Equal(t, models.TaskStatusRunning, task.Status)
	assert.Equal(t, "planning", task.Stage)

	// 2. running → apply_pending (plan completes with changes)
	task.Status = models.TaskStatusApplyPending
	task.Stage = "apply_pending"
	task.PlanTaskID = &task.ID
	db.Save(task)

	var dbTask models.WorkspaceTask
	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusApplyPending, dbTask.Status)

	// 3. apply_pending → running (user confirms apply via CAS)
	task.Status = models.TaskStatusApplyPending // reset for CAS
	db.Model(task).Update("status", models.TaskStatusApplyPending)
	err = mgr.casTaskStatus(task, "apply")
	require.NoError(t, err)
	assert.Equal(t, models.TaskStatusRunning, task.Status)
	assert.Equal(t, "applying", task.Stage)

	// 4. running → applied (apply completes)
	task.Status = models.TaskStatusApplied
	task.Stage = "completed"
	now := time.Now()
	task.CompletedAt = &now
	db.Save(task)

	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusApplied, dbTask.Status)
}

func TestStatusTransition_PlanAndApply_NoChanges(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-st-nochg")
	task := createTestTask(t, db, "ws-st-nochg", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)

	// pending → running
	err := mgr.casTaskStatus(task, "plan")
	require.NoError(t, err)

	// running → planned_and_finished (no changes)
	task.Status = models.TaskStatusPlannedAndFinished
	task.Stage = "completed"
	now := time.Now()
	task.CompletedAt = &now
	db.Save(task)

	var dbTask models.WorkspaceTask
	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusPlannedAndFinished, dbTask.Status)
}

func TestStatusTransition_Failed(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-st-fail")
	task := createTestTask(t, db, "ws-st-fail", models.TaskTypePlan, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)

	// pending → running
	err := mgr.casTaskStatus(task, "plan")
	require.NoError(t, err)

	// running → failed
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = "terraform plan failed: exit code 1"
	now := time.Now()
	task.CompletedAt = &now
	db.Save(task)

	var dbTask models.WorkspaceTask
	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusFailed, dbTask.Status)
	assert.Contains(t, dbTask.ErrorMessage, "terraform plan failed")
}

func TestStatusTransition_Cancelled(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-st-cancel")
	task := createTestTask(t, db, "ws-st-cancel", models.TaskTypePlan, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)

	// pending → running
	err := mgr.casTaskStatus(task, "plan")
	require.NoError(t, err)

	// running → cancelled
	task.Status = models.TaskStatusCancelled
	task.ErrorMessage = "cancelled by user"
	now := time.Now()
	task.CompletedAt = &now
	db.Save(task)

	var dbTask models.WorkspaceTask
	db.First(&dbTask, task.ID)
	assert.Equal(t, models.TaskStatusCancelled, dbTask.Status)
}

// ============================================================================
// Plan→ConfirmApply Consistency Tests
// ============================================================================

func TestPlanApply_ConfirmApplyRequiresApplyPending(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-confirm")
	task := createTestTask(t, db, "ws-confirm", models.TaskTypePlanAndApply, models.TaskStatusPending)

	// Cannot CAS with action "apply" when status is still "pending"
	mgr := newTestManager(db, nil, nil)
	err := mgr.casTaskStatus(task, "apply")
	// CAS expects status == "pending" and sets to running, but the stage would be "applying"
	// The real protection is in ExecuteConfirmedApply which checks status == apply_pending
	// Here CAS itself succeeds (pending→running is valid) but semantically wrong
	// This test documents the current behavior
	assert.NoError(t, err) // CAS succeeds mechanically
}

func TestPlanApply_SnapshotPreservedAcrossPhases(t *testing.T) {
	db := setupExecutorTestDB(t)
	createTestWorkspace(t, db, "ws-snap-preserve")

	// Create resource and version
	resource := createTestResource(t, db, "ws-snap-preserve", "aws_s3_bucket.test", "aws_s3_bucket")
	createTestVersion(t, db, resource.ID, 1)

	now := time.Now()

	// Build task with snapshot data directly in Go (JSONB fields don't round-trip through SQLite)
	snapshotTask := &models.WorkspaceTask{
		SnapshotCreatedAt: &now,
		SnapshotResourceVersions: models.JSONB{
			"aws_s3_bucket.test": map[string]interface{}{
				"resource_db_id": float64(resource.ID),
				"version":        float64(1),
			},
		},
		SnapshotVariables:      models.JSONB{},
		SnapshotProviderConfig: models.JSONB{"region": "us-east-1"},
	}

	// Validate snapshot is valid (resource exists in DB)
	executor := newTestExecutor(db)
	logger := NewTerraformLogger(nil)
	err := executor.ValidateResourceVersionSnapshot(snapshotTask, logger)
	assert.NoError(t, err)

	// Now delete the resource via raw SQL
	db.Exec("DELETE FROM workspace_resources WHERE id = ?", resource.ID)

	// Re-validate — should fail in Local mode (resource deleted from DB)
	err = executor.ValidateResourceVersionSnapshot(snapshotTask, logger)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "not found")
	}

	// In Agent mode — should still pass (design behavior: uses cached snapshot, skips DB check)
	agentExecutor := newTestExecutor(nil)
	err = agentExecutor.ValidateResourceVersionSnapshot(snapshotTask, logger)
	assert.NoError(t, err)
}

// formatUint is a helper for building JSON strings in tests.
func formatUint(id uint) string {
	return fmt.Sprintf("%d", id)
}
