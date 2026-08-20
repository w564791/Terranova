package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/domain/entity"

	"github.com/gin-gonic/gin"
)

func roleTenantSecurityRouter(h *RoleHandler, authOrg uint, isSystemAdmin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "org-admin")
		c.Set("auth_org_id", authOrg)
		c.Set("is_system_admin", isSystemAdmin)
		c.Next()
	})
	r.GET("/roles", h.ListRoles)
	r.DELETE("/roles/:id/policies/:policy_id", h.RemoveRolePolicy)
	r.POST("/roles/:id/clone", h.CloneRole)
	r.POST("/applications/:id/roles", h.AssignApplicationRole)
	return r
}

func TestRemoveRolePolicy_BindsRoleToAuthOrgAndHidesQuarantinedRole(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleTenantSecurityRouter(h, 1, false)

	foreignRole := &entity.Role{OrgID: 2, Name: "foreign-role", DisplayName: "Foreign", IsActive: true}
	if err := db.Create(foreignRole).Error; err != nil {
		t.Fatal(err)
	}
	foreignPolicy := &entity.RolePolicy{RoleID: foreignRole.ID, PermissionID: "p", PermissionLevel: "READ", ScopeType: "ORGANIZATION"}
	if err := db.Create(foreignPolicy).Error; err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/roles/"+itoa(foreignRole.ID)+"/policies/"+itoa(foreignPolicy.ID), nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-org policy removal: want 404 got %d %s", w.Code, w.Body.String())
	}
	var remaining entity.RolePolicy
	if err := db.First(&remaining, foreignPolicy.ID).Error; err != nil {
		t.Fatalf("cross-org policy must remain: %v", err)
	}

	// A non-system role at org_id=0 is a quarantined legacy record, not a
	// platform role. It must be hidden rather than globally administrable.
	quarantinedRole := &entity.Role{OrgID: 0, Name: "quarantined-role", DisplayName: "Quarantined", IsActive: false}
	if err := db.Create(quarantinedRole).Error; err != nil {
		t.Fatal(err)
	}
	quarantinedPolicy := &entity.RolePolicy{RoleID: quarantinedRole.ID, PermissionID: "p", PermissionLevel: "READ", ScopeType: "ORGANIZATION"}
	if err := db.Create(quarantinedPolicy).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/roles/"+itoa(quarantinedRole.ID)+"/policies/"+itoa(quarantinedPolicy.ID), nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("quarantined policy removal: want 404 got %d %s", w.Code, w.Body.String())
	}
	var quarantinedRemaining entity.RolePolicy
	if err := db.First(&quarantinedRemaining, quarantinedPolicy.ID).Error; err != nil {
		t.Fatalf("quarantined policy must remain: %v", err)
	}
}

func TestListRoles_OnlySystemRoleIsGlobal(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleTenantSecurityRouter(h, 1, false)

	roles := []*entity.Role{
		{OrgID: 0, Name: "system-global", DisplayName: "System", IsSystem: true, IsActive: true},
		{OrgID: 1, Name: "org-one", DisplayName: "Org one", IsActive: true},
		{OrgID: 2, Name: "org-two", DisplayName: "Org two", IsActive: true},
		{OrgID: 0, Name: "legacy-quarantined", DisplayName: "Legacy", IsActive: false},
	}
	for _, role := range roles {
		if err := db.Create(role).Error; err != nil {
			t.Fatal(err)
		}
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/roles", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list roles: want 200 got %d %s", w.Code, w.Body.String())
	}
	var response ListRolesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	visible := make(map[uint]struct{}, len(response.Roles))
	for _, role := range response.Roles {
		visible[role.ID] = struct{}{}
	}
	for _, role := range roles[:2] {
		if _, ok := visible[role.ID]; !ok {
			t.Fatalf("expected role %q to be visible, got %#v", role.Name, visible)
		}
	}
	for _, role := range roles[2:] {
		if _, ok := visible[role.ID]; ok {
			t.Fatalf("role %q must not be globally visible, got %#v", role.Name, visible)
		}
	}
}

