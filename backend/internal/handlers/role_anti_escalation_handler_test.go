package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
)

// denyWriteChecker allows READ only on MODULES
type denyWriteChecker struct{}

func (denyWriteChecker) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	have := valueobject.PermissionLevelNone
	if req.ResourceType == valueobject.ResourceTypeModules {
		have = valueobject.PermissionLevelRead
	}
	allowed := have != valueobject.PermissionLevelNone && have >= req.RequiredLevel
	return &service.CheckPermissionResult{IsAllowed: allowed, EffectiveLevel: have}, nil
}
func (denyWriteChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return denyWriteChecker{}.CheckPermission(ctx, req)
}
func (denyWriteChecker) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (denyWriteChecker) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func TestAssignRole_PrivilegeEscalation403(t *testing.T) {
	db := setupRoleHandlerDB(t)
	// role requires MODULES WRITE
	role := &entity.Role{OrgID: 1, Name: "mod-writer", DisplayName: "MW", IsActive: true}
	_ = db.Create(role)
	_ = db.Create(&entity.PermissionDefinition{
		ID: "mod-w", Name: "mod-w", ResourceType: valueobject.ResourceTypeModules, ScopeLevel: valueobject.ScopeTypeOrganization, DisplayName: "M",
	})
	_ = db.Create(&entity.RolePolicy{
		RoleID: role.ID, PermissionID: "mod-w", PermissionLevel: "WRITE", ScopeType: "ORGANIZATION",
	})

	h := NewRoleHandler(db, denyWriteChecker{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/users/:id/roles", func(c *gin.Context) {
		c.Set("user_id", "actor-low")
		c.Set("is_system_admin", false)
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.AssignRole)

	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/users/target/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
}

func TestAssignRole_SystemAdminRole403(t *testing.T) {
	db := setupRoleHandlerDB(t)
	role := &entity.Role{Name: "admin", DisplayName: "Admin", IsSystem: true, IsActive: true}
	_ = db.Create(role)

	// checker would allow anything
	h := NewRoleHandler(db, denyWriteChecker{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/users/:id/roles", func(c *gin.Context) {
		c.Set("user_id", "actor")
		c.Set("is_system_admin", false)
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.AssignRole)

	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/users/target/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 for admin role, got %d %s", w.Code, w.Body.String())
	}
}

func TestAssignRole_SystemAdminCanAssignAdminRole(t *testing.T) {
	db := setupRoleHandlerDB(t)
	role := &entity.Role{Name: "admin", DisplayName: "Admin", IsSystem: true, IsActive: true}
	_ = db.Create(role)

	h := NewRoleHandler(db, denyWriteChecker{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/users/:id/roles", func(c *gin.Context) {
		c.Set("user_id", "super")
		c.Set("is_system_admin", true)
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.AssignRole)

	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/users/target/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("system admin assign admin: %d %s", w.Code, w.Body.String())
	}
}

func TestAddRolePolicy_Escalation403(t *testing.T) {
	db := setupRoleHandlerDB(t)
	role := &entity.Role{OrgID: 1, Name: "custom", DisplayName: "C", IsActive: true}
	_ = db.Create(role)
	_ = db.Create(&entity.PermissionDefinition{
		ID: "mod-p", Name: "mod-p", ResourceType: valueobject.ResourceTypeModules, ScopeLevel: valueobject.ScopeTypeOrganization, DisplayName: "M",
	})

	h := NewRoleHandler(db, denyWriteChecker{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/roles/:id/policies", func(c *gin.Context) {
		c.Set("user_id", "actor")
		c.Set("is_system_admin", false)
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.AddRolePolicy)

	// actor only READ, requesting WRITE policy
	body := `{"permission_id":"mod-p","permission_level":"WRITE","scope_type":"ORGANIZATION"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/roles/"+itoa(role.ID)+"/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}

	// READ ok
	body2 := `{"permission_id":"mod-p","permission_level":"READ","scope_type":"ORGANIZATION"}`
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/roles/"+itoa(role.ID)+"/policies", bytes.NewBufferString(body2))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("READ policy should pass: %d %s", w2.Code, w2.Body.String())
	}
}

func TestRemoveRolePolicy_FailClosedWithoutGuard(t *testing.T) {
	db := setupRoleHandlerDB(t)
	role := &entity.Role{OrgID: 1, Name: "c", DisplayName: "C", IsActive: true}
	_ = db.Create(role)
	_ = db.Create(&entity.RolePolicy{RoleID: role.ID, PermissionID: "p", PermissionLevel: "READ", ScopeType: "ORGANIZATION"})

	h := NewRoleHandler(db, nil) // no checker → guard nil
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/roles/:id/policies/:policy_id", func(c *gin.Context) {
		c.Set("user_id", "u")
		c.Next()
	}, h.RemoveRolePolicy)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/roles/"+itoa(role.ID)+"/policies/1", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 fail-closed got %d %s", w.Code, w.Body.String())
	}
}

func TestCloneRole_FailClosedWithoutGuard(t *testing.T) {
	db := setupRoleHandlerDB(t)
	role := &entity.Role{OrgID: 1, Name: "src-clone", DisplayName: "S", IsActive: true}
	_ = db.Create(role)

	h := NewRoleHandler(db, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/roles/:id/clone", func(c *gin.Context) {
		c.Set("user_id", "u")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.CloneRole)
	body := `{"name":"cloned","display_name":"Cloned"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/roles/"+itoa(role.ID)+"/clone", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 fail-closed got %d %s", w.Code, w.Body.String())
	}
}

func TestAssignTeamRole_Escalation403(t *testing.T) {
	db := setupRoleHandlerDB(t)
	_ = db.Exec(`
CREATE TABLE IF NOT EXISTS iam_team_roles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  team_id TEXT, role_id INTEGER, scope_type TEXT, scope_id INTEGER,
  assigned_by TEXT, assigned_at DATETIME, expires_at DATETIME, reason TEXT
);`)
	role := &entity.Role{Name: "mod-w", DisplayName: "MW", IsActive: true, OrgID: 1}
	_ = db.Create(role)
	_ = db.Create(&entity.PermissionDefinition{
		ID: "m2", Name: "m2", ResourceType: valueobject.ResourceTypeModules, ScopeLevel: valueobject.ScopeTypeOrganization, DisplayName: "M",
	})
	_ = db.Create(&entity.RolePolicy{
		RoleID: role.ID, PermissionID: "m2", PermissionLevel: "ADMIN", ScopeType: "ORGANIZATION",
	})

	h := NewRoleHandler(db, denyWriteChecker{})
	r := roleRouterFull(h, true)
	// roleRouterFull sets user_id but not is_system_admin; inject via new router
	gin.SetMode(gin.TestMode)
	r = gin.New()
	r.POST("/teams/:id/roles", func(c *gin.Context) {
		c.Set("user_id", "user-admin")
		c.Set("is_system_admin", false)
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.AssignTeamRole)

	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/teams/team-1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
}
