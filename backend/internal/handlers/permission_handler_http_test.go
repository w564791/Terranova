package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type mockPermSvc struct {
	grantErr   error
	revokeErr  error
	presetErr  error
	list       []*entity.PermissionGrant
	defs       []*entity.PermissionDefinition
	lastGrant  *service.GrantPermissionRequest
	lastRevoke *service.RevokePermissionRequest
}

func (m *mockPermSvc) GrantPermission(ctx context.Context, req *service.GrantPermissionRequest) error {
	m.lastGrant = req
	return m.grantErr
}
func (m *mockPermSvc) RevokePermission(ctx context.Context, req *service.RevokePermissionRequest) error {
	m.lastRevoke = req
	return m.revokeErr
}
func (m *mockPermSvc) ModifyPermission(ctx context.Context, req *service.ModifyPermissionRequest) error {
	return nil
}
func (m *mockPermSvc) GrantPresetPermissions(ctx context.Context, req *service.GrantPresetRequest) error {
	return m.presetErr
}
func (m *mockPermSvc) AssignBuiltinRoleToUser(ctx context.Context, userID, roleName string, scopeType valueobject.ScopeType, scopeID uint, assignedBy, reason string) error {
	return nil
}
func (m *mockPermSvc) ListPermissions(ctx context.Context, st valueobject.ScopeType, id uint) ([]*entity.PermissionGrant, error) {
	return m.list, nil
}
func (m *mockPermSvc) ListPermissionDefinitions(ctx context.Context) ([]*entity.PermissionDefinition, error) {
	return m.defs, nil
}
func (m *mockPermSvc) ListPermissionsByPrincipal(ctx context.Context, pt valueobject.PrincipalType, id string) ([]*entity.PermissionGrant, error) {
	return m.list, nil
}
func (m *mockPermSvc) GetPermissionDefinitionByID(ctx context.Context, id string) (*entity.PermissionDefinition, error) {
	return nil, nil
}

type mockChecker struct {
	result *service.CheckPermissionResult
	err    error
	last   *service.CheckPermissionRequest
}

func (m *mockChecker) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	m.last = req
	return m.result, m.err
}
func (m *mockChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *mockChecker) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (m *mockChecker) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func setupHandlerRouter(h *PermissionHandler, method, path string, handler gin.HandlerFunc, withUser bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, path, func(c *gin.Context) {
		if withUser {
			c.Set("user_id", "user-admin")
			c.Set("principal_type", "USER")
			c.Set("principal_id", "user-admin")
			c.Set("auth_org_id", uint(1))
		}
		c.Next()
	}, handler)
	return r
}

func TestPermissionHandler_CheckPermission(t *testing.T) {
	chk := &mockChecker{result: &service.CheckPermissionResult{
		IsAllowed: true, EffectiveLevel: valueobject.PermissionLevelRead, Source: "regular",
	}}
	h := &PermissionHandler{permissionChecker: chk}

	r := setupHandlerRouter(h, "POST", "/check", h.CheckPermission, true)
	body := `{"resource_type":"MODULES","scope_type":"ORGANIZATION","scope_id":"1","required_level":"READ"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/check", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
	if chk.last == nil || chk.last.PrincipalID != "user-admin" {
		t.Fatalf("principal: %+v", chk.last)
	}
}

func TestPermissionHandler_CheckPermission_Unauthorized(t *testing.T) {
	h := &PermissionHandler{permissionChecker: &mockChecker{result: &service.CheckPermissionResult{IsAllowed: true}}}
	r := setupHandlerRouter(h, "POST", "/check", h.CheckPermission, false)
	body := `{"resource_type":"MODULES","scope_type":"ORGANIZATION","scope_id":"1","required_level":"READ"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/check", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestPermissionHandler_GrantPermission_UserRetired410(t *testing.T) {
	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "POST", "/grant", h.GrantPermission, true)
	body := map[string]interface{}{
		"scope_type": "ORGANIZATION", "scope_id": 1,
		"principal_type": "USER", "principal_id": "u2",
		"permission_id": "orgpm-modules", "permission_level": "WRITE",
		"expires_at": time.Now().UTC().Add(72 * time.Hour).Format("2006-01-02T15:04"), "reason": "test",
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/grant", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("USER direct grant want 410 got %d %s", w.Code, w.Body.String())
	}
	if svc.lastGrant != nil {
		t.Fatal("must not call service for retired USER grant")
	}
}

