package services

import (
	"database/sql"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	testDBName    = "iac_platform_test"
	defaultDSN    = "host=localhost user=postgres password=postgres123 port=5432 sslmode=disable"
	defaultProdDB = "iac_platform"
)

// testDB is the shared DB connection for all tests in this file
var testDB *gorm.DB

// TestMain manages the test database lifecycle:
// 1. Connect to default postgres DB
// 2. Create iac_platform_test if not exists
// 3. Copy schema from iac_platform (via pg_dump style: drop + recreate)
// 4. Run all tests
// 5. Drop iac_platform_test
func TestMain(m *testing.M) {
	// Allow skip via env
	if os.Getenv("SKIP_INTEGRATION") == "1" {
		log.Println("SKIP_INTEGRATION=1, skipping varset integration tests")
		os.Exit(0)
	}

	// Ensure JWT_SECRET is set (needed for sensitive variable encryption)
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "test-secret-key-for-varset-tests")
	}

	adminDSN := os.Getenv("TEST_ADMIN_DSN")
	if adminDSN == "" {
		adminDSN = defaultDSN + " dbname=postgres"
	}
	prodDBName := defaultProdDB

	// Connect to admin DB
	adminDB, err := sql.Open("pgx", adminDSN)
	if err != nil {
		log.Printf("Cannot connect to PostgreSQL admin DB, skipping: %v", err)
		os.Exit(0)
	}
	if err := adminDB.Ping(); err != nil {
		log.Printf("PostgreSQL not reachable, skipping: %v", err)
		os.Exit(0)
	}

	// Drop test DB if exists (clean slate)
	adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))

	// Create test DB with schema copied from production
	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", testDBName, prodDBName))
	if err != nil {
		// If template fails (e.g. prod DB has active connections), create empty + auto-migrate
		log.Printf("TEMPLATE failed (%v), creating empty DB with auto-migrate", err)
		adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", testDBName))
	}
	adminDB.Close()

	// Connect to test DB
	testDSN := os.Getenv("TEST_DATABASE_URL")
	if testDSN == "" {
		testDSN = defaultDSN + " dbname=" + testDBName
	}
	testDB, err = gorm.Open(postgres.Open(testDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		log.Fatalf("Cannot connect to test DB: %v", err)
	}

	// Auto-migrate essential tables if TEMPLATE didn't work
	sqlDB, _ := testDB.DB()
	var tableCount int
	sqlDB.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='variable_sets'").Scan(&tableCount)
	if tableCount == 0 {
		log.Println("Test DB has no schema, running auto-migrate...")
		testDB.AutoMigrate(
			&models.VariableSet{},
			&models.VarsetVariable{},
			&models.VarsetAssignment{},
		)
		// Create workspace table minimally for resolution tests
		testDB.Exec(`CREATE TABLE IF NOT EXISTS workspaces (
			id SERIAL PRIMARY KEY,
			workspace_id VARCHAR(50) UNIQUE NOT NULL,
			name VARCHAR(200) NOT NULL DEFAULT '',
			execution_mode VARCHAR(20) DEFAULT 'local',
			terraform_version VARCHAR(50) DEFAULT 'latest',
			workdir VARCHAR(500) DEFAULT '/workspace',
			state_backend VARCHAR(50) DEFAULT 'local'
		)`)
		testDB.Exec(`CREATE TABLE IF NOT EXISTS workspace_variables (
			id SERIAL PRIMARY KEY,
			variable_id VARCHAR(20) UNIQUE NOT NULL,
			workspace_id VARCHAR(50) NOT NULL,
			key VARCHAR(100) NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			value TEXT,
			variable_type VARCHAR(20) DEFAULT 'terraform',
			value_format VARCHAR(20) DEFAULT 'string',
			sensitive BOOLEAN DEFAULT false,
			description TEXT,
			is_deleted BOOLEAN DEFAULT false,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_by VARCHAR(20)
		)`)
		testDB.Exec(`CREATE TABLE IF NOT EXISTS workspace_project_relations (
			id SERIAL PRIMARY KEY,
			project_id INTEGER NOT NULL,
			workspace_id VARCHAR(50) NOT NULL
		)`)
	}

	// Run tests
	code := m.Run()

	// Cleanup: drop test DB
	sqlDB.Close()
	adminDB2, _ := sql.Open("pgx", adminDSN)
	if adminDB2 != nil {
		adminDB2.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName))
		adminDB2.Close()
	}

	os.Exit(code)
}

