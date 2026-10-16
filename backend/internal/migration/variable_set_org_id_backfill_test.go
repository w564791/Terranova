package migration

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupBackfillDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`,
		`CREATE TABLE variable_sets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			varset_id TEXT, name TEXT, scope TEXT, org_id INTEGER,
			is_deleted INTEGER DEFAULT 0, created_by TEXT
		)`,
		`CREATE TABLE varset_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			varset_id TEXT, scope_type TEXT, project_id INTEGER, workspace_id TEXT
		)`,
		`CREATE TABLE user_organizations (
			user_id TEXT, org_id INTEGER, joined_at DATETIME
		)`,
		`CREATE TABLE iam_roles (id INTEGER PRIMARY KEY, name TEXT, is_system INTEGER, org_id INTEGER)`,
		`CREATE TABLE permission_definitions (
			id TEXT PRIMARY KEY, resource_type TEXT, scope_level TEXT
		)`,
		`CREATE TABLE iam_role_policies (
			role_id INTEGER, permission_id TEXT, permission_level TEXT, scope_type TEXT
		)`,
		`CREATE TABLE iam_user_roles (
			user_id TEXT, role_id INTEGER, scope_type TEXT, scope_id INTEGER, expires_at DATETIME
		)`,
		`INSERT INTO projects VALUES (10, 1), (20, 1), (30, 2)`,
		`INSERT INTO workspace_project_relations VALUES ('ws-a', 10), ('ws-b', 20), ('ws-other', 30)`,
		`INSERT INTO permission_definitions VALUES ('pd-vs', 'VARIABLE_SETS', 'ORGANIZATION')`,
		`INSERT INTO iam_roles VALUES (1, 'org_admin', 1, 0)`,
		`INSERT INTO iam_role_policies VALUES (1, 'pd-vs', 'ADMIN', 'ORGANIZATION')`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	return db
}

func orgOf(t *testing.T, db *gorm.DB, varsetID string) *int64 {
	t.Helper()
	var org *int64
	if err := db.Raw(`SELECT org_id FROM variable_sets WHERE varset_id = ?`, varsetID).Scan(&org).Error; err != nil {
		t.Fatal(err)
	}
	return org
}

func TestBackfillSingleOrgAssignment(t *testing.T) {
	db := setupBackfillDB(t)
	db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES ('vs-1', 'a', 'specific', NULL, 'u1')`)
	db.Exec(`INSERT INTO varset_assignments (varset_id, scope_type, workspace_id) VALUES ('vs-1', 'workspace', 'ws-a')`)
	if err := backfillVariableSetOrgID(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	got := orgOf(t, db, "vs-1")
	if got == nil || *got != 1 {
		t.Fatalf("org_id = %v, want 1", got)
	}
}

func TestBackfillCrossOrgKeepsMajorityDropsOthers(t *testing.T) {
	db := setupBackfillDB(t)
	db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES ('vs-x', 'x', 'specific', NULL, 'u1')`)
	db.Exec(`INSERT INTO varset_assignments (varset_id, scope_type, workspace_id) VALUES
		('vs-x', 'workspace', 'ws-a'), ('vs-x', 'workspace', 'ws-b'), ('vs-x', 'workspace', 'ws-other')`)
	if err := backfillVariableSetOrgID(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	got := orgOf(t, db, "vs-x")
	if got == nil || *got != 1 {
		t.Fatalf("org_id = %v, want 1 (majority)", got)
	}
	var n int64
	db.Raw(`SELECT COUNT(*) FROM varset_assignments WHERE varset_id = 'vs-x'`).Scan(&n)
	if n != 2 {
		t.Fatalf("remaining mounts = %d, want 2 (org1 only)", n)
	}
	var other int64
	db.Raw(`SELECT COUNT(*) FROM varset_assignments a
		JOIN workspace_project_relations wpr ON wpr.workspace_id = a.workspace_id
		JOIN projects p ON p.id = wpr.project_id
		WHERE a.varset_id = 'vs-x' AND p.org_id = 2`).Scan(&other)
	if other != 0 {
		t.Fatalf("org2 mounts left: %d", other)
	}
}

func TestBackfillUnassignedCreatorOrg(t *testing.T) {
	db := setupBackfillDB(t)
	db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES ('vs-u', 'u', 'specific', NULL, 'u1')`)
	db.Exec(`INSERT INTO user_organizations VALUES ('u1', 2, '2020-01-02'), ('u1', 1, '2020-01-01')`)
	db.Exec(`INSERT INTO iam_user_roles (user_id, role_id, scope_type, scope_id) VALUES ('u1', 1, 'ORGANIZATION', 1)`)
	if err := backfillVariableSetOrgID(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	got := orgOf(t, db, "vs-u")
	if got == nil || *got != 1 {
		t.Fatalf("org_id = %v, want 1 (earliest join with WRITE)", got)
	}
}

func TestBackfillUnassignedNoWriteLeavesNull(t *testing.T) {
	db := setupBackfillDB(t)
	db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES ('vs-n', 'n', 'specific', NULL, 'u9')`)
	db.Exec(`INSERT INTO user_organizations VALUES ('u9', 1, '2020-01-01')`)
	// no iam_user_roles for u9
	if err := backfillVariableSetOrgID(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := orgOf(t, db, "vs-n"); got != nil {
		t.Fatalf("org_id = %v, want NULL", got)
	}
}

func TestBackfillGlobalStaysNull(t *testing.T) {
	db := setupBackfillDB(t)
	db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES ('vs-g', 'g', 'global', NULL, 'u1')`)
	db.Exec(`INSERT INTO varset_assignments (varset_id, scope_type, workspace_id) VALUES ('vs-g', 'workspace', 'ws-a')`)
	if err := backfillVariableSetOrgID(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := orgOf(t, db, "vs-g"); got != nil {
		t.Fatalf("global org_id = %v, want NULL", got)
	}
}

func TestBackfillIdempotent(t *testing.T) {
	db := setupBackfillDB(t)
	db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES ('vs-1', 'a', 'specific', NULL, 'u1')`)
	db.Exec(`INSERT INTO varset_assignments (varset_id, scope_type, workspace_id) VALUES ('vs-1', 'workspace', 'ws-a')`)
	ctx := context.Background()
	if err := backfillVariableSetOrgID(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := backfillVariableSetOrgID(ctx, db); err != nil {
		t.Fatal(err)
	}
	got := orgOf(t, db, "vs-1")
	if got == nil || *got != 1 {
		t.Fatalf("org_id = %v after second run", got)
	}
}