func TestPermissionHandler_GrantPermission_TeamRetired410(t *testing.T) {
	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "POST", "/grant", h.GrantPermission, true)
	body := `{"scope_type":"ORGANIZATION","scope_id":1,"principal_type":"TEAM","principal_id":"team-1","permission_id":"p1","permission_level":"READ"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/grant", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("TEAM direct grant want 410 got %d", w.Code)
	}
}

func TestPermissionHandler_GrantPermission_ApplicationRetired410(t *testing.T) {
	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "POST", "/grant", h.GrantPermission, true)
	body := map[string]interface{}{
		"scope_type": "ORGANIZATION", "scope_id": 1,
		"principal_type": "APPLICATION", "principal_id": "app_key_x",
		"permission_id": "orgpm-ws", "permission_level": "READ",
	}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/grant", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("APPLICATION direct grant want 410 got %d %s", w.Code, w.Body.String())
	}
	if svc.lastGrant != nil {
		t.Fatal("must not call service for retired APPLICATION grant")
	}
}

func TestPermissionHandler_GrantPermission_InvalidExpires_WithBypass(t *testing.T) {
	// Direct Grant 默认 410；用应急开关测 expires 解析
	t.Setenv("IAM_ALLOW_DIRECT_GRANT", "1")
	db, err := gorm.Open(sqlite.Open("file:grant-exp?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE applications (id INTEGER PRIMARY KEY, org_id INTEGER, app_key TEXT)`)
	_ = db.Exec(`INSERT INTO applications (id, org_id, app_key) VALUES (1, 1, 'k')`)
	h := &PermissionHandler{permissionService: &mockPermSvc{}, db: db}
	r := setupHandlerRouter(h, "POST", "/grant", h.GrantPermission, true)
	body := `{"scope_type":"ORGANIZATION","scope_id":1,"principal_type":"APPLICATION","principal_id":"k","permission_id":"p1","permission_level":"READ","expires_at":"not-a-date"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/grant", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestPermissionHandler_RevokePermission(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:perm-revoke?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE org_permissions (id INTEGER PRIMARY KEY, org_id INTEGER)`)
	_ = db.Exec(`INSERT INTO org_permissions (id, org_id) VALUES (9, 1)`)

	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc, db: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/permissions/:scope_type/:id", func(c *gin.Context) {
		c.Set("user_id", "admin")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.RevokePermission)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/permissions/ORGANIZATION/9", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("revoke want 200 got %d %s", w.Code, w.Body.String())
	}
	if svc.lastRevoke == nil || svc.lastRevoke.AssignmentID != 9 {
		t.Fatalf("revoke body=%s last=%+v", w.Body.String(), svc.lastRevoke)
	}
}

func TestPermissionHandler_RevokePermission_CrossOrg(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:perm-revoke-x?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE org_permissions (id INTEGER PRIMARY KEY, org_id INTEGER)`)
	_ = db.Exec(`INSERT INTO org_permissions (id, org_id) VALUES (9, 2)`)

	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc, db: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/permissions/:scope_type/:id", func(c *gin.Context) {
		c.Set("user_id", "admin")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.RevokePermission)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/permissions/ORGANIZATION/9", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-org revoke want 404 got %d %s", w.Code, w.Body.String())
	}
	if svc.lastRevoke != nil {
		t.Fatal("must not call service on cross-org revoke")
	}
}

func TestPermissionHandler_ListPermissionDefinitions(t *testing.T) {
	svc := &mockPermSvc{defs: []*entity.PermissionDefinition{{ID: "p1", Name: "N"}}}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "GET", "/defs", h.ListPermissionDefinitions, true)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/defs", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPermissionHandler_ListPermissions(t *testing.T) {
	svc := &mockPermSvc{list: []*entity.PermissionGrant{{PermissionID: "p1"}}}
	h := &PermissionHandler{permissionService: svc}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/permissions/:scope_type/:scope_id", func(c *gin.Context) {
		c.Set("user_id", "admin")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.ListPermissions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/permissions/ORGANIZATION/1", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPermissionHandler_ListPermissions_CrossOrg(t *testing.T) {
	h := &PermissionHandler{permissionService: &mockPermSvc{}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/permissions/:scope_type/:scope_id", func(c *gin.Context) {
		c.Set("user_id", "admin")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.ListPermissions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/permissions/ORGANIZATION/2", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
}

func TestPermissionHandler_GrantPermission_CrossOrg(t *testing.T) {
	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "POST", "/grant", h.GrantPermission, true)
	body := `{"scope_type":"ORGANIZATION","scope_id":2,"principal_type":"USER","principal_id":"u2","permission_id":"p1","permission_level":"READ"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/grant", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
	if svc.lastGrant != nil {
		t.Fatal("must not grant cross-org")
	}
}

func TestPermissionHandler_ListUserPermissions_Self(t *testing.T) {
	svc := &mockPermSvc{list: []*entity.PermissionGrant{{PermissionID: "p1"}}}
	h := &PermissionHandler{permissionService: svc}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users/:id/permissions", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Set("is_system_admin", false)
		c.Next()
	}, h.ListUserPermissions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/users/user-1/permissions", nil))
	if w.Code != 200 {
		t.Fatalf("self list: %d %s", w.Code, w.Body.String())
	}
}

