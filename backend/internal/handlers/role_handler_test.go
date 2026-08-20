package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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

// allowAllPermChecker 现有 CRUD 测试用：闭包防提权全部放行
type allowAllPermChecker struct{}

func (allowAllPermChecker) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	return &service.CheckPermissionResult{IsAllowed: true, EffectiveLevel: valueobject.PermissionLevelAdmin}, nil
}
func (allowAllPermChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return allowAllPermChecker{}.CheckPermission(ctx, req)
}
func (allowAllPermChecker) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (allowAllPermChecker) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func setupRoleHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&entity.Organization{},
		&entity.UserOrganization{},
		&entity.Role{},
		&entity.UserRole{},
		&entity.RolePolicy{},
		&entity.PermissionDefinition{},
	); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Organization{ID: 1, Name: "org-1", DisplayName: "Org 1", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	// teams for AssignTeamRole org binding
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS teams (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_id TEXT UNIQUE,
		org_id INTEGER,
		name TEXT
	)`)
	_ = db.Exec(`INSERT INTO teams (team_id, org_id, name) VALUES
		('team-1', 1, 'Team 1'),
		('team-2', 1, 'Team 2'),
		('team-other', 2, 'Other Org Team')`)
	// Team-role assignment repairs organization memberships for existing team
	// members, so the shared handler fixture needs the production relation too.
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS team_members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role TEXT,
		joined_at DATETIME,
		joined_by TEXT
	)`)
	// projects for scope checks (optional)
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT, org_id INTEGER, name TEXT
	)`)
	_ = db.Exec(`INSERT INTO projects (id, org_id, name) VALUES (1, 1, 'p1')`)
	return db
}

func roleRouter(h *RoleHandler, withUser bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if withUser {
			c.Set("user_id", "user-admin")
			c.Set("auth_org_id", uint(1))
			c.Set("is_system_admin", true) // CRUD 测试不测防提权细节
		}
		c.Next()
	})
	r.GET("/roles", h.ListRoles)
	r.GET("/roles/:id", h.GetRole)
	r.POST("/roles", h.CreateRole)
	r.PUT("/roles/:id", h.UpdateRole)
	r.DELETE("/roles/:id", h.DeleteRole)
	r.POST("/users/:id/roles", h.AssignRole)
	r.DELETE("/users/:id/roles/:assignment_id", h.RevokeRole)
	r.GET("/users/:id/roles", h.ListUserRoles)
	return r
}

func TestRoleHandler_ListAndCreate(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouter(h, true)

	// seed system role
	sys := &entity.Role{Name: "admin", DisplayName: "Admin", IsSystem: true, IsActive: true}
	if err := db.Create(sys).Error; err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/roles?is_active=true", nil))
	if w.Code != 200 {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	var list ListRolesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if list.Total < 1 {
		t.Fatalf("expected roles, got %+v", list)
	}

	// create custom role
	body := `{"name":"custom-dev","display_name":"Custom Dev","description":"d"}`
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req)
	if w2.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w2.Code, w2.Body.String())
	}
	var created entity.Role
	_ = json.Unmarshal(w2.Body.Bytes(), &created)
	if created.IsSystem {
		t.Fatal("API-created roles must not be system")
	}
	if created.Name != "custom-dev" {
		t.Fatalf("%+v", created)
	}
}

func TestRoleHandler_CreateUnauthenticated(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouter(h, false)
	body := `{"name":"x","display_name":"X"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestRoleHandler_GetRoleWithPolicies(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouter(h, true)

	role := &entity.Role{OrgID: 1, Name: "reader", DisplayName: "Reader", IsActive: true}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}
	_ = db.Create(&entity.PermissionDefinition{
		ID: "MODULES", Name: "modules", DisplayName: "Modules", ResourceType: "MODULES", ScopeLevel: valueobject.ScopeTypeOrganization,
	})
	_ = db.Create(&entity.RolePolicy{
		RoleID: role.ID, PermissionID: "MODULES", PermissionLevel: "READ", ScopeType: "ORGANIZATION",
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/roles/1", nil))
	// id may not be 1 if autoincrement starts differently — use role.ID
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/roles/"+itoa(role.ID), nil))
	if w.Code != 200 {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	var detail RoleDetailResponse
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Policies) != 1 {
		t.Fatalf("policies=%+v", detail.Policies)
	}

	// not found
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/roles/9999", nil))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w2.Code)
	}
	// bad id
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("GET", "/roles/abc", nil))
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w3.Code)
	}
}

