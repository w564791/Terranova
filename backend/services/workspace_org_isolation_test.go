package services

import (
	"testing"
	"time"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWorkspaceOrgTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Unique DSN per test to avoid shared-cache collisions
	dsn := "file:ws_org_" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// AutoMigrate full Workspace model for create path
	if err := db.AutoMigrate(&models.Workspace{}); err != nil {
		// Fall back to minimal tables if AutoMigrate fails on complex types
		t.Logf("AutoMigrate Workspace: %v (using minimal schema)", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			org_id INTEGER,
			name TEXT,
			description TEXT DEFAULT '',
			is_default INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS workspace_project_relations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT UNIQUE,
			project_id INTEGER,
			created_at DATETIME
		)`,
	}
	// Ensure workspaces table exists even if AutoMigrate partially failed
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT UNIQUE,
		name TEXT,
		description TEXT DEFAULT '',
		execution_mode TEXT DEFAULT 'local',
		state_backend TEXT DEFAULT 'local',
		state TEXT DEFAULT '',
		created_by TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("schema: %v\n%s", err, s)
		}
	}
	now := time.Now()
	_ = db.Exec(`INSERT INTO projects (id, org_id, name, is_default, created_at, updated_at) VALUES
		(1, 1, 'default', 1, ?, ?),
		(2, 2, 'default', 1, ?, ?)`, now, now, now, now)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name, state_backend, created_at, updated_at) VALUES
		(10, 'ws-a', 'wsA', 'local', ?, ?),
		(20, 'ws-b', 'wsB', 'local', ?, ?)`, now, now, now, now)
	_ = db.Exec(`INSERT INTO workspace_project_relations (workspace_id, project_id, created_at) VALUES
		('ws-a', 1, ?), ('ws-b', 2, ?)`, now, now)
	return db
}

func TestSearchWorkspacesInOrg_IsolatesTenants(t *testing.T) {
	db := setupWorkspaceOrgTestDB(t)
	svc := NewWorkspaceService(db)

	list1, total1, err := svc.SearchWorkspacesInOrg("", 1, 20, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total1 != 1 || len(list1) != 1 || list1[0].WorkspaceID != "ws-a" {
		t.Fatalf("org1 want only ws-a, got total=%d items=%+v", total1, list1)
	}

	list2, total2, err := svc.SearchWorkspacesInOrg("", 1, 20, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total2 != 1 || list2[0].WorkspaceID != "ws-b" {
		t.Fatalf("org2 want only ws-b, got total=%d items=%+v", total2, list2)
	}
}

func TestSearchWorkspacesInOrgAndWorkspaceIDs_NeverBroadensScopedList(t *testing.T) {
	db := setupWorkspaceOrgTestDB(t)
	svc := NewWorkspaceService(db)

	// A scoped Role allow-list only exposes its explicit workspace.
	list, total, err := svc.SearchWorkspacesInOrgAndWorkspaceIDs("", 1, 20, 1, 0, []string{"ws-a"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].WorkspaceID != "ws-a" {
		t.Fatalf("scoped list must contain only ws-a, got total=%d items=%+v", total, list)
	}

	// An allow-list from another tenant cannot override the org predicate.
	list, total, err = svc.SearchWorkspacesInOrgAndWorkspaceIDs("", 1, 20, 1, 0, []string{"ws-b"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(list) != 0 {
		t.Fatalf("cross-tenant ID must not be visible, got total=%d items=%+v", total, list)
	}

	// Empty-but-non-nil means no scoped access, not an omitted filter.
	list, total, err = svc.SearchWorkspacesInOrgAndWorkspaceIDs("", 1, 20, 1, 0, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(list) != 0 {
		t.Fatalf("empty scoped list must not fall back to all workspaces, got total=%d items=%+v", total, list)
	}
}

func TestEnsureWorkspaceInOrg_CrossTenantDenied(t *testing.T) {
	db := setupWorkspaceOrgTestDB(t)
	svc := NewWorkspaceService(db)

	if err := svc.EnsureWorkspaceInOrg("ws-a", 1); err != nil {
		t.Fatalf("same org should pass: %v", err)
	}
	if err := svc.EnsureWorkspaceInOrg("ws-b", 1); err == nil {
		t.Fatal("cross-tenant should fail")
	}
	if err := svc.EnsureWorkspaceInOrg("ws-a", 2); err == nil {
		t.Fatal("cross-tenant should fail")
	}
}

func TestCreateWorkspaceInOrg_BindsDefaultProject(t *testing.T) {
	db := setupWorkspaceOrgTestDB(t)
	svc := NewWorkspaceService(db)

	ws := &models.Workspace{
		Name:         "new-in-org1",
		StateBackend: "local",
		State:        models.WorkspaceStateCreated,
	}
	if err := svc.CreateWorkspaceInOrg(ws, 1, 0); err != nil {
		t.Fatalf("create: %v", err)
	}
	if ws.WorkspaceID == "" {
		t.Fatal("expected workspace_id")
	}
	var orgID uint
	if err := db.Raw(`
SELECT p.org_id FROM workspace_project_relations wpr
JOIN projects p ON p.id = wpr.project_id
WHERE wpr.workspace_id = ?`, ws.WorkspaceID).Scan(&orgID).Error; err != nil {
		t.Fatal(err)
	}
	if orgID != 1 {
		t.Fatalf("want org 1 binding, got %d", orgID)
	}

	// Cross-org project bind rejected
	ws2 := &models.Workspace{Name: "bad-bind", StateBackend: "local", State: models.WorkspaceStateCreated}
	if err := svc.CreateWorkspaceInOrg(ws2, 1, 2); err == nil {
		t.Fatal("expected reject binding project of other org")
	}
}
