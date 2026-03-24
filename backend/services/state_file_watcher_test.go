package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"iac-platform/internal/database"
	"iac-platform/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupWatcherTestDB creates an in-memory SQLite DB with the tables needed for watcher tests.
func setupWatcherTestDB(t *testing.T) (*LocalDataAccessor, *gorm.DB) {
	t.Helper()
	db := setupTestDB(t)

	// 注册全局 temp 过滤回调（和生产环境一致）
	database.RegisterStateVersionTempFilter(db)

	sqlDB, err := db.DB()
	require.NoError(t, err)

	_, err = sqlDB.Exec(`CREATE TABLE IF NOT EXISTS workspace_state_versions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL,
		created_by TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		content TEXT NOT NULL DEFAULT '{}',
		version INTEGER NOT NULL DEFAULT 0,
		checksum TEXT NOT NULL DEFAULT '',
		size_bytes INTEGER DEFAULT 0,
		lineage TEXT DEFAULT '',
		serial INTEGER DEFAULT 0,
		is_imported INTEGER DEFAULT 0,
		import_source TEXT DEFAULT '',
		is_rollback INTEGER DEFAULT 0,
		rollback_from_version INTEGER,
		description TEXT DEFAULT '',
		task_id INTEGER,
		resource_count INTEGER DEFAULT 0,
		is_temp INTEGER DEFAULT 0
	)`)
	require.NoError(t, err)

	return NewLocalDataAccessor(db), db
}

func writeStateFile(t *testing.T, dir string, resources int) {
	t.Helper()
	state := map[string]interface{}{
		"version":           4,
		"terraform_version": "1.5.0",
		"serial":            resources,
		"lineage":           "test-lineage",
		"resources":         make([]interface{}, resources),
	}
	data, err := json.Marshal(state)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "terraform.tfstate"), data, 0644)
	require.NoError(t, err)
}

func TestStateFileWatcher_StartStop(t *testing.T) {
	dir := t.TempDir()
	accessor, _ := setupWatcherTestDB(t)

	taskID := uint(1)
	createdBy := "test-user"
	watcher := NewStateFileWatcher(dir, "ws-test", taskID, &createdBy, accessor)

	err := watcher.Start()
	require.NoError(t, err)

	watcher.Stop()
	assert.Equal(t, uint(0), watcher.GetTempRecordID(), "no temp record without file changes")
}

func TestStateFileWatcher_DetectsFileChange(t *testing.T) {
	dir := t.TempDir()
	accessor, _ := setupWatcherTestDB(t)

	taskID := uint(1)
	createdBy := "test-user"
	watcher := NewStateFileWatcher(dir, "ws-test", taskID, &createdBy, accessor)

	err := watcher.Start()
	require.NoError(t, err)

	writeStateFile(t, dir, 3)
	time.Sleep(500 * time.Millisecond)

	watcher.Stop()
	assert.Greater(t, watcher.GetTempRecordID(), uint(0), "should have created a temp record")
}

func TestStateFileWatcher_UpsertSameRecord(t *testing.T) {
	dir := t.TempDir()
	accessor, _ := setupWatcherTestDB(t)

	taskID := uint(1)
	createdBy := "test-user"
	watcher := NewStateFileWatcher(dir, "ws-test", taskID, &createdBy, accessor)

	err := watcher.Start()
	require.NoError(t, err)

	writeStateFile(t, dir, 3)
	time.Sleep(500 * time.Millisecond)
	firstID := watcher.GetTempRecordID()

	writeStateFile(t, dir, 5)
	time.Sleep(500 * time.Millisecond)

	watcher.Stop()
	assert.Equal(t, firstID, watcher.GetTempRecordID(), "should reuse the same temp record")
}

func TestStateFileWatcher_ChecksumDedup(t *testing.T) {
	dir := t.TempDir()
	accessor, _ := setupWatcherTestDB(t)

	taskID := uint(1)
	createdBy := "test-user"
	watcher := NewStateFileWatcher(dir, "ws-test", taskID, &createdBy, accessor)

	err := watcher.Start()
	require.NoError(t, err)

	writeStateFile(t, dir, 3)
	time.Sleep(500 * time.Millisecond)

	writeStateFile(t, dir, 3) // same content
	time.Sleep(500 * time.Millisecond)

	watcher.Stop()
	assert.Greater(t, watcher.GetTempRecordID(), uint(0))
}

func TestStateFileWatcher_Promote(t *testing.T) {
	dir := t.TempDir()
	accessor, db := setupWatcherTestDB(t)

	// Create workspace record for promote to update tf_state
	db.Exec(`INSERT INTO workspaces (workspace_id, name, tf_state) VALUES ('ws-test', 'test', '{}')`)

	taskID := uint(1)
	createdBy := "test-user"
	watcher := NewStateFileWatcher(dir, "ws-test", taskID, &createdBy, accessor)

	err := watcher.Start()
	require.NoError(t, err)

	writeStateFile(t, dir, 3)
	time.Sleep(500 * time.Millisecond)

	watcher.Stop()
	require.Greater(t, watcher.GetTempRecordID(), uint(0))

	// Promote
	err = watcher.Promote()
	require.NoError(t, err)

	// Verify record is no longer temp
	var record models.WorkspaceStateVersion
	err = db.First(&record, watcher.GetTempRecordID()).Error
	require.NoError(t, err)
	assert.False(t, record.IsTemp, "promoted record should not be temp")
}

func TestStateFileWatcher_PromoteNoRecord(t *testing.T) {
	dir := t.TempDir()
	accessor, _ := setupWatcherTestDB(t)

	taskID := uint(1)
	createdBy := "test-user"
	watcher := NewStateFileWatcher(dir, "ws-test", taskID, &createdBy, accessor)

	err := watcher.Promote()
	assert.NoError(t, err, "promote with no temp record should be a no-op")
}

func TestStateFileWatcher_FinalPushOnStop(t *testing.T) {
	dir := t.TempDir()
	accessor, _ := setupWatcherTestDB(t)

	taskID := uint(1)
	createdBy := "test-user"
	watcher := NewStateFileWatcher(dir, "ws-test", taskID, &createdBy, accessor)

	// Write file before starting watcher
	writeStateFile(t, dir, 3)

	err := watcher.Start()
	require.NoError(t, err)

	// Write new file and immediately stop - final push should catch it
	writeStateFile(t, dir, 7)

	watcher.Stop()

	assert.Greater(t, watcher.GetTempRecordID(), uint(0),
		"final push on stop should have captured the state")
}
