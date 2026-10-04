package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// varsetChecker: VARIABLE_SETS READ at org; WORKSPACE_MANAGEMENT READ only on ws-a.
type varsetChecker struct{}

func (varsetChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	allowed := (req.ResourceType == valueobject.ResourceTypeVariableSets && req.ScopeType == valueobject.ScopeTypeOrganization) ||
		(req.ResourceType == valueobject.ResourceTypeWorkspaceManagement && req.ScopeIDStr == "ws-a")
	lvl := valueobject.PermissionLevelNone
	if allowed {
		lvl = valueobject.PermissionLevelRead
	}
	return &service.CheckPermissionResult{IsAllowed: allowed, EffectiveLevel: lvl}, nil
}
func (c varsetChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return c.CheckPermission(ctx, req)
}
func (varsetChecker) CheckBatchPermissions(context.Context, []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (varsetChecker) GetUserTeams(context.Context, string) ([]string, error) { return nil, nil }

func TestVariableSetList_WorkspaceIDFilterAndAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("IAM_SINGLE_TENANT", "0")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`,
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`,
		`CREATE TABLE variable_sets (id INTEGER PRIMARY KEY, varset_id TEXT, name TEXT, description TEXT, scope TEXT, is_deleted INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME, created_by TEXT)`,
		`CREATE TABLE varset_variables (id INTEGER PRIMARY KEY, variable_id TEXT, varset_id TEXT, key TEXT, value TEXT, sensitive INTEGER, is_deleted INTEGER DEFAULT 0, version INTEGER)`,
		`CREATE TABLE varset_assignments (id INTEGER PRIMARY KEY, varset_id TEXT, scope_type TEXT, project_id INTEGER, workspace_id TEXT, attached_at DATETIME, attached_by TEXT)`,
		`INSERT INTO workspaces (id, workspace_id) VALUES (1, 'ws-a'), (2, 'ws-b'), (3, 'ws-other')`,
		`INSERT INTO projects (id, org_id) VALUES (10, 1), (30, 2)`,
		`INSERT INTO workspace_project_relations VALUES ('ws-a', 10), ('ws-b', 10), ('ws-other', 30)`,
		`INSERT INTO variable_sets (varset_id, name, scope) VALUES
		   ('vs-global', 'g', 'global'), ('vs-proj', 'p', 'specific'), ('vs-ws', 'w', 'specific'),
		   ('vs-ws-b', 'wb', 'specific'), ('vs-none', 'n', 'specific')`,
		`INSERT INTO variable_sets (varset_id, name, scope, is_deleted) VALUES ('vs-deleted', 'd', 'global', 1)`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id, workspace_id) VALUES
		   ('vs-proj', 'project', 10, NULL), ('vs-ws', 'workspace', NULL, 'ws-a'), ('vs-ws-b', 'workspace', NULL, 'ws-b')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}

	r := gin.New()
	protected := r.Group("/api/v1", func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	SetupVariableSetRoutes(protected, db, middleware.NewIAMPermissionMiddlewareWithChecker(varsetChecker{}))

	get := func(q string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/variable-sets?org_id=1"+q, nil))
		return w
	}

	w := get("&workspace_id=ws-a")
	if w.Code != http.StatusOK {
		t.Fatalf("readable workspace: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []struct {
			VarsetID string `json:"varset_id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	got := map[string]bool{}
	for _, it := range body.Items {
		got[it.VarsetID] = true
	}
	for _, want := range []string{"vs-global", "vs-proj", "vs-ws"} {
		if !got[want] {
			t.Fatalf("missing mountable %s: %v", want, got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("only mountable varsets expected, got %v", got)
	}

	if w := get("&workspace_id=ws-b"); w.Code != http.StatusForbidden {
		t.Fatalf("unreadable workspace must 403, got %d", w.Code)
	}
	if w := get("&workspace_id=ws-other"); w.Code != http.StatusNotFound {
		t.Fatalf("other-org workspace must 404, got %d", w.Code)
	}
	// without workspace_id the existing list behaviour is unchanged
	if w := get(""); w.Code != http.StatusOK {
		t.Fatalf("plain list: %d", w.Code)
	}
}
