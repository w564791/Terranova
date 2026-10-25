package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/internal/application/service"
	"iac-platform/internal/sandboxreg"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type sandboxPermChecker struct {
	allow map[string]bool // workspace_id -> allow WRITE
	calls []*service.CheckPermissionRequest
}

func (s *sandboxPermChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	s.calls = append(s.calls, req)
	ok := true
	if req.ResourceType == valueobject.ResourceTypeWorkspaceResources {
		ok = s.allow[req.ScopeIDStr]
	}
	lvl := valueobject.PermissionLevelNone
	if ok {
		lvl = valueobject.PermissionLevelWrite
	}
	return &service.CheckPermissionResult{IsAllowed: ok && lvl >= req.RequiredLevel, EffectiveLevel: lvl}, nil
}
func (s *sandboxPermChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return s.CheckPermission(ctx, req)
}
func (s *sandboxPermChecker) CheckBatchPermissions(context.Context, []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (s *sandboxPermChecker) GetUserTeams(context.Context, string) ([]string, error) { return nil, nil }

func setupSandboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:sandbox_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, s := range []string{
		`CREATE TABLE workspaces (
			id INTEGER PRIMARY KEY, workspace_id TEXT UNIQUE, name TEXT,
			state_backend TEXT, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY, org_id INTEGER, name TEXT, is_default INTEGER,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE workspace_project_relations (
			id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id TEXT UNIQUE, project_id INTEGER, created_at DATETIME
		)`,
		`CREATE TABLE sandboxes (
			id TEXT PRIMARY KEY, org_id INTEGER NOT NULL, workspace_id TEXT NOT NULL,
			sandbox_ref TEXT NOT NULL, runtime_ref TEXT, label TEXT, driver TEXT NOT NULL,
			image_digest TEXT, assume_role_arn TEXT, assume_external_id TEXT,
			network_mode TEXT, status TEXT NOT NULL DEFAULT 'registered',
			lease_expires_at DATETIME, created_by TEXT NOT NULL,
			created_at DATETIME, updated_at DATETIME,
			UNIQUE(org_id, sandbox_ref)
		)`,
		`CREATE TABLE shell_sessions (
			id TEXT PRIMARY KEY, sandbox_id TEXT NOT NULL, org_id INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'active', expires_at DATETIME NOT NULL,
			closed_at DATETIME, created_by TEXT NOT NULL, created_at DATETIME, updated_at DATETIME
		)`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	db.Exec(`INSERT INTO projects (id, org_id, name, is_default, created_at, updated_at) VALUES
		(1, 1, 'p1', 1, ?, ?), (2, 2, 'p2', 1, ?, ?)`, now, now, now, now)
	db.Exec(`INSERT INTO workspaces (id, workspace_id, name, state_backend, created_at, updated_at) VALUES
		(10, 'ws-a', 'A', 'local', ?, ?), (20, 'ws-b', 'B', 'local', ?, ?)`, now, now, now, now)
	db.Exec(`INSERT INTO workspace_project_relations (workspace_id, project_id, created_at) VALUES
		('ws-a', 1, ?), ('ws-b', 2, ?)`, now, now)
	return db
}

func sandboxRouter(h *SandboxHandler, orgID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/organizations/:org_id/sandboxes")
	g.Use(func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Set("auth_org_id", orgID)
		c.Next()
	})
	g.POST("", h.CreateSandbox)
	g.GET("", h.ListSandboxes)
	g.GET("/:id", h.GetSandbox)
	g.DELETE("/:id", h.DeleteSandbox)
	g.POST("/:id/shell-sessions", h.CreateShellSession)
	g.DELETE("/shell-sessions/:session_id", h.DeleteShellSession)
	return r
}

func TestCreateSandbox_OK(t *testing.T) {
	t.Setenv("ENV", "development")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"` + sandboxreg.FixtureHarnessID + `","driver":"agentcore_external","runtime_ref":"` + sandboxreg.FixtureRuntimeID + `","network_mode":"PUBLIC","label":"联调"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var row models.Sandbox
	if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row.Status != models.SandboxStatusReady {
		t.Fatalf("status=%q", row.Status)
	}
	if row.NetworkMode == nil || *row.NetworkMode != sandboxreg.NetworkModePublic {
		t.Fatalf("network_mode=%v", row.NetworkMode)
	}
}

func TestCreateSandbox_InvalidRef422(t *testing.T) {
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"https://evil","driver":"agentcore_external"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), sandboxreg.ErrInvalidSandboxRef) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestCreateSandbox_CrossOrg404(t *testing.T) {
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-b": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1) // org 1 trying to use ws-b (org 2)
	body := `{"workspace_id":"ws-b","sandbox_ref":"ref-x","driver":"shell_debug"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestCreateSandbox_VaultRequired422(t *testing.T) {
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm).WithVaultConfigured(false)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-vault","driver":"agentcore_external"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "vault_credential_required") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestCreateSandbox_ProductionRejectsPublic(t *testing.T) {
	t.Setenv("ENV", "production")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-pub","driver":"agentcore_external","network_mode":"PUBLIC"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), sandboxreg.ErrNetworkModeNotAllowed) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestListGetDeleteSandbox(t *testing.T) {
	t.Setenv("ENV", "development")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-list","driver":"shell_debug"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var created models.Sandbox
	json.Unmarshal(w.Body.Bytes(), &created)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/organizations/1/sandboxes", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ref-list") {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/organizations/1/sandboxes/"+created.ID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get %d %s", w.Code, w.Body.String())
	}

	// cross-org get → 404
	r2 := sandboxRouter(h, 2)
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/organizations/2/sandboxes/"+created.ID, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-org get %d", w.Code)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/organizations/1/sandboxes/"+created.ID, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	if len(h.revokeIntents) == 0 {
		t.Fatal("expected revoke intent on delete")
	}
	var status string
	db.Raw(`SELECT status FROM sandboxes WHERE id = ?`, created.ID).Scan(&status)
	if status != models.SandboxStatusUnbound {
		t.Fatalf("status=%q", status)
	}
}

func TestShellSessionCreateDeleteStub(t *testing.T) {
	t.Setenv("ENV", "development")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-shell","driver":"shell_debug","network_mode":"VPC"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var sb models.Sandbox
	json.Unmarshal(w.Body.Bytes(), &sb)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/"+sb.ID+"/shell-sessions", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("shell create %d %s", w.Code, w.Body.String())
	}
	var sess models.ShellSession
	json.Unmarshal(w.Body.Bytes(), &sess)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/organizations/1/sandboxes/shell-sessions/"+sess.ID, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("shell delete %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, ri := range h.revokeIntents {
		if strings.Contains(ri.Reason, "shell_session_close") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected shell revoke intent")
	}
}

func TestAssertAttachable_ProductionRejectsStoredPublic(t *testing.T) {
	pub := sandboxreg.NetworkModePublic
	sb := &models.Sandbox{Status: models.SandboxStatusReady, NetworkMode: &pub}
	t.Setenv("ENV", "development")
	if err := AssertAttachable(sb); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "production")
	err := AssertAttachable(sb)
	if err == nil {
		t.Fatal("expected reject")
	}
	ve, ok := err.(*sandboxreg.ValidationError)
	if !ok || ve.Code != sandboxreg.ErrNetworkModeNotAllowed {
		t.Fatalf("got %#v", err)
	}
}
