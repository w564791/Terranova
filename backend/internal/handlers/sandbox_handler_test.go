package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/internal/sandboxreg"
	"iac-platform/services"

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
		`CREATE TABLE shell_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT, shell_session_id TEXT NOT NULL,
			sandbox_id TEXT NOT NULL, org_id INTEGER NOT NULL, workspace_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE, expires_at DATETIME NOT NULL,
			revoked_at DATETIME, created_at DATETIME
		)`,
		`CREATE TABLE workspace_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			created_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			description TEXT,
			task_type TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			execution_mode TEXT NOT NULL DEFAULT 'local',
			stage TEXT DEFAULT 'pending',
			context TEXT,
			error_code TEXT,
			error_message TEXT,
			error_reason TEXT,
			completed_at DATETIME,
			is_background INTEGER DEFAULT 0
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
	g.POST("/shell-sessions/:session_id/plans", h.CreateShellPlan)
	g.GET("/shell-sessions/:session_id/plans", h.ListShellPlans)
	return r
}

func TestCreateSandbox_OK(t *testing.T) {
	t.Setenv("ENV", "development")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"` + sandboxreg.FixtureHarnessARN + `","driver":"agentcore_external","runtime_ref":"` + sandboxreg.FixtureRuntimeARN + `","network_mode":"PUBLIC","label":"联调"}`
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

func TestCreateSandbox_InvalidNetworkMode422(t *testing.T) {
	t.Setenv("ENV", "development")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-badmode","driver":"agentcore_external","network_mode":"PRIVATE"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), sandboxreg.ErrInvalidNetworkMode) {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), sandboxreg.ErrNetworkModeNotAllowed) {
		t.Fatalf("must not reuse network_mode_not_allowed for bad body: %s", w.Body.String())
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
	var status string
	db.Raw(`SELECT status FROM sandboxes WHERE id = ?`, created.ID).Scan(&status)
	if status != models.SandboxStatusUnbound {
		t.Fatalf("status=%q", status)
	}
}

