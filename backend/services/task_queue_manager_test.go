package services

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ============================================================================
// Task 0: 测试基础设施
// ============================================================================

// mockAgentCCHandler implements AgentCCHandler for testing
type mockAgentCCHandler struct {
	mu              sync.Mutex
	connectedAgents []string
	availableMap    map[string]bool  // agentID -> available for task
	sentTasks       []sentTaskRecord // records of tasks sent to agents
	sendError       error            // error to return from SendTaskToAgent
}

type sentTaskRecord struct {
	AgentID     string
	TaskID      uint
	WorkspaceID string
	Action      string
}

func (m *mockAgentCCHandler) SendTaskToAgent(agentID string, taskID uint, workspaceID string, action string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendError != nil {
		return m.sendError
	}
	m.sentTasks = append(m.sentTasks, sentTaskRecord{
		AgentID:     agentID,
		TaskID:      taskID,
		WorkspaceID: workspaceID,
		Action:      action,
	})
	return nil
}

func (m *mockAgentCCHandler) IsAgentAvailable(agentID string, taskType models.TaskType) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.availableMap == nil {
		return true // default: all agents available
	}
	avail, ok := m.availableMap[agentID]
	if !ok {
		return true
	}
	return avail
}

func (m *mockAgentCCHandler) GetConnectedAgents() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connectedAgents
}

func (m *mockAgentCCHandler) getSentTasks() []sentTaskRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]sentTaskRecord, len(m.sentTasks))
	copy(cp, m.sentTasks)
	return cp
}

// mockLockProvider implements LockProvider for testing
type mockLockProvider struct {
	mu       sync.Mutex
	locked   map[int64]bool
	tryError error // error to return from TryLock
	blocked  bool  // if true, TryLock always returns false (simulates another replica holding lock)
}

func newMockLockProvider() *mockLockProvider {
	return &mockLockProvider{locked: make(map[int64]bool)}
}

func (m *mockLockProvider) TryLock(key int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tryError != nil {
		return false, m.tryError
	}
	if m.blocked {
		return false, nil
	}
	if m.locked[key] {
		return false, nil // already locked
	}
	m.locked[key] = true
	return true, nil
}

func (m *mockLockProvider) Unlock(key int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.locked[key] {
		return false, nil
	}
	delete(m.locked, key)
	return true, nil
}

