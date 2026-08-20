package controllers

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAICMDBScopeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER NOT NULL)`,
		`CREATE TABLE workspaces (workspace_id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT NOT NULL, project_id INTEGER NOT NULL)`,
		`INSERT INTO projects (id, org_id) VALUES (1, 1), (2, 2)`,
		`INSERT INTO workspaces (workspace_id, name) VALUES
			('ws-org-1', 'Organization 1'), ('ws-org-1-peer', 'Organization 1 peer'),
			('ws-org-2', 'Organization 2'), ('ws-corrupt', 'Corrupt')`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES
			('ws-org-1', 1), ('ws-org-1-peer', 1), ('ws-org-2', 2),
			('ws-corrupt', 1), ('ws-corrupt', 2)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed scope fixture: %v\n%s", err, statement)
		}
	}
	return db
}

func newAICMDBScopeContext(orgID uint) *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	if orgID > 0 {
		ctx.Set("auth_org_id", orgID)
	}
	return ctx
}

func TestResolveAICMDBTenantScopeUsesAuthOrgAndStrictWorkspaceBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAICMDBScopeDB(t)

	scope, err := resolveAICMDBTenantScope(newAICMDBScopeContext(1), db, "1", "ws-org-1")
	if err != nil {
		t.Fatalf("resolve valid scope: %v", err)
	}
	if scope.organizationID != "1" || scope.workspaceID != "ws-org-1" {
		t.Fatalf("unexpected trusted context: %+v", scope)
	}
	if got, want := scope.cmdbScope.WorkspaceIDs(), []string{"ws-org-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("workspace scope = %v, want %v", got, want)
	}

	for _, tc := range []struct {
		name       string
		requestOrg string
		workspace  string
	}{
		{name: "body org mismatch", requestOrg: "2", workspace: "ws-org-1"},
		{name: "cross org workspace", requestOrg: "1", workspace: "ws-org-2"},
		{name: "duplicate workspace relation", requestOrg: "1", workspace: "ws-corrupt"},
		{name: "invalid body org", requestOrg: "not-a-number", workspace: "ws-org-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := resolveAICMDBTenantScope(newAICMDBScopeContext(1), db, tc.requestOrg, tc.workspace); err == nil {
				t.Fatal("expected tenant context rejection")
			}
		})
	}

	if _, err := resolveAICMDBTenantScope(newAICMDBScopeContext(0), db, "", "ws-org-1"); err == nil {
		t.Fatal("missing auth_org_id must be rejected")
	}
}

func TestResolveAICMDBTenantScopeListsOnlyUnambiguousOrganizationWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAICMDBScopeDB(t)

	scope, err := resolveAICMDBTenantScope(newAICMDBScopeContext(1), db, "", "")
	if err != nil {
		t.Fatalf("resolve organization scope: %v", err)
	}
	if got, want := scope.cmdbScope.WorkspaceIDs(), []string{"ws-org-1", "ws-org-1-peer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("organization scope leaked or included corrupt relation: got %v want %v", got, want)
	}
}