func TestPermissionHandler_ListUserPermissions_OtherDenied(t *testing.T) {
	h := &PermissionHandler{permissionService: &mockPermSvc{}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users/:id/permissions", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Set("is_system_admin", false)
		c.Next()
	}, h.ListUserPermissions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/users/user-2/permissions", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", w.Code)
	}
}

func TestPermissionHandler_GrantPreset_UserRetired410(t *testing.T) {
	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "POST", "/preset", h.GrantPresetPermissions, true)
	body := `{"scope_type":"ORGANIZATION","scope_id":1,"principal_type":"USER","principal_id":"u1","preset_name":"READ"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/preset", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("want 410 got %d %s", w.Code, w.Body.String())
	}
}

func TestContainsHelper(t *testing.T) {
	if !contains("permission already exists here", "permission already exists") {
		t.Fatal("contains")
	}
	if contains("abc", "xyz") {
		t.Fatal("not contains")
	}
}

func TestPermissionHandler_BatchGrant_UserRetired410(t *testing.T) {
	svc := &mockPermSvc{}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "POST", "/batch", h.BatchGrantPermissions, true)
	body := `{
		"scope_type":"ORGANIZATION",
		"scope_id":1,
		"principal_type":"USER",
		"principal_id":"u1",
		"permissions":[
			{"permission_id":"MODULES","permission_level":"READ"}
		]
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("want 410 got %d %s", w.Code, w.Body.String())
	}
}

func TestPermissionHandler_BatchGrant_TeamRetired410(t *testing.T) {
	svc := &mockPermSvc{grantErr: errors.New("permission already exists with level: READ")}
	h := &PermissionHandler{permissionService: svc}
	r := setupHandlerRouter(h, "POST", "/batch", h.BatchGrantPermissions, true)
	body := `{
		"scope_type":"ORGANIZATION",
		"scope_id":"1",
		"principal_type":"TEAM",
		"principal_id":"team-1",
		"permissions":[{"permission_id":"MODULES","permission_level":"WRITE"}]
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusGone {
		t.Fatalf("want 410 got %d %s", w.Code, w.Body.String())
	}
}

func TestPermissionHandler_BatchGrant_Unauthenticated(t *testing.T) {
	h := &PermissionHandler{permissionService: &mockPermSvc{}}
	r := setupHandlerRouter(h, "POST", "/batch", h.BatchGrantPermissions, false)
	body := `{
		"scope_type":"ORGANIZATION","scope_id":1,"principal_type":"USER","principal_id":"u1",
		"permissions":[{"permission_id":"MODULES","permission_level":"READ"}]
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestPermissionHandler_BatchGrant_InvalidScope(t *testing.T) {
	h := &PermissionHandler{permissionService: &mockPermSvc{}}
	r := setupHandlerRouter(h, "POST", "/batch", h.BatchGrantPermissions, true)
	body := `{
		"scope_type":"NOT_A_SCOPE","scope_id":1,"principal_type":"USER","principal_id":"u1",
		"permissions":[{"permission_id":"MODULES","permission_level":"READ"}]
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestPermissionHandler_ListTeamPermissions_RequiresSystemAdmin(t *testing.T) {
	h := &PermissionHandler{permissionService: &mockPermSvc{list: []*entity.PermissionGrant{{PermissionID: "p1"}}}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/teams/:id/permissions", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Set("is_system_admin", false)
		c.Next()
	}, h.ListTeamPermissions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/teams/team-1/permissions", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", w.Code)
	}
}

func TestPermissionHandler_ListTeamPermissions_SystemAdminOK(t *testing.T) {
	h := &PermissionHandler{permissionService: &mockPermSvc{list: []*entity.PermissionGrant{{PermissionID: "p1"}}}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/teams/:id/permissions", func(c *gin.Context) {
		c.Set("user_id", "admin")
		c.Set("is_system_admin", true)
		c.Next()
	}, h.ListTeamPermissions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/teams/team-1/permissions", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestPermissionHandler_ListUserPermissions_SystemAdminOther(t *testing.T) {
	h := &PermissionHandler{permissionService: &mockPermSvc{list: []*entity.PermissionGrant{{PermissionID: "p1"}}}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/users/:id/permissions", func(c *gin.Context) {
		c.Set("user_id", "admin")
		c.Set("is_system_admin", true)
		c.Next()
	}, h.ListUserPermissions)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/users/user-other/permissions", nil))
	if w.Code != 200 {
		t.Fatalf("admin other: %d %s", w.Code, w.Body.String())
	}
}

func TestExtractLevelHelper(t *testing.T) {
	if got := extractLevel("permission already exists with level: WRITE"); got != "WRITE" {
		t.Fatalf("got %q", got)
	}
	if got := extractLevel("no level here"); got != "" {
		t.Fatalf("got %q", got)
	}
}