func TestRoleHandler_AssignAndRevoke(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouter(h, true)

	role := &entity.Role{OrgID: 1, Name: "ws-admin", DisplayName: "WS Admin", IsActive: true}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}

	// assign
	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1,"reason":"test"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/users/user-1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("assign: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]interface{})
	if data == nil {
		t.Fatalf("resp=%v", resp)
	}
	assignID := uint(data["id"].(float64))
	var membership entity.UserOrganization
	if err := db.Where("user_id = ? AND org_id = ?", "user-1", 1).First(&membership).Error; err != nil {
		t.Fatalf("role assignment must ensure user organization membership: %v", err)
	}

	// duplicate → 409
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/users/user-1/roles", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("dup want 409 got %d %s", w2.Code, w2.Body.String())
	}

	// invalid scope
	bad := `{"role_id":` + itoa(role.ID) + `,"scope_type":"NOPE","scope_id":1}`
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("POST", "/users/user-1/roles", bytes.NewBufferString(bad))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("bad scope want 400 got %d", w3.Code)
	}

	// role not found
	miss := `{"role_id":999,"scope_type":"ORGANIZATION","scope_id":1}`
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("POST", "/users/user-1/roles", bytes.NewBufferString(miss))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d", w4.Code)
	}

	// assign with expires
	exp := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	bodyExp := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1,"expires_at":"` + exp + `"}`
	w5 := httptest.NewRecorder()
	req5 := httptest.NewRequest("POST", "/users/user-2/roles", bytes.NewBufferString(bodyExp))
	req5.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w5, req5)
	if w5.Code != 200 {
		t.Fatalf("assign exp: %d %s", w5.Code, w5.Body.String())
	}

	// invalid expires
	bodyBadExp := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1,"expires_at":"not-a-date"}`
	w6 := httptest.NewRecorder()
	req6 := httptest.NewRequest("POST", "/users/user-3/roles", bytes.NewBufferString(bodyBadExp))
	req6.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w6, req6)
	if w6.Code != http.StatusBadRequest {
		t.Fatalf("bad exp want 400 got %d", w6.Code)
	}

	// revoke
	w7 := httptest.NewRecorder()
	r.ServeHTTP(w7, httptest.NewRequest("DELETE", "/users/user-1/roles/"+itoa(assignID), nil))
	if w7.Code != http.StatusNoContent && w7.Code != http.StatusOK {
		t.Fatalf("revoke want 204/200 got %d body=%s", w7.Code, w7.Body.String())
	}

	// revoke wrong user binding
	// re-assign for isolation
	_ = db.Create(&entity.UserRole{UserID: "user-9", RoleID: role.ID, ScopeType: "ORGANIZATION", ScopeID: 9, AssignedAt: time.Now()})
	var ur entity.UserRole
	_ = db.Where("user_id = ?", "user-9").First(&ur).Error
	w8 := httptest.NewRecorder()
	r.ServeHTTP(w8, httptest.NewRequest("DELETE", "/users/user-other/roles/"+itoa(ur.ID), nil))
	if w8.Code != http.StatusNotFound {
		t.Fatalf("cross-user revoke want 404 got %d", w8.Code)
	}
}

func TestRoleHandler_AssignUnauthenticated(t *testing.T) {
	db := setupRoleHandlerDB(t)
	role := &entity.Role{Name: "r", DisplayName: "R", IsSystem: true, IsActive: true}
	_ = db.Create(role)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouter(h, false)
	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/users/u1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestRoleHandler_UpdateAndDelete_SystemProtected(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouter(h, true)

	sys := &entity.Role{Name: "admin", DisplayName: "Admin", IsSystem: true, IsActive: true, OrgID: 0}
	custom := &entity.Role{Name: "custom", DisplayName: "Custom", IsSystem: false, IsActive: true, OrgID: 1}
	if err := db.Create(sys).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(custom).Error; err != nil {
		t.Fatal(err)
	}

	// update custom
	body := `{"display_name":"Custom2","description":"d","is_active":true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/roles/"+itoa(custom.ID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("update custom: %d %s", w.Code, w.Body.String())
	}

	// cannot delete system role
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("DELETE", "/roles/"+itoa(sys.ID), nil))
	if w2.Code != http.StatusForbidden {
		t.Fatalf("delete system want 403 got %d %s", w2.Code, w2.Body.String())
	}

	// delete custom ok
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("DELETE", "/roles/"+itoa(custom.ID), nil))
	if w3.Code != http.StatusNoContent {
		t.Fatalf("delete custom want 204 got %d %s", w3.Code, w3.Body.String())
	}
}

func TestRoleHandler_ListUserRoles(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouter(h, true)
	role := &entity.Role{OrgID: 1, Name: "r1", DisplayName: "R1", IsActive: true}
	_ = db.Create(role)
	_ = db.Create(&entity.UserRole{
		UserID: "user-x", RoleID: role.ID, ScopeType: "ORGANIZATION", ScopeID: 1, AssignedAt: time.Now(),
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/users/user-x/roles", nil))
	if w.Code != 200 {
		t.Fatalf("list user roles: %d %s", w.Code, w.Body.String())
	}
}

func itoa(u uint) string {
	return jsonNumber(u)
}

func jsonNumber(u uint) string {
	b, _ := json.Marshal(u)
	return string(b)
}
