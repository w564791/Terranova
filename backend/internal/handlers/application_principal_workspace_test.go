package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type wsPermMock struct {
	orgAllow bool
	wsAllow  map[string]bool
}

func (m *wsPermMock) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	ok := false
	if req.ScopeType == valueobject.ScopeTypeOrganization && req.ResourceType == valueobject.ResourceTypeAllWorkspaces {
		ok = m.orgAllow
	}
	if req.ScopeType == valueobject.ScopeTypeWorkspace {
		ok = m.wsAllow != nil && m.wsAllow[req.ScopeIDStr]
	}
	lv := valueobject.PermissionLevelNone
	if ok {
		lv = valueobject.PermissionLevelRead
	}
	return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: lv}, nil
}
func (m *wsPermMock) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *wsPermMock) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (m *wsPermMock) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func setupAppWSDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_ = db.Exec(`CREATE TABLE workspaces (
		id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT, description TEXT,
		execution_mode TEXT, terraform_version TEXT, state TEXT, tags TEXT,
		created_at DATETIME, updated_at DATETIME
	)`)
	_ = db.Exec(`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`)
	_ = db.Exec(`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`)
	_ = db.Exec(`INSERT INTO projects (id, org_id) VALUES (1, 1), (2, 2)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name, description, tags, created_at, updated_at)
		VALUES
		(1, 'ws-a', 'A', 'da', '{"env":"prod","team":"platform"}', ?, ?),
		(2, 'ws-b', 'B', 'db', '{"env":"prod"}', ?, ?),
		(3, 'ws-c', 'C', 'dc', '{"env":"dev","team":"platform"}', ?, ?)`,
		now, now, now, now, now, now)
	_ = db.Exec(`INSERT INTO workspace_project_relations (workspace_id, project_id)
		VALUES ('ws-a', 1), ('ws-b', 2), ('ws-c', 1)`)
	return db
}

func TestApplicationListWorkspaces_FiltersByOrg(t *testing.T) {
	db := setupAppWSDB(t)
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(&wsPermMock{orgAllow: true})
	h := NewApplicationWorkspaceHandler(db, iam)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/app/workspaces", func(c *gin.Context) {
		c.Set("user_id", "app:k")
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "k")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.ListWorkspaces)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/workspaces", nil))
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "ws-a") {
		t.Fatalf("want ws-a in body: %s", body)
	}
	if strings.Contains(body, "ws-b") {
		t.Fatalf("must not include other org ws-b: %s", body)
	}
}

func TestApplicationGetWorkspace_DeniedWithoutGrant(t *testing.T) {
	db := setupAppWSDB(t)
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(&wsPermMock{orgAllow: false})
	h := NewApplicationWorkspaceHandler(db, iam)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/app/workspaces/:id", func(c *gin.Context) {
		c.Set("user_id", "app:k")
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "k")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.GetWorkspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/workspaces/ws-a", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
}

func TestApplicationGetWorkspace_OKWithOrgGrant(t *testing.T) {
	db := setupAppWSDB(t)
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(&wsPermMock{orgAllow: true})
	h := NewApplicationWorkspaceHandler(db, iam)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/app/workspaces/:id", func(c *gin.Context) {
		c.Set("user_id", "app:k")
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "k")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.GetWorkspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/workspaces/ws-a", nil))
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
}

func TestApplicationGetWorkspace_CrossOrg404(t *testing.T) {
	db := setupAppWSDB(t)
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(&wsPermMock{orgAllow: true})
	h := NewApplicationWorkspaceHandler(db, iam)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/app/workspaces/:id", func(c *gin.Context) {
		c.Set("user_id", "app:k")
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "k")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.GetWorkspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/workspaces/ws-b", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d %s", w.Code, w.Body.String())
	}
}

func TestApplicationListWorkspaces_TagFilter(t *testing.T) {
	db := setupAppWSDB(t)
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(&wsPermMock{orgAllow: true})
	h := NewApplicationWorkspaceHandler(db, iam)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/app/workspaces", func(c *gin.Context) {
		c.Set("user_id", "app:k")
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "k")
		c.Set("auth_org_id", uint(1))
		// Application 只允许 env=prod
		c.Set("application", &entity.Application{
			WorkspaceTagFilter: map[string]interface{}{"env": "prod"},
		})
		c.Next()
	}, h.ListWorkspaces)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/workspaces", nil))
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// org1 有 ws-a(prod) 与 ws-c(dev)；filter env=prod → 仅 ws-a
	if !strings.Contains(body, "ws-a") {
		t.Fatalf("want ws-a: %s", body)
	}
	if strings.Contains(body, "ws-c") {
		t.Fatalf("ws-c env=dev must be filtered: %s", body)
	}
	if strings.Contains(body, "ws-b") {
		t.Fatalf("ws-b is other org: %s", body)
	}
}

func TestApplicationGetWorkspace_TagMismatch404(t *testing.T) {
	db := setupAppWSDB(t)
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(&wsPermMock{orgAllow: true})
	h := NewApplicationWorkspaceHandler(db, iam)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/app/workspaces/:id", func(c *gin.Context) {
		c.Set("user_id", "app:k")
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "k")
		c.Set("auth_org_id", uint(1))
		c.Set("application", &entity.Application{
			WorkspaceTagFilter: map[string]interface{}{"env": "prod"},
		})
		c.Next()
	}, h.GetWorkspace)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/app/workspaces/ws-c", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("tag mismatch want 404 got %d %s", w.Code, w.Body.String())
	}
}