// setupVarsetTestDB returns the shared test DB connection, skipping if not available
func setupVarsetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if testDB == nil {
		t.Skip("Test DB not available")
	}
	return testDB
}

// cleanupVarset removes all test data for a given varset
func cleanupVarset(db *gorm.DB, varsetID string) {
	db.Exec("DELETE FROM varset_assignments WHERE varset_id = ?", varsetID)
	db.Exec("DELETE FROM varset_variables WHERE varset_id = ?", varsetID)
	db.Exec("DELETE FROM variable_sets WHERE varset_id = ?", varsetID)
}

// cleanupWorkspaceVar removes a test workspace variable
func cleanupWorkspaceVar(db *gorm.DB, variableID string) {
	db.Exec("DELETE FROM workspace_variables WHERE variable_id = ?", variableID)
}

// ----- Versioning Tests -----

func TestVarsetVariable_VersionOnCreate(t *testing.T) {
	db := setupVarsetTestDB(t)
	vsSvc := NewVariableSetService(db)
	vs, err := vsSvc.Create("test-ver-create", "test", "specific", nil)
	if err != nil {
		t.Fatalf("create varset: %v", err)
	}
	defer cleanupVarset(db, vs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	v, err := vvSvc.Create(vs.VarsetID, "region", "us-east-1", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)
	if err != nil {
		t.Fatalf("create var: %v", err)
	}
	if v.Version != 1 {
		t.Errorf("initial version: want 1, got %d", v.Version)
	}
}

func TestVarsetVariable_UpdateCreatesNewVersion(t *testing.T) {
	db := setupVarsetTestDB(t)
	vsSvc := NewVariableSetService(db)
	vs, _ := vsSvc.Create("test-ver-update", "test", "specific", nil)
	defer cleanupVarset(db, vs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	v1, _ := vvSvc.Create(vs.VarsetID, "region", "us-east-1", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)

	newVal := "us-west-2"
	v2, err := vvSvc.Update(vs.VarsetID, v1.VariableID, &newVal, nil, nil)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if v2.Version != 2 {
		t.Errorf("want version 2, got %d", v2.Version)
	}
	if v2.Value != "us-west-2" {
		t.Errorf("want 'us-west-2', got '%s'", v2.Value)
	}
	if v2.VariableID != v1.VariableID {
		t.Error("variable_id should be preserved across versions")
	}

	// Old version must still exist in DB (for snapshot)
	var old models.VarsetVariable
	if err := db.Where("variable_id = ? AND version = 1", v1.VariableID).First(&old).Error; err != nil {
		t.Error("old version should remain in DB for snapshot references")
	}
}

func TestVarsetVariable_ListReturnsLatestVersionOnly(t *testing.T) {
	db := setupVarsetTestDB(t)
	vsSvc := NewVariableSetService(db)
	vs, _ := vsSvc.Create("test-ver-list", "test", "specific", nil)
	defer cleanupVarset(db, vs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	v1, _ := vvSvc.Create(vs.VarsetID, "host", "old.example.com", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)

	newVal := "new.example.com"
	vvSvc.Update(vs.VarsetID, v1.VariableID, &newVal, nil, nil)

	vars, err := vvSvc.List(vs.VarsetID, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(vars) != 1 {
		t.Fatalf("want 1 variable, got %d", len(vars))
	}
	if vars[0].Version != 2 {
		t.Errorf("list should return latest version (2), got %d", vars[0].Version)
	}
	if vars[0].Value != "new.example.com" {
		t.Errorf("list should return latest value")
	}
}

func TestVarsetVariable_DeleteMarksAllVersions(t *testing.T) {
	db := setupVarsetTestDB(t)
	vsSvc := NewVariableSetService(db)
	vs, _ := vsSvc.Create("test-ver-delete", "test", "specific", nil)
	defer cleanupVarset(db, vs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	v1, _ := vvSvc.Create(vs.VarsetID, "key1", "val1", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)

	newVal := "val2"
	vvSvc.Update(vs.VarsetID, v1.VariableID, &newVal, nil, nil)
	newVal3 := "val3"
	vvSvc.Update(vs.VarsetID, v1.VariableID, &newVal3, nil, nil)

	if err := vvSvc.Delete(vs.VarsetID, v1.VariableID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	vars, _ := vvSvc.List(vs.VarsetID, "")
	if len(vars) != 0 {
		t.Errorf("list after delete: want 0, got %d", len(vars))
	}

	var deletedCount int64
	db.Model(&models.VarsetVariable{}).
		Where("variable_id = ? AND is_deleted = true", v1.VariableID).
		Count(&deletedCount)
	if deletedCount != 3 {
		t.Errorf("want 3 deleted versions, got %d", deletedCount)
	}
}

// ----- Key Uniqueness -----

func TestVarsetVariable_DuplicateKeyRejected(t *testing.T) {
	db := setupVarsetTestDB(t)
	vsSvc := NewVariableSetService(db)
	vs, _ := vsSvc.Create("test-dup-key", "test", "specific", nil)
	defer cleanupVarset(db, vs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	_, err := vvSvc.Create(vs.VarsetID, "db_host", "localhost", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err = vvSvc.Create(vs.VarsetID, "db_host", "localhost", "",
		models.VariableTypeEnvironment, models.ValueFormatString, false, nil)
	if err == nil {
		t.Error("duplicate key with different type should be rejected")
	}
}

// ----- Sensitive Constraints -----

func TestVarsetVariable_SensitiveUpgradeOnly(t *testing.T) {
	db := setupVarsetTestDB(t)
	vsSvc := NewVariableSetService(db)
	vs, _ := vsSvc.Create("test-sensitive", "test", "specific", nil)
	defer cleanupVarset(db, vs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	v, _ := vvSvc.Create(vs.VarsetID, "api_key", "secret", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)

	sens := true
	v2, err := vvSvc.Update(vs.VarsetID, v.VariableID, nil, nil, &sens)
	if err != nil {
		t.Fatalf("upgrade to sensitive: %v", err)
	}
	if !v2.Sensitive {
		t.Error("should be sensitive after upgrade")
	}

	notSens := false
	_, err = vvSvc.Update(vs.VarsetID, v.VariableID, nil, nil, &notSens)
	if err == nil {
		t.Error("downgrade sensitive should fail")
	}

	resp := v2.ToResponse()
	if resp["value"] != "" {
		t.Error("sensitive value should be masked in response")
	}
}

// ----- Resolution Tests -----

func TestResolution_GlobalVarsetIncluded(t *testing.T) {
	db := setupVarsetTestDB(t)

	// Ensure test workspace
	db.Exec("INSERT INTO workspaces (workspace_id, name) VALUES ('ws-test-resolve', 'test-resolve') ON CONFLICT (workspace_id) DO NOTHING")
	defer db.Exec("DELETE FROM workspaces WHERE workspace_id = 'ws-test-resolve'")

	vsSvc := NewVariableSetService(db)
	gvs, err := vsSvc.Create("test-resolve-global", "test", "global", nil)
	if err != nil {
		t.Fatalf("create global varset: %v", err)
	}
	defer cleanupVarset(db, gvs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	_, err = vvSvc.Create(gvs.VarsetID, "test_resolve_env", "production", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)
	if err != nil {
		t.Fatalf("create var: %v", err)
	}

	resolver := NewVariableResolutionService(db)
	effective, err := resolver.ResolveDisplay("ws-test-resolve")
	if err != nil {
		t.Fatalf("resolve display: %v", err)
	}

	found := false
	for _, ev := range effective {
		if ev.Key == "test_resolve_env" && ev.SourceID == gvs.VarsetID {
			found = true
			if ev.Value != "production" {
				t.Errorf("want 'production', got '%s'", ev.Value)
			}
			if ev.Version != 1 {
				t.Errorf("want version 1, got %d", ev.Version)
			}
			if ev.ScopeLevel != "global" {
				t.Errorf("want scope 'global', got '%s'", ev.ScopeLevel)
			}
		}
	}
	if !found {
		t.Error("global varset variable should appear in effective variables")
	}

	flat, err := resolver.ResolveFlat("ws-test-resolve", models.VariableTypeTerraform)
	if err != nil {
		t.Fatalf("resolve flat: %v", err)
	}
	if flat["test_resolve_env"] != "production" {
		t.Errorf("flat: want 'production', got '%s'", flat["test_resolve_env"])
	}
}

func TestResolution_WorkspaceOverridesVarset(t *testing.T) {
	db := setupVarsetTestDB(t)

	db.Exec("INSERT INTO workspaces (workspace_id, name) VALUES ('ws-test-override', 'test-override') ON CONFLICT (workspace_id) DO NOTHING")
	defer db.Exec("DELETE FROM workspace_variables WHERE workspace_id = 'ws-test-override'")
	defer db.Exec("DELETE FROM workspaces WHERE workspace_id = 'ws-test-override'")

	vsSvc := NewVariableSetService(db)
	gvs, _ := vsSvc.Create("test-override-global", "test", "global", nil)
	defer cleanupVarset(db, gvs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	vvSvc.Create(gvs.VarsetID, "override_key", "from_varset", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)

	wsVar := &models.WorkspaceVariable{
		WorkspaceID:  "ws-test-override",
		Key:          "override_key",
		Value:        "from_workspace",
		VariableType: models.VariableTypeTerraform,
		ValueFormat:  models.ValueFormatString,
		Version:      1,
	}
	db.Create(wsVar)
	defer cleanupWorkspaceVar(db, wsVar.VariableID)

	resolver := NewVariableResolutionService(db)

	effective, _ := resolver.ResolveDisplay("ws-test-override")
	var activeVal, overriddenSource string
	for _, ev := range effective {
		if ev.Key == "override_key" {
			if ev.IsOverridden {
				overriddenSource = ev.SourceType
				if ev.Value != "" {
					t.Error("overridden value should be empty")
				}
			} else {
				activeVal = ev.Value
			}
		}
	}
	if activeVal != "from_workspace" {
		t.Errorf("active should be workspace value, got '%s'", activeVal)
	}
	if overriddenSource != "varset" {
		t.Errorf("overridden should be varset, got '%s'", overriddenSource)
	}

	flat, _ := resolver.ResolveFlat("ws-test-override", models.VariableTypeTerraform)
	if flat["override_key"] != "from_workspace" {
		t.Errorf("flat should return workspace value, got '%s'", flat["override_key"])
	}
}

func TestResolution_VersionInEffectiveVariable(t *testing.T) {
	db := setupVarsetTestDB(t)

	db.Exec("INSERT INTO workspaces (workspace_id, name) VALUES ('ws-test-version', 'test-version') ON CONFLICT (workspace_id) DO NOTHING")
	defer db.Exec("DELETE FROM workspaces WHERE workspace_id = 'ws-test-version'")

	vsSvc := NewVariableSetService(db)
	gvs, _ := vsSvc.Create("test-ver-resolve", "test", "global", nil)
	defer cleanupVarset(db, gvs.VarsetID)

	vvSvc := NewVarsetVariableService(db)
	v1, _ := vvSvc.Create(gvs.VarsetID, "ver_test", "v1", "",
		models.VariableTypeTerraform, models.ValueFormatString, false, nil)

	newVal := "v2"
	vvSvc.Update(gvs.VarsetID, v1.VariableID, &newVal, nil, nil)

	resolver := NewVariableResolutionService(db)
	effective, _ := resolver.ResolveDisplay("ws-test-version")
	for _, ev := range effective {
		if ev.Key == "ver_test" && ev.SourceID == gvs.VarsetID {
			if ev.Version != 2 {
				t.Errorf("should resolve to latest version (2), got %d", ev.Version)
			}
			if ev.Value != "v2" {
				t.Errorf("should have latest value 'v2', got '%s'", ev.Value)
			}
		}
	}
}
