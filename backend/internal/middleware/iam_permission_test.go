package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mockPermChecker struct {
	allowed bool
	level   valueobject.PermissionLevel
	reason  string
	err     error
	last    *service.CheckPermissionRequest
}

type staticWorkspaceListAccessResolver struct {
	access *service.WorkspaceListAccess
	err    error
	last   service.WorkspaceListAccessRequest
}

func (r *staticWorkspaceListAccessResolver) ResolveWorkspaceListAccess(_ context.Context, req service.WorkspaceListAccessRequest) (*service.WorkspaceListAccess, error) {
	r.last = req
	return r.access, r.err
}

func (m *mockPermChecker) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	m.last = req
	if m.err != nil {
		return nil, m.err
	}
	return &service.CheckPermissionResult{
		IsAllowed:      m.allowed,
		EffectiveLevel: m.level,
		DenyReason:     m.reason,
	}, nil
}
func (m *mockPermChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *mockPermChecker) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (m *mockPermChecker) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func setupGin() *gin.Engine {
	gin.SetMode(gin.TestMode)
	// 中间件单测默认单租户，避免每条请求都要带 org_id
	_ = os.Setenv("IAM_SINGLE_TENANT", "1")
	return gin.New()
}

func TestResolveOrgScopeID_RequiresOrgWhenMultiTenant(t *testing.T) {
	_ = os.Setenv("IAM_SINGLE_TENANT", "0")
	t.Cleanup(func() { _ = os.Setenv("IAM_SINGLE_TENANT", "1") })

	gin.SetMode(gin.TestMode)

	// 无 org_id：必须报错（各自独立 Context，避免 gin queryCache 粘连）
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("GET", "/x", nil)
	_, err := resolveOrgScopeID(c1)
	if err == nil {
		t.Fatal("multi-tenant must require org_id")
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/x?org_id=3", nil)
	id, err := resolveOrgScopeID(c2)
	if err != nil || id != 3 {
		t.Fatalf("got %d err=%v", id, err)
	}
}

func TestRequirePermission_Unauthenticated(t *testing.T) {
	m := &IAMPermissionMiddleware{permissionChecker: &mockPermChecker{}}
	r := setupGin()
	r.GET("/x", m.RequirePermission("MODULES", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestRequirePermission_SystemAdminNoBypass(t *testing.T) {
	// 目标态：业务 IAM 不旁路 is_system_admin，必须走 Role/grant
	mock := &mockPermChecker{allowed: false, level: valueobject.PermissionLevelNone, reason: "No permission"}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "admin-1")
		c.Set("is_system_admin", true)
		c.Next()
	}, m.RequirePermission("MODULES", "ORGANIZATION", "ADMIN"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("system_admin must not bypass business IAM: want 403 got %d", w.Code)
	}
	if mock.last == nil {
		t.Fatal("checker must be called for system admin on business routes")
	}
}

func TestRequirePermission_TeamPrincipalPassed(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelWrite}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "team:team-99")
		c.Set("principal_type", "TEAM")
		c.Set("principal_id", "team-99")
		c.Next()
	}, m.RequirePermission("MODULES", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	if mock.last.PrincipalType != valueobject.PrincipalTypeTeam || mock.last.PrincipalID != "team-99" {
		t.Fatalf("principal not passed: %+v", mock.last)
	}
}

func TestRequirePermission_Allowed(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelRead}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequirePermission("MODULES", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != 200 {
		t.Fatalf("want 200 got %d", w.Code)
	}
	if mock.last == nil || mock.last.ScopeID != 1 {
		t.Fatalf("default org scope_id=1, last=%+v", mock.last)
	}
}

func TestRequirePermission_Denied(t *testing.T) {
	mock := &mockPermChecker{allowed: false, level: valueobject.PermissionLevelNone, reason: "No permission"}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequirePermission("MODULES", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", w.Code)
	}
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["deny_reason"] != "No permission" {
		t.Fatalf("body: %v", body)
	}
}

