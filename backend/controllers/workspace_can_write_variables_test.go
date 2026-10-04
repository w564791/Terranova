package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

// varsWriteChecker grants WORKSPACE_VARIABLES WRITE only on the listed workspaces.
type varsWriteChecker struct{ writable map[string]bool }

func (v *varsWriteChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	ok := req.ResourceType == valueobject.ResourceTypeWorkspaceVars &&
		req.ScopeType == valueobject.ScopeTypeWorkspace && v.writable[req.ScopeIDStr]
	lvl := valueobject.PermissionLevelNone
	if ok {
		lvl = valueobject.PermissionLevelWrite
	}
	return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: lvl}, nil
}
func (v *varsWriteChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return v.CheckPermission(ctx, req)
}
func (v *varsWriteChecker) CheckBatchPermissions(context.Context, []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (v *varsWriteChecker) GetUserTeams(context.Context, string) ([]string, error) { return nil, nil }

func TestGetWorkspaces_CapabilityListCarriesCanWriteVariables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWorkspaceCtrlDB(t)
	now := time.Now()
	for _, stmt := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`,
		`INSERT INTO projects (id, org_id) VALUES (1, 1)`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES ('ws-w', 1), ('ws-r', 1)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(`INSERT INTO workspaces (id, workspace_id, name, state_backend, created_at, updated_at) VALUES
		(1, 'ws-w', 'w', 'local', ?, ?), (2, 'ws-r', 'r', 'local', ?, ?)`, now, now, now, now)

	iam := middleware.NewIAMPermissionMiddlewareWithChecker(&varsWriteChecker{writable: map[string]bool{"ws-w": true}})
	ctrl := NewWorkspaceController(services.NewWorkspaceService(db), services.NewWorkspaceOverviewService(db), nil)
	ctrl.CanWriteVariables = func(c *gin.Context, ws string) bool {
		return iam.HasWorkspaceResourcePermission(c, ws, "WORKSPACE_VARIABLES", "WRITE")
	}
	r := gin.New()
	r.GET("/workspaces", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Set("auth_org_id", uint(1))
		c.Set(service.WorkspaceListAccessContextKey, &service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{"ws-w", "ws-r"}})
		c.Next()
	}, ctrl.GetWorkspaces)

	list := func(q string) map[string]*bool {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces"+q, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		var body struct {
			Data struct {
				Items []struct {
					WorkspaceID       string `json:"workspace_id"`
					CanWriteVariables *bool  `json:"can_write_variables"`
				} `json:"items"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		out := map[string]*bool{}
		for _, it := range body.Data.Items {
			out[it.WorkspaceID] = it.CanWriteVariables
		}
		return out
	}
	got := list("?capability=WORKSPACE_RESOURCES:WRITE")
	if got["ws-w"] == nil || !*got["ws-w"] || got["ws-r"] == nil || *got["ws-r"] {
		t.Fatalf("can_write_variables wrong: ws-w=%v ws-r=%v", got["ws-w"], got["ws-r"])
	}
	if plain := list(""); plain["ws-w"] != nil {
		t.Fatal("plain list must not carry can_write_variables")
	}
}