func TestShellSessionIssueAndRevokeToken(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-shell","driver":"shell_debug","network_mode":"VPC"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("sandbox create %d %s", w.Code, w.Body.String())
	}
	var sb models.Sandbox
	json.Unmarshal(w.Body.Bytes(), &sb)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/"+sb.ID+"/shell-sessions", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("shell create %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID         string `json:"id"`
		ShellToken string `json:"shell_token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ShellToken == "" || resp.ID == "" {
		t.Fatalf("missing token/id: %s", w.Body.String())
	}
	// Token typ=shell + shell_session_id claim
	caller, err := h.tokens.Authenticate(context.Background(), resp.ShellToken)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if caller.Type != services.StateTokenTypeShell || caller.ShellSessionID != resp.ID {
		t.Fatalf("caller=%+v", caller)
	}
	if !caller.ReadOnly() {
		t.Fatal("shell token must be read-only")
	}
	var hashCount int64
	db.Table("shell_tokens").Where("shell_session_id = ? AND revoked_at IS NULL", resp.ID).Count(&hashCount)
	if hashCount != 1 {
		t.Fatalf("active hashes=%d", hashCount)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/organizations/1/sandboxes/shell-sessions/"+resp.ID, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("shell delete %d %s", w.Code, w.Body.String())
	}
	if _, err := h.tokens.Authenticate(context.Background(), resp.ShellToken); err == nil {
		t.Fatal("expected revoke to reject second use")
	}
	db.Table("shell_tokens").Where("shell_session_id = ? AND revoked_at IS NULL", resp.ID).Count(&hashCount)
	if hashCount != 0 {
		t.Fatalf("active hashes after revoke=%d", hashCount)
	}
	var closedAt *time.Time
	db.Raw(`SELECT closed_at FROM shell_sessions WHERE id = ?`, resp.ID).Scan(&closedAt)
	if closedAt == nil {
		t.Fatal("session not closed")
	}
}

func TestShellSession_ProductionRejectsEmptyOrPublic(t *testing.T) {
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	for _, mode := range []string{"", "PUBLIC"} {
		t.Run("mode_"+mode, func(t *testing.T) {
			t.Setenv("ENV", "development") // register first (PUBLIC/empty OK)
			db := setupSandboxTestDB(t)
			h := NewSandboxHandler(db, perm)
			r := sandboxRouter(h, 1)
			body := `{"workspace_id":"ws-a","sandbox_ref":"ref-` + mode + `","driver":"shell_debug"`
			if mode != "" {
				body += `,"network_mode":"` + mode + `"`
			}
			body += `}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != http.StatusCreated {
				t.Fatalf("create %d %s", w.Code, w.Body.String())
			}
			var sb models.Sandbox
			json.Unmarshal(w.Body.Bytes(), &sb)
			t.Setenv("ENV", "production")
			w = httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/"+sb.ID+"/shell-sessions", nil))
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), sandboxreg.ErrNetworkModeNotAllowed) {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestShellSession_HarnessInvokeFailure(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	stub := sandboxreg.NewStubHarnessInvoker()
	stub.Err = errors.New("simulated InvokeHarness failure")
	h := NewSandboxHandler(db, perm).WithHarness(stub)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-wake","driver":"shell_debug","network_mode":"VPC"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var sb models.Sandbox
	json.Unmarshal(w.Body.Bytes(), &sb)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/"+sb.ID+"/shell-sessions", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "harness_invoke_failed") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	var n int64
	db.Table("shell_sessions").Count(&n)
	if n != 0 {
		t.Fatalf("session should not be created on wake failure, n=%d", n)
	}
}

func TestAssertAttachable_ProductionRequiresVPCSnapshot(t *testing.T) {
	pub := sandboxreg.NetworkModePublic
	vpc := sandboxreg.NetworkModeVPC
	t.Setenv("ENV", "development")
	if err := AssertAttachable(&models.Sandbox{Status: models.SandboxStatusReady, NetworkMode: &pub}); err != nil {
		t.Fatal(err)
	}
	if err := AssertAttachable(&models.Sandbox{Status: models.SandboxStatusReady}); err != nil {
		t.Fatal(err) // empty OK in non-prod
	}
	t.Setenv("ENV", "production")
	for _, sb := range []*models.Sandbox{
		{Status: models.SandboxStatusReady, NetworkMode: &pub},
		{Status: models.SandboxStatusReady, NetworkMode: nil}, // empty/unknown
	} {
		err := AssertAttachable(sb)
		if err == nil {
			t.Fatalf("expected reject for %#v", sb.NetworkMode)
		}
		ve, ok := err.(*sandboxreg.ValidationError)
		if !ok || ve.Code != sandboxreg.ErrNetworkModeNotAllowed {
			t.Fatalf("got %#v", err)
		}
	}
	if err := AssertAttachable(&models.Sandbox{Status: models.SandboxStatusReady, NetworkMode: &vpc}); err != nil {
		t.Fatal(err)
	}
}

// captureCtxHarness records the context passed to InvokeHarness (timeout check).
type captureCtxHarness struct {
	ctx context.Context
}

func (c *captureCtxHarness) InvokeHarness(ctx context.Context, req sandboxreg.InvokeRequest) error {
	c.ctx = ctx
	return nil
}

func TestShellSession_InvokeHarnessUsesTimeoutContext(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	cap := &captureCtxHarness{}
	h := NewSandboxHandler(db, perm).WithHarness(cap)
	r := sandboxRouter(h, 1)
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-timeout","driver":"shell_debug","network_mode":"VPC"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var sb models.Sandbox
	json.Unmarshal(w.Body.Bytes(), &sb)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/"+sb.ID+"/shell-sessions", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	if cap.ctx == nil {
		t.Fatal("InvokeHarness was not called")
	}
	deadline, ok := cap.ctx.Deadline()
	if !ok {
		t.Fatal("InvokeHarness context must have a deadline (InvokeHarnessTimeout)")
	}
	remaining := time.Until(deadline)
	// Bound should be ~InvokeHarnessTimeout (60s); allow slack for scheduling.
	if remaining > sandboxreg.InvokeHarnessTimeout || remaining < sandboxreg.InvokeHarnessTimeout-5*time.Second {
		t.Fatalf("deadline remaining=%v, want roughly %v", remaining, sandboxreg.InvokeHarnessTimeout)
	}
}