func TestCloneRole_UsesAuthOrgForSourceVisibilityAndName(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleTenantSecurityRouter(h, 1, false)

	source := &entity.Role{OrgID: 1, Name: "source-org-1", DisplayName: "Source", IsActive: true}
	if err := db.Create(source).Error; err != nil {
		t.Fatal(err)
	}
	// The same name in another tenant must not block a clone into org 1.
	if err := db.Create(&entity.Role{OrgID: 2, Name: "copied", DisplayName: "Other tenant", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"name":"copied","display_name":"Copied"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/roles/"+itoa(source.ID)+"/clone", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("clone: want 201 got %d %s", w.Code, w.Body.String())
	}
	var cloned entity.Role
	if err := db.Where("name = ? AND org_id = ?", "copied", 1).First(&cloned).Error; err != nil {
		t.Fatalf("clone must be created in auth org: %v", err)
	}
	if cloned.OrgID != 1 || cloned.IsSystem {
		t.Fatalf("unexpected cloned role: %+v", cloned)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/roles/"+itoa(source.ID)+"/clone", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("same-tenant name collision: want 409 got %d %s", w.Code, w.Body.String())
	}

	foreignSource := &entity.Role{OrgID: 2, Name: "source-org-2", DisplayName: "Foreign", IsActive: true}
	if err := db.Create(foreignSource).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/roles/"+itoa(foreignSource.ID)+"/clone", bytes.NewBufferString(`{"name":"foreign-copy","display_name":"Foreign copy"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("foreign source must be hidden: want 404 got %d %s", w.Code, w.Body.String())
	}
}

func TestAssignApplicationRole_RequiresOrganizationScopeAndVisibleRole(t *testing.T) {
	db := setupRoleHandlerDB(t)
	if err := db.AutoMigrate(&entity.ApplicationRole{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE applications (id INTEGER PRIMARY KEY, org_id INTEGER, name TEXT, app_key TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO applications (id, org_id, name, app_key) VALUES (1, 1, 'ci', 'app-key-ci')`).Error; err != nil {
		t.Fatal(err)
	}

	h := NewRoleHandler(db, allowAllPermChecker{})
	// System admin avoids the empty-policy Role anti-escalation guard so this test
	// isolates tenant/scope validation at the handler boundary.
	r := roleTenantSecurityRouter(h, 1, true)
	localRole := &entity.Role{OrgID: 1, Name: "app-local", DisplayName: "Local", IsActive: true}
	foreignRole := &entity.Role{OrgID: 2, Name: "app-foreign", DisplayName: "Foreign", IsActive: true}
	if err := db.Create(localRole).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(foreignRole).Error; err != nil {
		t.Fatal(err)
	}

	projectScope := `{"role_id":` + itoa(localRole.ID) + `,"scope_type":"PROJECT","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/applications/1/roles", bytes.NewBufferString(projectScope))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("application project-scope role: want 400 got %d %s", w.Code, w.Body.String())
	}

	foreignRoleBody := `{"role_id":` + itoa(foreignRole.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/applications/1/roles", bytes.NewBufferString(foreignRoleBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("foreign role assignment: want 404 got %d %s", w.Code, w.Body.String())
	}

	validBody := `{"role_id":` + itoa(localRole.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/applications/1/roles", bytes.NewBufferString(validBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("same-org application role assignment: want 200 got %d %s", w.Code, w.Body.String())
	}

	var assignments []entity.ApplicationRole
	if err := db.Find(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 1 || assignments[0].RoleID != localRole.ID || assignments[0].ScopeType != "ORGANIZATION" {
		payload, _ := json.Marshal(assignments)
		t.Fatalf("only valid application assignment may persist: %s", payload)
	}
}
