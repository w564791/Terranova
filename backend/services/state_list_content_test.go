package services

import (
	"encoding/json"
	"testing"
	"time"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupStateListDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	// SQLite: store content as TEXT JSON; model uses JSONB which is []byte-compatible
	if err := db.Exec(`
CREATE TABLE workspace_state_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL,
  version INTEGER NOT NULL,
  checksum TEXT NOT NULL,
  size_bytes INTEGER,
  task_id INTEGER,
  created_by TEXT,
  created_at DATETIME,
  description TEXT,
  is_imported INTEGER DEFAULT 0,
  import_source TEXT,
  is_rollback INTEGER DEFAULT 0,
  rollback_from_version INTEGER,
  content TEXT NOT NULL,
  lineage TEXT,
  serial INTEGER DEFAULT 0,
  resource_count INTEGER DEFAULT 0,
  is_temp INTEGER DEFAULT 0
);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE users (
  user_id TEXT PRIMARY KEY,
  username TEXT
);`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestListStateVersions_OmitsContent(t *testing.T) {
	db := setupStateListDB(t)
	sensitive := `{"resources":[{"type":"aws_db_instance","instances":[{"attributes":{"password":"s3cret"}}]}]}`
	createdBy := "user-1"
	if err := db.Exec(`
INSERT INTO workspace_state_versions
  (workspace_id, version, checksum, size_bytes, created_by, created_at, description, content)
VALUES (?,?,?,?,?,?,?,?)`,
		"ws-a", 2, "abc", 100, createdBy, time.Now(), "v2", sensitive,
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
INSERT INTO workspace_state_versions
  (workspace_id, version, checksum, size_bytes, created_by, created_at, description, content)
VALUES (?,?,?,?,?,?,?,?)`,
		"ws-a", 1, "def", 50, createdBy, time.Now().Add(-time.Hour), "v1", `{"x":1}`,
	).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewStateService(db)
	versions, total, err := svc.ListStateVersions("ws-a", 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(versions) != 2 {
		t.Fatalf("total=%d len=%d", total, len(versions))
	}
	if versions[0].Version != 2 {
		t.Fatalf("order desc expected v2 first, got %d", versions[0].Version)
	}
	for _, v := range versions {
		if len(v.Content) != 0 {
			t.Fatalf("list must not load content (R-S02); version=%d content=%v", v.Version, v.Content)
		}
		if v.Checksum == "" {
			t.Fatalf("metadata should still load for version %d", v.Version)
		}
	}

	// GetStateVersion loads the row for SENSITIVE retrieve path (full SELECT, not list Select).
	// Note: SQLite may not round-trip JSONB map content the same as Postgres; the security
	// property under test is that *list* never selects content.
	full, err := svc.GetStateVersion("ws-a", 2)
	if err != nil {
		t.Fatal(err)
	}
	if full.Version != 2 || full.WorkspaceID != "ws-a" {
		t.Fatalf("GetStateVersion metadata: %+v", full)
	}
}

func TestListStateVersionsWithUsernames_OmitsContent(t *testing.T) {
	db := setupStateListDB(t)
	_ = db.Exec(`INSERT INTO users (user_id, username) VALUES ('user-1', 'alice')`)
	createdBy := "user-1"
	secret := `{"outputs":{"db_password":{"value":"p@ss"}}}`
	if err := db.Exec(`
INSERT INTO workspace_state_versions
  (workspace_id, version, checksum, size_bytes, created_by, created_at, description, content)
VALUES (?,?,?,?,?,?,?,?)`,
		"ws-b", 1, "chk", 10, createdBy, time.Now(), "d", secret,
	).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewStateService(db)
	rows, total, err := svc.ListStateVersionsWithUsernames("ws-b", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("total=%d len=%d", total, len(rows))
	}
	if len(rows[0].Content) != 0 {
		t.Fatalf("username list must omit content: %v", rows[0].Content)
	}
	if rows[0].CreatedByName != "alice" {
		t.Fatalf("username=%q", rows[0].CreatedByName)
	}

	// ensure JSON of list item wouldn't re-embed content accidentally when zero
	b, _ := json.Marshal(rows[0])
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	if c, ok := m["content"]; ok && c != nil {
		// empty JSONB may serialize as null or empty; never as the secret string
		if s, ok := c.(string); ok && s != "" && s != "null" {
			t.Fatalf("marshaled content leaked: %v", c)
		}
		if arr, ok := c.(map[string]interface{}); ok && len(arr) > 0 {
			t.Fatalf("marshaled content object leaked: %v", c)
		}
	}
}

// compile-time touch for model shape
var _ = models.WorkspaceStateVersion{}