func TestShellSession_FailingHarnessInvoker_Returns503(t *testing.T) {
	// FailingHarnessInvoker is what HarnessInvokerFromEnv installs when flag=1
	// and AWS config fails; shell-create must surface 503 harness_invoke_failed.
	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	fail := &sandboxreg.FailingHarnessInvoker{Err: errors.New("AWS config for InvokeHarness: simulated")}
	h := NewSandboxHandler(db, perm).WithHarness(fail)
	r := sandboxRouter(h, 1)
	t.Setenv("ENV", "development")
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	body := `{"workspace_id":"ws-a","sandbox_ref":"ref-failcfg","driver":"shell_debug","network_mode":"VPC"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	var sb models.Sandbox
	json.Unmarshal(w.Body.Bytes(), &sb)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/"+sb.ID+"/shell-sessions", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "harness_invoke_failed") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func createShellDebugSession(t *testing.T, r *gin.Engine, ref string) (sandboxID, sessionID string) {
	t.Helper()
	body := `{"workspace_id":"ws-a","sandbox_ref":"` + ref + `","driver":"shell_debug","network_mode":"VPC"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("sandbox create %d %s", w.Code, w.Body.String())
	}
	var sb models.Sandbox
	if err := json.Unmarshal(w.Body.Bytes(), &sb); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/"+sb.ID+"/shell-sessions", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("shell create %d %s", w.Code, w.Body.String())
	}
	var sess struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sess); err != nil || sess.ID == "" {
		t.Fatalf("session: %s", w.Body.String())
	}
	return sb.ID, sess.ID
}

func TestShellPlan_CreateList_HistoryOnly(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	restore := services.SetProviderMirrorConfiguredForTest(func() bool { return true })
	defer restore()

	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	_, sessID := createShellDebugSession(t, r, "ref-shell-plan")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/shell-sessions/"+sessID+"/plans", nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("create plan %d %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["task_type"] != string(models.TaskTypeShellPlan) {
		t.Fatalf("task_type=%v", created["task_type"])
	}
	if created["approvable"] != false || created["lock"] != false || created["history_only"] != true {
		t.Fatalf("must be history-only / non-approvable: %v", created)
	}
	if created["cloud_identity"] != "user_prepared" {
		t.Fatalf("cloud_identity=%v", created["cloud_identity"])
	}
	// Persist: workspace_tasks row exists with shell_plan
	var n int64
	db.Table("workspace_tasks").Where("task_type = ?", models.TaskTypeShellPlan).Count(&n)
	if n != 1 {
		t.Fatalf("shell_plan rows=%d", n)
	}
	// No approval run side-effect table is required; EnsureApprovalRun is a no-op.
	if services.IsShellPlanTask(&models.WorkspaceTask{TaskType: models.TaskTypeShellPlan}) == false {
		t.Fatal("IsShellPlanTask")
	}
	if !services.TaskExcludedFromApproval(&models.WorkspaceTask{TaskType: models.TaskTypeShellPlan}) {
		t.Fatal("shell_plan must be excluded from approval")
	}

	// List
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/organizations/1/sandboxes/shell-sessions/"+sessID+"/plans", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Total != 1 || len(listed.Items) != 1 {
		t.Fatalf("list=%+v", listed)
	}
	if listed.Items[0]["task_type"] != string(models.TaskTypeShellPlan) {
		t.Fatalf("list item=%v", listed.Items[0])
	}
	if listed.Items[0]["approvable"] != false {
		t.Fatal("list must mark non-approvable")
	}
}

func TestShellPlan_NoMirror_503(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	restore := services.SetProviderMirrorConfiguredForTest(func() bool { return false })
	defer restore()

	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	_, sessID := createShellDebugSession(t, r, "ref-no-mirror")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/shell-sessions/"+sessID+"/plans", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "provider_unreachable") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
	var n int64
	db.Table("workspace_tasks").Where("task_type = ?", models.TaskTypeShellPlan).Count(&n)
	if n != 0 {
		t.Fatalf("must not persist history on 503, got %d", n)
	}
	// Sandbox stays ready
	var status string
	db.Raw(`SELECT status FROM sandboxes WHERE sandbox_ref = ?`, "ref-no-mirror").Scan(&status)
	if status != models.SandboxStatusReady {
		t.Fatalf("sandbox status=%q want ready", status)
	}
}

func TestShellPlan_ClosedSession_422(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	restore := services.SetProviderMirrorConfiguredForTest(func() bool { return true })
	defer restore()

	db := setupSandboxTestDB(t)
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&sandboxPermChecker{allow: map[string]bool{"ws-a": true}})
	h := NewSandboxHandler(db, perm)
	r := sandboxRouter(h, 1)
	_, sessID := createShellDebugSession(t, r, "ref-closed-plan")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/organizations/1/sandboxes/shell-sessions/"+sessID, nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("close %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/organizations/1/sandboxes/shell-sessions/"+sessID+"/plans", nil))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "shell_session_inactive") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