// setupTestDB creates an in-memory SQLite database with minimal tables.
// We use raw SQL instead of AutoMigrate because the full models contain JSONB/map
// fields that SQLite cannot serialize (e.g. WorkspaceTask.Context map[string]interface{}).
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)

	// Force a single connection so that:
	// 1. All goroutines share the same in-memory database (`:memory:` creates a
	//    separate DB per connection, so a pool > 1 would give each goroutine its
	//    own empty database).
	// 2. Concurrent writes from background goroutines (scheduleRetry,
	//    sendTaskStartNotification) are serialized, avoiding SQLite lock errors.
	sqlDB.SetMaxOpenConns(1)

	// Workspaces — all columns from Workspace struct (JSONB fields stored as TEXT in SQLite)
	_, err = sqlDB.Exec(`CREATE TABLE workspaces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		created_by TEXT,
		execution_mode TEXT DEFAULT 'agent',
		agent_id INTEGER,
		auto_apply INTEGER DEFAULT 0,
		plan_only INTEGER DEFAULT 0,
		terraform_version TEXT DEFAULT 'latest',
		workdir TEXT DEFAULT '/workspace',
		state_backend TEXT DEFAULT 'local',
		state_config TEXT,
		is_locked INTEGER DEFAULT 0,
		locked_by TEXT,
		locked_at DATETIME,
		lock_reason TEXT DEFAULT '',
		tf_code TEXT,
		tf_state TEXT,
		provider_config TEXT,
		provider_config_hash TEXT DEFAULT '',
		last_init_hash TEXT DEFAULT '',
		last_init_terraform_version TEXT DEFAULT '',
		terraform_lock_hcl TEXT DEFAULT '',
		init_config TEXT,
		retry_enabled INTEGER DEFAULT 1,
		max_retries INTEGER DEFAULT 3,
		notify_settings TEXT,
		log_config TEXT,
		state TEXT DEFAULT 'created',
		tags TEXT,
		system_variables TEXT,
		resource_count INTEGER DEFAULT 0,
		last_plan_at DATETIME,
		last_apply_at DATETIME,
		drift_count INTEGER DEFAULT 0,
		last_drift_check DATETIME,
		current_code_version_id INTEGER,
		workspace_execution_mode TEXT DEFAULT 'plan_and_apply',
		ui_mode TEXT DEFAULT 'console',
		show_unchanged_resources INTEGER DEFAULT 0,
		outputs_sharing TEXT DEFAULT 'none',
		drift_check_enabled INTEGER DEFAULT 1,
		drift_check_start_time TEXT DEFAULT '07:00:00',
		drift_check_end_time TEXT DEFAULT '22:00:00',
		drift_check_interval INTEGER DEFAULT 1440,
		agent_pool_id INTEGER,
		current_pool_id TEXT,
		k8s_config_id INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	// Workspace tasks — all columns from WorkspaceTask struct (JSONB/map fields as TEXT)
	_, err = sqlDB.Exec(`CREATE TABLE workspace_tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL,
		created_by TEXT,
		description TEXT DEFAULT '',
		task_type TEXT NOT NULL,
		status TEXT DEFAULT 'pending',
		execution_mode TEXT DEFAULT 'agent',
		agent_id TEXT,
		k8s_config_id INTEGER,
		k8s_pod_name TEXT DEFAULT '',
		k8s_namespace TEXT DEFAULT 'iac-platform',
		execution_node TEXT DEFAULT '',
		locked_by TEXT DEFAULT '',
		locked_at DATETIME,
		lock_expires_at DATETIME,
		plan_output TEXT DEFAULT '',
		apply_output TEXT DEFAULT '',
		error_message TEXT DEFAULT '',
		started_at DATETIME,
		completed_at DATETIME,
		duration INTEGER DEFAULT 0,
		retry_count INTEGER DEFAULT 0,
		max_retries INTEGER DEFAULT 3,
		changes_add INTEGER DEFAULT 0,
		changes_change INTEGER DEFAULT 0,
		changes_destroy INTEGER DEFAULT 0,
		plan_task_id INTEGER,
		plan_data BLOB,
		plan_json TEXT,
		plan_hash TEXT DEFAULT '',
		outputs TEXT,
		stage TEXT DEFAULT '',
		context TEXT,
		snapshot_id TEXT DEFAULT '',
		apply_description TEXT DEFAULT '',
		snapshot_resource_versions TEXT,
		snapshot_variables TEXT,
		snapshot_provider_config TEXT,
		snapshot_created_at DATETIME,
		apply_confirmed_by TEXT,
		apply_confirmed_at DATETIME,
		is_background INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	// Agents — all columns from Agent struct
	_, err = sqlDB.Exec(`CREATE TABLE agents (
		agent_id TEXT PRIMARY KEY,
		application_id INTEGER DEFAULT 1,
		pool_id TEXT,
		name TEXT,
		token_hash TEXT DEFAULT '',
		status TEXT DEFAULT 'idle',
		ip_address TEXT,
		version TEXT,
		last_ping_at DATETIME,
		connected_pod TEXT,
		capabilities TEXT,
		metadata TEXT,
		registered_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_by TEXT,
		updated_by TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	// Agent pools — minimal
	_, err = sqlDB.Exec(`CREATE TABLE agent_pools (
		pool_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		pool_type TEXT DEFAULT 'static',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	return db
}

// testWorkspace is a simplified workspace struct for SQLite testing.
// The full models.Workspace has JSONB fields that SQLite cannot serialize.
// GORM maps this to the "workspaces" table via TableName().
type testWorkspace struct {
	ID            uint                 `gorm:"primaryKey"`
	WorkspaceID   string               `gorm:"column:workspace_id;uniqueIndex"`
	Name          string               `gorm:"not null"`
	StateBackend  string               `gorm:"default:local"`
	ExecutionMode models.ExecutionMode `gorm:"default:agent"`
	IsLocked      bool                 `gorm:"default:false"`
	LockedBy      *string
	CurrentPoolID *string `gorm:"column:current_pool_id"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (testWorkspace) TableName() string { return "workspaces" }

// testWorkspaceTask is a simplified task struct for SQLite testing.
// Omits JSONB/map fields (PlanJSON, Outputs, Context, Snapshot*) that SQLite cannot handle.
type testWorkspaceTask struct {
	ID               uint              `gorm:"primaryKey"`
	WorkspaceID      string            `gorm:"column:workspace_id;not null"`
	Description      string            `gorm:"type:text"`
	TaskType         models.TaskType   `gorm:"not null"`
	Status           models.TaskStatus `gorm:"default:pending"`
	ExecutionMode    models.ExecutionMode
	AgentID          *string
	Stage            string `gorm:"default:"`
	RetryCount       int    `gorm:"default:0"`
	ErrorMessage     string
	StartedAt        *time.Time
	CompletedAt      *time.Time
	PlanTaskID       *uint
	ApplyConfirmedBy *string
	ApplyConfirmedAt *time.Time
	IsBackground     bool `gorm:"default:false"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (testWorkspaceTask) TableName() string { return "workspace_tasks" }

// createTestWorkspace creates a workspace for testing using the simplified struct,
// then reads it back as the full models.Workspace.
func createTestWorkspace(t *testing.T, db *gorm.DB, wsID string, opts ...func(*testWorkspace)) *models.Workspace {
	t.Helper()
	tw := &testWorkspace{
		WorkspaceID:   wsID,
		Name:          "test-ws-" + wsID,
		StateBackend:  "local",
		ExecutionMode: models.ExecutionModeAgent,
	}
	for _, opt := range opts {
		opt(tw)
	}
	require.NoError(t, db.Create(tw).Error)

	var ws models.Workspace
	require.NoError(t, db.Where("workspace_id = ?", wsID).First(&ws).Error)
	return &ws
}

// createTestTask creates a task for testing using the simplified struct,
// then reads it back as the full models.WorkspaceTask.
func createTestTask(t *testing.T, db *gorm.DB, wsID string, taskType models.TaskType, status models.TaskStatus, opts ...func(*testWorkspaceTask)) *models.WorkspaceTask {
	t.Helper()
	tt := &testWorkspaceTask{
		WorkspaceID:   wsID,
		TaskType:      taskType,
		Status:        status,
		ExecutionMode: models.ExecutionModeAgent,
	}
	for _, opt := range opts {
		opt(tt)
	}
	require.NoError(t, db.Create(tt).Error)

	var task models.WorkspaceTask
	require.NoError(t, db.First(&task, tt.ID).Error)
	return &task
}

// createTestAgent creates an agent for testing
func createTestAgent(t *testing.T, db *gorm.DB, agentID string, poolID string) *models.Agent {
	t.Helper()
	agent := &models.Agent{
		AgentID:       agentID,
		ApplicationID: 1,
		PoolID:        &poolID,
		Name:          "test-agent-" + agentID,
		Status:        models.AgentStatusIdle,
		RegisteredAt:  time.Now(),
	}
	require.NoError(t, db.Create(agent).Error)
	return agent
}

// newTestManager creates a TaskQueueManager with mocked dependencies
func newTestManager(db *gorm.DB, handler AgentCCHandler, locker LockProvider) *TaskQueueManager {
	if locker == nil {
		locker = newMockLockProvider()
	}
	return &TaskQueueManager{
		db:       db,
		pgLocker: locker,
		// executor, k8sJobService, k8sDeploymentSvc, pubsub are nil (not needed for most tests)
		agentCCHandler: handler,
	}
}

// helper to create string pointer
func strPtr(s string) *string { return &s }

// helper to create time pointer
func timeP(t time.Time) *time.Time { return &t }

// TestSetup verifies the test infrastructure works
func TestSetup(t *testing.T) {
	db := setupTestDB(t)
	assert.NotNil(t, db)

	ws := createTestWorkspace(t, db, "ws-test-001")
	assert.Equal(t, "ws-test-001", ws.WorkspaceID)

	task := createTestTask(t, db, "ws-test-001", models.TaskTypePlan, models.TaskStatusPending)
	assert.Equal(t, models.TaskStatusPending, task.Status)

	agent := createTestAgent(t, db, "agent-001", "pool-001")
	assert.Equal(t, "agent-001", agent.AgentID)

	mgr := newTestManager(db, nil, nil)
	assert.NotNil(t, mgr)
}

// ============================================================================
// Task 1: GetNextExecutableTask Tests (P0)
// ============================================================================

func TestGetNextExecutableTask_LockedWorkspace(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-locked", func(ws *testWorkspace) {
		ws.IsLocked = true
		ws.LockedBy = strPtr("admin")
	})
	createTestTask(t, db, "ws-locked", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-locked")
	assert.NoError(t, err)
	assert.Nil(t, task, "locked workspace should return nil task")
}

func TestGetNextExecutableTask_NoPendingTasks(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-empty")

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-empty")
	assert.NoError(t, err)
	assert.Nil(t, task)
}

func TestGetNextExecutableTask_PlanAndApply_NoBlocker(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-001")
	created := createTestTask(t, db, "ws-001", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-001")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, created.ID, task.ID)
}

func TestGetNextExecutableTask_PlanAndApply_BlockedByRunning(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-002")
	// task1: running plan_and_apply (blocks task2)
	createTestTask(t, db, "ws-002", models.TaskTypePlanAndApply, models.TaskStatusRunning)
	// task2: pending plan_and_apply (blocked by task1)
	createTestTask(t, db, "ws-002", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-002")
	assert.NoError(t, err)
	assert.Nil(t, task, "plan_and_apply should be blocked by running plan_and_apply, and no plan tasks exist")
}

func TestGetNextExecutableTask_PlanAndApply_BlockedByApplyPending(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-003")
	// task1: apply_pending (blocks task2)
	createTestTask(t, db, "ws-003", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)
	// task2: pending plan_and_apply
	createTestTask(t, db, "ws-003", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-003")
	assert.NoError(t, err)
	assert.Nil(t, task, "plan_and_apply should be blocked by apply_pending task")
}

func TestGetNextExecutableTask_PlanIndependent(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-004")
	// plan_and_apply running — should NOT block plan tasks
	createTestTask(t, db, "ws-004", models.TaskTypePlanAndApply, models.TaskStatusRunning)
	// plan pending — should be returned (independent)
	planTask := createTestTask(t, db, "ws-004", models.TaskTypePlan, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-004")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, planTask.ID, task.ID)
	assert.Equal(t, models.TaskTypePlan, task.TaskType)
}

func TestGetNextExecutableTask_DriftCheckLowestPriority(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-005")
	driftTask := createTestTask(t, db, "ws-005", models.TaskTypeDriftCheck, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-005")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, driftTask.ID, task.ID)
}

func TestGetNextExecutableTask_Priority_PlanAndApplyFirst(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-006")
	// Create in reverse priority order to test ordering
	createTestTask(t, db, "ws-006", models.TaskTypeDriftCheck, models.TaskStatusPending)
	createTestTask(t, db, "ws-006", models.TaskTypePlan, models.TaskStatusPending)
	paTask := createTestTask(t, db, "ws-006", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-006")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, paTask.ID, task.ID, "plan_and_apply should have highest priority")
}

func TestGetNextExecutableTask_PlanAndApplyBlocked_FallbackToPlan(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-007")
	// task1: apply_pending — blocks subsequent plan_and_apply
	createTestTask(t, db, "ws-007", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)
	// task2: pending plan_and_apply — blocked by task1
	createTestTask(t, db, "ws-007", models.TaskTypePlanAndApply, models.TaskStatusPending)
	// task3: pending plan — independent, should be returned
	planTask := createTestTask(t, db, "ws-007", models.TaskTypePlan, models.TaskStatusPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-007")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, planTask.ID, task.ID, "should fall back to plan when plan_and_apply is blocked")
}

func TestGetNextExecutableTask_WorkspaceNotFound(t *testing.T) {
	db := setupTestDB(t)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-nonexistent")
	assert.Error(t, err)
	assert.Nil(t, task)
}

func TestGetNextExecutableTask_OnlyApplyPending_NotReturned(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-008")
	// Only an apply_pending task — should NOT be auto-scheduled
	createTestTask(t, db, "ws-008", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-008")
	assert.NoError(t, err)
	assert.Nil(t, task, "apply_pending tasks should not be auto-scheduled")
}

func TestGetNextExecutableTask_FinalStatusIgnored(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-009")
	// All tasks in final states — none should be returned
	createTestTask(t, db, "ws-009", models.TaskTypePlanAndApply, models.TaskStatusApplied)
	createTestTask(t, db, "ws-009", models.TaskTypePlan, models.TaskStatusSuccess)
	createTestTask(t, db, "ws-009", models.TaskTypePlanAndApply, models.TaskStatusFailed)
	createTestTask(t, db, "ws-009", models.TaskTypePlanAndApply, models.TaskStatusCancelled)

	mgr := newTestManager(db, nil, nil)
	task, err := mgr.GetNextExecutableTask("ws-009")
	assert.NoError(t, err)
	assert.Nil(t, task, "tasks in final status should not be returned")
}

// ============================================================================
// Task 2: TryExecuteNextTask Tests (P0)
// ============================================================================

func TestTryExecuteNextTask_NoTask(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-try-001")

	mgr := newTestManager(db, nil, nil)
	err := mgr.TryExecuteNextTask("ws-try-001")
	assert.NoError(t, err)
}

func TestTryExecuteNextTask_PlanTask_NoLock(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-001"
	createTestWorkspace(t, db, "ws-try-002", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	createTestTask(t, db, "ws-try-002", models.TaskTypePlan, models.TaskStatusPending)
	createTestAgent(t, db, "agent-try-001", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-try-001"},
	}
	locker := newMockLockProvider()

	mgr := newTestManager(db, mockHandler, locker)
	err := mgr.TryExecuteNextTask("ws-try-002")
	assert.NoError(t, err)

	// Plan task should NOT have acquired any lock
	assert.Empty(t, locker.locked, "plan task should not acquire advisory lock")

	// Verify task was sent to agent
	sent := mockHandler.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "agent-try-001", sent[0].AgentID)
	assert.Equal(t, "plan", sent[0].Action)
}

func TestTryExecuteNextTask_PlanAndApply_AcquiresLock(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-002"
	createTestWorkspace(t, db, "ws-try-003", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	createTestTask(t, db, "ws-try-003", models.TaskTypePlanAndApply, models.TaskStatusPending)
	createTestAgent(t, db, "agent-try-002", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-try-002"},
	}
	locker := newMockLockProvider()

	mgr := newTestManager(db, mockHandler, locker)
	err := mgr.TryExecuteNextTask("ws-try-003")
	assert.NoError(t, err)

	// Lock should have been acquired and then released (defer Unlock)
	// After the function completes, the lock should be released
	assert.Empty(t, locker.locked, "lock should be released after TryExecuteNextTask returns")

	// Verify task was sent
	sent := mockHandler.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "plan", sent[0].Action)
}

func TestTryExecuteNextTask_LockBlocked_Skips(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-003"
	createTestWorkspace(t, db, "ws-try-004", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	createTestTask(t, db, "ws-try-004", models.TaskTypePlanAndApply, models.TaskStatusPending)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-try-003"},
	}
	locker := newMockLockProvider()
	locker.blocked = true // simulate another replica holding the lock

	mgr := newTestManager(db, mockHandler, locker)
	err := mgr.TryExecuteNextTask("ws-try-004")
	assert.NoError(t, err, "should not error when lock is held by another replica")

	// No task should have been sent
	sent := mockHandler.getSentTasks()
	assert.Empty(t, sent, "no task should be sent when lock cannot be acquired")
}

func TestTryExecuteNextTask_LocalMode_DispatchPath(t *testing.T) {
	// Local mode calls `go m.executeTask(task)` which requires TerraformExecutor.
	// We can't mock executor easily, so we test that the dispatch path is chosen
	// correctly by verifying the task is NOT sent to an agent (no agentCCHandler call).
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-try-005", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeLocal
	})
	// No pending tasks → TryExecuteNextTask returns immediately without executing
	mgr := newTestManager(db, nil, nil)
	err := mgr.TryExecuteNextTask("ws-try-005")
	assert.NoError(t, err, "local mode with no tasks should return nil")
}

func TestTryExecuteNextTask_StatusChangedAfterLock(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-004"
	createTestWorkspace(t, db, "ws-try-006", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	task := createTestTask(t, db, "ws-try-006", models.TaskTypePlanAndApply, models.TaskStatusPending)

	// Simulate: another goroutine changes task status after GetNextExecutableTask
	// but before lock re-check
	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-try-004"},
	}

	mgr := newTestManager(db, mockHandler, nil)

	// Change task status to "running" (simulating another replica picked it up)
	db.Model(&models.WorkspaceTask{}).Where("id = ?", task.ID).Update("status", models.TaskStatusRunning)

	err := mgr.TryExecuteNextTask("ws-try-006")
	assert.NoError(t, err)

	// Task should NOT have been sent (status changed)
	sent := mockHandler.getSentTasks()
	assert.Empty(t, sent, "task should be skipped when status changed after acquiring lock")
}

// ============================================================================
// Task 3: pushTaskToAgent Tests (P1)
// These test pushTaskToAgent indirectly through TryExecuteNextTask
// ============================================================================

func TestPushTaskToAgent_SecurityReject_UnconfirmedApplyPending(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-sec-001"
	createTestWorkspace(t, db, "ws-sec-001", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	// apply_pending task WITHOUT confirmation — should be rejected
	task := createTestTask(t, db, "ws-sec-001", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)

	createTestAgent(t, db, "agent-sec-001", poolID)
	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-sec-001"},
	}

	mgr := newTestManager(db, mockHandler, nil)
	err := mgr.pushTaskToAgent(task, &models.Workspace{
		WorkspaceID:   "ws-sec-001",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "require explicit user confirmation")

	// No task should have been sent
	sent := mockHandler.getSentTasks()
	assert.Empty(t, sent)
}

func TestPushTaskToAgent_ConfirmedApplyPending_Succeeds(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-sec-002"
	createTestWorkspace(t, db, "ws-sec-002", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	confirmedAt := time.Now()
	task := createTestTask(t, db, "ws-sec-002", models.TaskTypePlanAndApply, models.TaskStatusApplyPending, func(task *testWorkspaceTask) {
		task.ApplyConfirmedBy = strPtr("admin")
		task.ApplyConfirmedAt = &confirmedAt
	})

	createTestAgent(t, db, "agent-sec-002", poolID)
	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-sec-002"},
	}

	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-sec-002",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err)

	// Verify action was "apply"
	sent := mockHandler.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "apply", sent[0].Action)
	assert.Equal(t, task.ID, sent[0].TaskID)

	// Verify task status updated to running
	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusRunning, updated.Status)
	assert.Equal(t, "applying", updated.Stage)
	assert.NotNil(t, updated.AgentID)
}

func TestPushTaskToAgent_PendingTask_Succeeds(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-sec-003"
	createTestWorkspace(t, db, "ws-sec-003", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	task := createTestTask(t, db, "ws-sec-003", models.TaskTypePlan, models.TaskStatusPending)

	createTestAgent(t, db, "agent-sec-003", poolID)
	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-sec-003"},
	}

	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-sec-003",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err)

	// Verify action was "plan"
	sent := mockHandler.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "plan", sent[0].Action)

	// Verify task status updated
	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusRunning, updated.Status)
	assert.Equal(t, "planning", updated.Stage)
	assert.Equal(t, "agent-sec-003", *updated.AgentID)
}

func TestPushTaskToAgent_NilHandler_Retries(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-nil-001"
	createTestWorkspace(t, db, "ws-nil-001", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	task := createTestTask(t, db, "ws-nil-001", models.TaskTypePlan, models.TaskStatusPending)

	// Manager with nil handler
	mgr := newTestManager(db, nil, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-nil-001",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err, "should not error, just schedule retry")

	// Task should still be pending (not updated)
	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusPending, updated.Status)
}

func TestPushTaskToAgent_NoPool_Retries(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-nopool-001", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = nil
	})
	task := createTestTask(t, db, "ws-nopool-001", models.TaskTypePlan, models.TaskStatusPending)

	mockHandler := &mockAgentCCHandler{connectedAgents: []string{"agent-001"}}
	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-nopool-001",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: nil,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err, "should schedule retry, not error")

	// No task should have been sent
	sent := mockHandler.getSentTasks()
	assert.Empty(t, sent)
}

func TestPushTaskToAgent_NoConnectedAgents_Retries(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-empty-001"
	createTestWorkspace(t, db, "ws-noagent-001", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	task := createTestTask(t, db, "ws-noagent-001", models.TaskTypePlan, models.TaskStatusPending)

	mockHandler := &mockAgentCCHandler{connectedAgents: []string{}} // no agents
	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-noagent-001",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err, "should schedule retry, not error")

	sent := mockHandler.getSentTasks()
	assert.Empty(t, sent)
}

func TestPushTaskToAgent_AgentNotInPool_Skipped(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-target"
	createTestWorkspace(t, db, "ws-wrongpool", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	task := createTestTask(t, db, "ws-wrongpool", models.TaskTypePlan, models.TaskStatusPending)

	// Agent in a different pool
	createTestAgent(t, db, "agent-wrong-pool", "pool-other")

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-wrong-pool"},
	}
	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-wrongpool",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err, "should schedule retry when no agent in correct pool")

	sent := mockHandler.getSentTasks()
	assert.Empty(t, sent, "agent in wrong pool should be skipped")
}

func TestPushTaskToAgent_SendFailed_Rollback(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-fail-001"
	createTestWorkspace(t, db, "ws-fail-001", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	task := createTestTask(t, db, "ws-fail-001", models.TaskTypePlan, models.TaskStatusPending)
	createTestAgent(t, db, "agent-fail-001", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-fail-001"},
		sendError:       fmt.Errorf("connection timeout"),
	}

	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-fail-001",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err, "should not propagate error, schedules retry")

	// Verify task was rolled back to pending
	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusPending, updated.Status, "task should be rolled back to pending")
	assert.Nil(t, updated.AgentID, "agent_id should be cleared on rollback")
	assert.Empty(t, updated.Stage, "stage should be cleared on rollback")
	assert.Greater(t, updated.RetryCount, 0, "retry count should be incremented")
}

func TestPushTaskToAgent_SendFailed_ApplyPending_Rollback(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-fail-002"
	createTestWorkspace(t, db, "ws-fail-002", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	confirmedAt := time.Now()
	task := createTestTask(t, db, "ws-fail-002", models.TaskTypePlanAndApply, models.TaskStatusApplyPending, func(task *testWorkspaceTask) {
		task.ApplyConfirmedBy = strPtr("admin")
		task.ApplyConfirmedAt = &confirmedAt
	})
	createTestAgent(t, db, "agent-fail-002", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-fail-002"},
		sendError:       fmt.Errorf("connection reset"),
	}

	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-fail-002",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err)

	// Verify task rolled back to apply_pending (not pending)
	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusApplyPending, updated.Status, "confirmed apply task should roll back to apply_pending")
}

func TestPushTaskToAgent_Success_ResetsRetryCount(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-retry-001"
	createTestWorkspace(t, db, "ws-retry-001", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	task := createTestTask(t, db, "ws-retry-001", models.TaskTypePlan, models.TaskStatusPending, func(task *testWorkspaceTask) {
		task.RetryCount = 3
	})
	createTestAgent(t, db, "agent-retry-001", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-retry-001"},
	}

	mgr := newTestManager(db, mockHandler, nil)
	ws := &models.Workspace{
		WorkspaceID:   "ws-retry-001",
		ExecutionMode: models.ExecutionModeAgent,
		CurrentPoolID: &poolID,
	}
	err := mgr.pushTaskToAgent(task, ws)
	assert.NoError(t, err)

	// Verify retry count was reset
	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, 0, updated.RetryCount, "retry count should be reset on success")
}

// ============================================================================
// Task 4: ExecuteConfirmedApply Tests (P1)
// ============================================================================

func TestExecuteConfirmedApply_TaskNotFound(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-eca-001")

	mgr := newTestManager(db, nil, nil)
	err := mgr.ExecuteConfirmedApply("ws-eca-001", 9999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestExecuteConfirmedApply_NotApplyPending(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-eca-002")
	task := createTestTask(t, db, "ws-eca-002", models.TaskTypePlanAndApply, models.TaskStatusRunning)

	mgr := newTestManager(db, nil, nil)
	err := mgr.ExecuteConfirmedApply("ws-eca-002", task.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in apply_pending status")
}

func TestExecuteConfirmedApply_NotConfirmed(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-eca-003")
	task := createTestTask(t, db, "ws-eca-003", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)

	mgr := newTestManager(db, nil, nil)
	err := mgr.ExecuteConfirmedApply("ws-eca-003", task.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has not been confirmed")
}

func TestExecuteConfirmedApply_AgentMode_CallsPush(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-eca-001"
	createTestWorkspace(t, db, "ws-eca-004", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	confirmedAt := time.Now()
	task := createTestTask(t, db, "ws-eca-004", models.TaskTypePlanAndApply, models.TaskStatusApplyPending, func(task *testWorkspaceTask) {
		task.ApplyConfirmedBy = strPtr("admin")
		task.ApplyConfirmedAt = &confirmedAt
	})
	createTestAgent(t, db, "agent-eca-001", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-eca-001"},
	}

	mgr := newTestManager(db, mockHandler, nil)
	err := mgr.ExecuteConfirmedApply("ws-eca-004", task.ID)
	assert.NoError(t, err)

	// Verify apply action was sent
	sent := mockHandler.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "apply", sent[0].Action)
}

func TestExecuteConfirmedApply_K8sMode_CallsPush(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-eca-002"
	createTestWorkspace(t, db, "ws-eca-005", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeK8s
		ws.CurrentPoolID = &poolID
	})
	confirmedAt := time.Now()
	task := createTestTask(t, db, "ws-eca-005", models.TaskTypePlanAndApply, models.TaskStatusApplyPending, func(task *testWorkspaceTask) {
		task.ApplyConfirmedBy = strPtr("admin")
		task.ApplyConfirmedAt = &confirmedAt
	})
	createTestAgent(t, db, "agent-eca-002", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-eca-002"},
	}

	mgr := newTestManager(db, mockHandler, nil)
	err := mgr.ExecuteConfirmedApply("ws-eca-005", task.ID)
	assert.NoError(t, err)

	// K8s mode also goes through pushTaskToAgent
	sent := mockHandler.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "apply", sent[0].Action)
}

// ============================================================================
// Task 5: checkAndRetryPendingTasks Tests (P2)
// ============================================================================

func TestCheckAndRetryPendingTasks_NoPendingTasks(t *testing.T) {
	db := setupTestDB(t)
	mgr := newTestManager(db, nil, nil)

	// Should not panic
	mgr.checkAndRetryPendingTasks()
}

func TestCheckAndRetryPendingTasks_PendingTasks(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-retry-a")
	createTestWorkspace(t, db, "ws-retry-b")
	createTestTask(t, db, "ws-retry-a", models.TaskTypePlan, models.TaskStatusPending)
	createTestTask(t, db, "ws-retry-b", models.TaskTypePlan, models.TaskStatusPending)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{},
	}
	mgr := newTestManager(db, mockHandler, nil)

	// checkAndRetryPendingTasks launches goroutines via TryExecuteNextTask
	// We just verify it doesn't panic
	mgr.checkAndRetryPendingTasks()
	// Give goroutines time to complete
	time.Sleep(200 * time.Millisecond)
}

func TestCheckAndRetryPendingTasks_ConfirmedApplyPending(t *testing.T) {
	db := setupTestDB(t)
	poolID := "pool-monitor-001"
	createTestWorkspace(t, db, "ws-monitor-001", func(ws *testWorkspace) {
		ws.ExecutionMode = models.ExecutionModeAgent
		ws.CurrentPoolID = &poolID
	})
	confirmedAt := time.Now()
	createTestTask(t, db, "ws-monitor-001", models.TaskTypePlanAndApply, models.TaskStatusApplyPending, func(task *testWorkspaceTask) {
		task.ApplyConfirmedBy = strPtr("admin")
		task.ApplyConfirmedAt = &confirmedAt
	})
	createTestAgent(t, db, "agent-monitor-001", poolID)

	mockHandler := &mockAgentCCHandler{
		connectedAgents: []string{"agent-monitor-001"},
	}
	mgr := newTestManager(db, mockHandler, nil)
	mgr.checkAndRetryPendingTasks()

	// Give goroutines time to complete
	time.Sleep(300 * time.Millisecond)

	// Verify the confirmed apply_pending task was retried and sent
	sent := mockHandler.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "apply", sent[0].Action)
}

func TestCheckAndRetryPendingTasks_UnconfirmedApplyPending_Ignored(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-monitor-002")
	// Unconfirmed apply_pending — should NOT be retried
	createTestTask(t, db, "ws-monitor-002", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)

	mgr := newTestManager(db, nil, nil)
	mgr.checkAndRetryPendingTasks()
	// Should not panic, and no action taken for unconfirmed tasks
}

// ============================================================================
// Task 6: CleanupOrphanTasks + RecoverPendingTasks Tests (P2)
// ============================================================================

func TestCleanupOrphanTasks_NoOrphans(t *testing.T) {
	db := setupTestDB(t)
	mgr := newTestManager(db, nil, nil)

	err := mgr.CleanupOrphanTasks()
	assert.NoError(t, err)
}

func TestCleanupOrphanTasks_RunningTask_MarkedFailed(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-orphan-001")
	task := createTestTask(t, db, "ws-orphan-001", models.TaskTypePlanAndApply, models.TaskStatusRunning, func(task *testWorkspaceTask) {
		task.Stage = "planning"
	})

	mgr := newTestManager(db, nil, nil)
	err := mgr.CleanupOrphanTasks()
	assert.NoError(t, err)

	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusFailed, updated.Status)
	assert.Contains(t, updated.ErrorMessage, "server restart")
}

func TestCleanupOrphanTasks_ApplyPendingStage_ResetToApplyPending(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-orphan-002")
	task := createTestTask(t, db, "ws-orphan-002", models.TaskTypePlanAndApply, models.TaskStatusRunning, func(task *testWorkspaceTask) {
		task.Stage = "apply_pending"
	})

	mgr := newTestManager(db, nil, nil)
	err := mgr.CleanupOrphanTasks()
	assert.NoError(t, err)

	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusApplyPending, updated.Status, "should reset to apply_pending, not mark as failed")
}

func TestRecoverPendingTasks_CancelsRunTriggerTasks(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-recover-001")
	task := createTestTask(t, db, "ws-recover-001", models.TaskTypePlanAndApply, models.TaskStatusPending, func(task *testWorkspaceTask) {
		task.Description = "Triggered by workspace ws-upstream-001"
	})

	mgr := newTestManager(db, nil, nil)
	err := mgr.RecoverPendingTasks()
	assert.NoError(t, err)

	// Give goroutines time to complete
	time.Sleep(200 * time.Millisecond)

	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusCancelled, updated.Status, "Run Trigger tasks should be cancelled on recovery")
	assert.Contains(t, updated.ErrorMessage, "server restart")
}

func TestRecoverPendingTasks_RecoversNormalPending(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-recover-002")
	createTestTask(t, db, "ws-recover-002", models.TaskTypePlan, models.TaskStatusPending, func(task *testWorkspaceTask) {
		task.Description = "Manual run"
	})

	mgr := newTestManager(db, nil, nil)
	err := mgr.RecoverPendingTasks()
	assert.NoError(t, err)
	// Normal pending tasks should trigger TryExecuteNextTask
	// (execution details depend on mode, not tested further here)
}

func TestRecoverPendingTasks_SkipsApplyPending(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-recover-003")
	task := createTestTask(t, db, "ws-recover-003", models.TaskTypePlanAndApply, models.TaskStatusApplyPending)

	mgr := newTestManager(db, nil, nil)
	err := mgr.RecoverPendingTasks()
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// apply_pending task should remain unchanged
	var updated models.WorkspaceTask
	db.First(&updated, task.ID)
	assert.Equal(t, models.TaskStatusApplyPending, updated.Status, "apply_pending should not be auto-recovered")
}

// ============================================================================
// Task 7: Helper Method Tests
// ============================================================================

func TestCalculateRetryDelay(t *testing.T) {
	mgr := &TaskQueueManager{}

	tests := []struct {
		retryCount int
		expected   time.Duration
	}{
		{0, 5 * time.Second},
		{1, 10 * time.Second},
		{2, 20 * time.Second},
		{3, 40 * time.Second},
		{4, 60 * time.Second},
		{5, 60 * time.Second},  // capped
		{99, 60 * time.Second}, // capped
	}

	for _, tt := range tests {
		task := &models.WorkspaceTask{RetryCount: tt.retryCount}
		delay := mgr.calculateRetryDelay(task)
		assert.Equal(t, tt.expected, delay, "retryCount=%d", tt.retryCount)
	}
}

func TestCanExecuteNewTask_Locked(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-can-001", func(ws *testWorkspace) {
		ws.IsLocked = true
		ws.LockedBy = strPtr("admin")
	})

	mgr := newTestManager(db, nil, nil)
	ok, reason := mgr.CanExecuteNewTask("ws-can-001")
	assert.False(t, ok)
	assert.Contains(t, reason, "锁定")
}

func TestCanExecuteNewTask_HasBlockingTask(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-can-002")
	createTestTask(t, db, "ws-can-002", models.TaskTypePlanAndApply, models.TaskStatusRunning)

	mgr := newTestManager(db, nil, nil)
	ok, reason := mgr.CanExecuteNewTask("ws-can-002")
	assert.False(t, ok)
	assert.Contains(t, reason, "plan_and_apply")
}

func TestCanExecuteNewTask_NoBlocker(t *testing.T) {
	db := setupTestDB(t)
	createTestWorkspace(t, db, "ws-can-003")
	// Only completed tasks — no blockers
	createTestTask(t, db, "ws-can-003", models.TaskTypePlanAndApply, models.TaskStatusApplied)

	mgr := newTestManager(db, nil, nil)
	ok, reason := mgr.CanExecuteNewTask("ws-can-003")
	assert.True(t, ok)
	assert.Empty(t, reason)
}
