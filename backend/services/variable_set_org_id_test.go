package services

import (
	"testing"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupVarsetOrgDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`,
		`CREATE TABLE variable_sets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			varset_id TEXT UNIQUE,
			name TEXT,
			description TEXT,
			scope TEXT,
			org_id INTEGER,
			is_deleted INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME,
			created_by TEXT
		)`,
		`CREATE TABLE varset_assignments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			varset_id TEXT,
			scope_type TEXT,
			project_id INTEGER,
			workspace_id TEXT,
			attached_at DATETIME,
			attached_by TEXT
		)`,
		`INSERT INTO projects (id, org_id) VALUES (10, 1), (30, 2)`,
		`INSERT INTO workspace_project_relations VALUES ('ws-a', 10), ('ws-other', 30)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	return db
}

func TestCreateSpecificSetsOrgID(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	org := uint(1)
	vs, err := svc.Create("alpha", "d", "specific", vsStrPtr("u1"), &org)
	if err != nil {
		t.Fatal(err)
	}
	if vs.OrgID == nil || *vs.OrgID != 1 {
		t.Fatalf("org_id = %v, want 1", vs.OrgID)
	}
}

func TestCreateGlobalKeepsOrgIDNil(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	vs, err := svc.Create("g", "d", "global", vsStrPtr("u1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if vs.OrgID != nil {
		t.Fatalf("global org_id = %v, want nil", vs.OrgID)
	}
}

func TestListForOrgPrefersOrgIDColumn(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	org1, org2 := uint(1), uint(2)
	a, _ := svc.Create("in-1", "", "specific", vsStrPtr("u1"), &org1)
	b, _ := svc.Create("in-2", "", "specific", vsStrPtr("u2"), &org2)
	_ = db.Exec(`INSERT INTO varset_assignments (varset_id, scope_type, workspace_id) VALUES (?, 'workspace', 'ws-a')`, b.VarsetID)

	list, err := svc.ListForOrg("", 1, "u1")
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, vs := range list {
		ids[vs.VarsetID] = true
	}
	if !ids[a.VarsetID] {
		t.Fatal("org1 varset missing")
	}
	if ids[b.VarsetID] {
		t.Fatal("org2 varset must not appear via assignment join when org_id is set")
	}
}

func TestCrossOrgNotVisibleOrWritable(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	org2 := uint(2)
	vs, _ := svc.Create("foreign", "", "specific", vsStrPtr("u2"), &org2)

	ok, err := svc.VarsetVisibleInOrg(vs.VarsetID, 1, "u1")
	if err != nil || ok {
		t.Fatalf("visible in other org = %v, %v; want false", ok, err)
	}
	w, err := svc.VarsetWritableInOrg(vs.VarsetID, 1, "u1", false)
	if err != nil || w {
		t.Fatalf("writable in other org = %v, %v; want false", w, err)
	}
}

func TestMountRequiresMatchingOrg(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	org1 := uint(1)
	vs, _ := svc.Create("mine", "", "specific", vsStrPtr("u1"), &org1)
	ws := "ws-other"
	_, err := svc.CreateAssignment(vs.VarsetID, "workspace", nil, &ws, vsStrPtr("u1"), 2)
	if err == nil || err.Error() != "assignment target not found" {
		t.Fatalf("cross-org mount err = %v, want assignment target not found", err)
	}
	wsA := "ws-a"
	if _, err := svc.CreateAssignment(vs.VarsetID, "workspace", nil, &wsA, vsStrPtr("u1"), 1); err != nil {
		t.Fatalf("same-org mount: %v", err)
	}
}

func TestMountOrphanSetsOrgID(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	if err := db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by, is_deleted)
		VALUES ('vs-orphan', 'orphan', 'specific', NULL, 'u1', 0)`).Error; err != nil {
		t.Fatal(err)
	}
	wsA := "ws-a"
	if _, err := svc.CreateAssignment("vs-orphan", "workspace", nil, &wsA, vsStrPtr("u1"), 1); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetByVarsetID("vs-orphan")
	if err != nil {
		t.Fatal(err)
	}
	if got.OrgID == nil || *got.OrgID != 1 {
		t.Fatalf("orphan org_id after mount = %v, want 1", got.OrgID)
	}
}

func TestUpdateScopeGlobalClearsOrgID(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	org1 := uint(1)
	vs, _ := svc.Create("tog", "", "specific", vsStrPtr("u1"), &org1)
	wsA := "ws-a"
	_, _ = svc.CreateAssignment(vs.VarsetID, "workspace", nil, &wsA, vsStrPtr("u1"), 1)
	out, err := svc.UpdateScope(vs.VarsetID, "global", 1)
	if err != nil {
		t.Fatal(err)
	}
	if out.OrgID != nil || out.Scope != "global" {
		t.Fatalf("after global: scope=%s org=%v", out.Scope, out.OrgID)
	}
	var n int64
	db.Model(&models.VarsetAssignment{}).Where("varset_id = ?", vs.VarsetID).Count(&n)
	if n != 0 {
		t.Fatalf("mounts left after global: %d", n)
	}
}

func TestNameUniquePerOrg(t *testing.T) {
	db := setupVarsetOrgDB(t)
	svc := NewVariableSetService(db)
	org1, org2 := uint(1), uint(2)
	if _, err := svc.Create("same", "", "specific", vsStrPtr("u1"), &org1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create("same", "", "specific", vsStrPtr("u2"), &org2); err != nil {
		t.Fatalf("same name in other org should be allowed: %v", err)
	}
	if _, err := svc.Create("same", "", "specific", vsStrPtr("u1"), &org1); err == nil {
		t.Fatal("duplicate name in same org must fail")
	}
}

func vsStrPtr(s string) *string { return &s }