func TestRequireWorkspaceListAccess_AllowsScopedRoleAndStoresAllowList(t *testing.T) {
	resolver := &staticWorkspaceListAccessResolver{
		access: &service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{"ws-a"}},
	}
	m := &IAMPermissionMiddleware{workspaceListAccess: resolver}
	r := setupGin()
	r.GET("/workspaces", func(c *gin.Context) {
		c.Set("user_id", "u-scoped")
		c.Next()
	}, m.RequireWorkspaceListAccess(), func(c *gin.Context) {
		raw, exists := c.Get(service.WorkspaceListAccessContextKey)
		access, ok := raw.(*service.WorkspaceListAccess)
		if !exists || !ok || access == nil || len(access.WorkspaceIDs) != 1 || access.WorkspaceIDs[0] != "ws-a" {
			c.Status(http.StatusInternalServerError)
			return
		}
		if org, _ := c.Get("auth_org_id"); org != uint(42) {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces?org_id=42", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("scoped list should reach controller, got %d: %s", w.Code, w.Body.String())
	}
	if resolver.last.UserID != "u-scoped" || resolver.last.PrincipalID != "u-scoped" || resolver.last.OrgID != 42 {
		t.Fatalf("resolver request mismatch: %+v", resolver.last)
	}
}

func TestRequireWorkspaceListAccess_DeniesNoGrantButAllowsAuthorizedEmptyList(t *testing.T) {
	for name, access := range map[string]*service.WorkspaceListAccess{
		"none":          {},
		"full":          {FullOrganization: true, HasAccess: true},
		"empty_project": {HasAccess: true},
	} {
		t.Run(name, func(t *testing.T) {
			m := &IAMPermissionMiddleware{workspaceListAccess: &staticWorkspaceListAccessResolver{access: access}}
			r := setupGin()
			r.GET("/workspaces", func(c *gin.Context) {
				c.Set("user_id", "u1")
				c.Next()
			}, m.RequireWorkspaceListAccess(), func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces?org_id=1", nil))
			if name == "none" && w.Code != http.StatusForbidden {
				t.Fatalf("no scoped workspace must be denied, got %d: %s", w.Code, w.Body.String())
			}
			if name != "none" && w.Code != http.StatusOK {
				t.Fatalf("valid %s grant must permit an empty visible set, got %d: %s", name, w.Code, w.Body.String())
			}
		})
	}
}

func TestRequirePermission_InvalidOrgID(t *testing.T) {
	m := &IAMPermissionMiddleware{permissionChecker: &mockPermChecker{allowed: true}}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequirePermission("MODULES", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x?org_id=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid org_id got %d", w.Code)
	}
}

func TestRequirePermission_OrgIDFromQuery(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelRead}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequirePermission("MODULES", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x?org_id=42", nil))
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	if mock.last.ScopeID != 42 {
		t.Fatalf("scope_id want 42 got %d", mock.last.ScopeID)
	}
}

func TestRequirePermission_WorkspacePath(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelWrite}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/workspaces/:id/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "WRITE"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces/ws-abc/x", nil))
	if w.Code != 200 {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
	if mock.last.ScopeIDStr != "ws-abc" {
		t.Fatalf("want ScopeIDStr=ws-abc got %+v", mock.last)
	}
}

func TestRequirePermission_MissingWorkspaceScope(t *testing.T) {
	m := &IAMPermissionMiddleware{permissionChecker: &mockPermChecker{allowed: true}}
	r := setupGin()
	// no :id in path
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestRequireAnyPermission_OR(t *testing.T) {
	// first denied, second allowed — mock returns based on resource type
	call := 0
	mock := &mockSeqChecker{results: []bool{false, true}}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/workspaces/:id/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequireAnyPermission([]PermissionRequirement{
		{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
	}), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces/ws-1/x", nil))
	if w.Code != 200 {
		t.Fatalf("OR should pass on second: %d (calls=%d)", w.Code, call)
	}
}

type mockSeqChecker struct {
	results []bool
	i       int
}

func (m *mockSeqChecker) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	ok := false
	if m.i < len(m.results) {
		ok = m.results[m.i]
		m.i++
	}
	return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: valueobject.PermissionLevelWrite}, nil
}
func (m *mockSeqChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *mockSeqChecker) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (m *mockSeqChecker) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func TestRequireWorkspacePermission_BindsWorkspace(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelWrite}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.POST("/do", func(c *gin.Context) {
		c.Set("user_id", "u1")
		if !m.RequireWorkspacePermission(c, "ws-xyz", "WRITE") {
			return
		}
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/do", nil))
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	if mock.last.ScopeIDStr != "ws-xyz" {
		t.Fatalf("scope: %+v", mock.last)
	}
}

func TestEnforceWorkspaceOrgBindingForParam_BindsCustomPathParamAndFailsClosedOnDuplicates(t *testing.T) {
	t.Setenv("IAM_SINGLE_TENANT", "0")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		"CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL)",
		"CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER NOT NULL)",
		"CREATE TABLE workspace_project_relations (id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL, project_id INTEGER NOT NULL)",
		"INSERT INTO workspaces (id, workspace_id) VALUES (1, 'ws-org-1'), (2, 'ws-org-2'), (3, 'ws-duplicate')",
		"INSERT INTO projects (id, org_id) VALUES (1, 1), (2, 2)",
		"INSERT INTO workspace_project_relations (id, workspace_id, project_id) VALUES (1, 'ws-org-1', 1), (2, 'ws-org-2', 2), (3, 'ws-duplicate', 1), (4, 'ws-duplicate', 2)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed database: %v", err)
		}
	}

	r := gin.New()
	r.GET("/cmdb/workspaces/:workspace_id/tree",
		EnforceWorkspaceOrgBindingForParam(db, "workspace_id"),
		func(c *gin.Context) {
			if orgID, ok := AuthOrgID(c); !ok || orgID != 1 {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Status(http.StatusNoContent)
		},
	)

	for _, tc := range []struct {
		name string
		url  string
		want int
	}{
		{name: "matching organization", url: "/cmdb/workspaces/ws-org-1/tree?org_id=1", want: http.StatusNoContent},
		{name: "foreign workspace", url: "/cmdb/workspaces/ws-org-2/tree?org_id=1", want: http.StatusNotFound},
		{name: "duplicate relationship fails closed", url: "/cmdb/workspaces/ws-duplicate/tree?org_id=1", want: http.StatusNotFound},
		{name: "missing organization", url: "/cmdb/workspaces/ws-org-1/tree", want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if w.Code != tc.want {
				t.Fatalf("want %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
		})
	}
}
