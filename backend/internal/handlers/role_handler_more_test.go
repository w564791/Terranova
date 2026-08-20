package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"

	"github.com/gin-gonic/gin"
)

func roleRouterFull(h *RoleHandler, withUser bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if withUser {
			c.Set("user_id", "user-admin")
			c.Set("auth_org_id", uint(1))
			c.Set("is_system_admin", true)
		}
		c.Next()
	})
	r.POST("/roles/:id/policies", h.AddRolePolicy)
	r.DELETE("/roles/:id/policies/:policy_id", h.RemoveRolePolicy)
	r.POST("/teams/:id/roles", h.AssignTeamRole)
	r.GET("/teams/:id/roles", h.ListTeamRoles)
	r.DELETE("/teams/:id/roles/:assignment_id", h.RevokeTeamRole)
	r.POST("/applications/:id/roles", h.AssignApplicationRole)
	r.GET("/applications/:id/roles", h.ListApplicationRoles)
	r.DELETE("/applications/:id/roles/:assignment_id", h.RevokeApplicationRole)
	return r
}

func TestRoleHandler_AddAndRemovePolicy(t *testing.T) {
	db := setupRoleHandlerDB(t)
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouterFull(h, true)

	role := &entity.Role{OrgID: 1, Name: "pol-role", DisplayName: "Pol", IsActive: true}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.PermissionDefinition{
		ID: "MODULES", Name: "modules", DisplayName: "Modules", ResourceType: "MODULES", ScopeLevel: "ORGANIZATION",
	}).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"permission_id":"MODULES","permission_level":"READ","scope_type":"ORGANIZATION"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/roles/"+itoa(role.ID)+"/policies", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("add policy: %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	policyID := uint(data["id"].(float64))

	// PermissionDefinition 的组织级 scope 不能被伪造成项目级 Role policy。
	badScope := `{"permission_id":"MODULES","permission_level":"READ","scope_type":"PROJECT"}`
	wScope := httptest.NewRecorder()
	reqScope := httptest.NewRequest("POST", "/roles/"+itoa(role.ID)+"/policies", bytes.NewBufferString(badScope))
	reqScope.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wScope, reqScope)
	if wScope.Code != http.StatusBadRequest {
		t.Fatalf("mismatched policy scope want 400 got %d %s", wScope.Code, wScope.Body.String())
	}

	// bad level
	bad := `{"permission_id":"MODULES","permission_level":"NOPE","scope_type":"ORGANIZATION"}`
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/roles/"+itoa(role.ID)+"/policies", bytes.NewBufferString(bad))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("bad level want 400 got %d", w2.Code)
	}

	// unknown permission
	miss := `{"permission_id":"NOPE","permission_level":"READ","scope_type":"ORGANIZATION"}`
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest("POST", "/roles/"+itoa(role.ID)+"/policies", bytes.NewBufferString(miss))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("miss perm want 404 got %d", w3.Code)
	}

	// bad role id
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("POST", "/roles/abc/policies", bytes.NewBufferString(body))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("bad role id want 400 got %d", w4.Code)
	}

	// role not found
	w5 := httptest.NewRecorder()
	req5 := httptest.NewRequest("POST", "/roles/9999/policies", bytes.NewBufferString(body))
	req5.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w5, req5)
	if w5.Code != http.StatusNotFound {
		t.Fatalf("role 404 want 404 got %d", w5.Code)
	}

	// remove wrong role binding
	other := &entity.Role{OrgID: 1, Name: "other", DisplayName: "O", IsActive: true}
	_ = db.Create(other)
	w6 := httptest.NewRecorder()
	r.ServeHTTP(w6, httptest.NewRequest("DELETE", "/roles/"+itoa(other.ID)+"/policies/"+itoa(policyID), nil))
	if w6.Code != http.StatusNotFound {
		t.Fatalf("cross-role policy want 404 got %d", w6.Code)
	}

	// remove ok
	w7 := httptest.NewRecorder()
	r.ServeHTTP(w7, httptest.NewRequest("DELETE", "/roles/"+itoa(role.ID)+"/policies/"+itoa(policyID), nil))
	if w7.Code != http.StatusNoContent {
		t.Fatalf("remove want 204 got %d %s", w7.Code, w7.Body.String())
	}

	// invalid policy id
	w8 := httptest.NewRecorder()
	r.ServeHTTP(w8, httptest.NewRequest("DELETE", "/roles/"+itoa(role.ID)+"/policies/abc", nil))
	if w8.Code != http.StatusBadRequest {
		t.Fatalf("bad policy id want 400 got %d", w8.Code)
	}
}

func TestRoleHandler_TeamRoleAssignListRevoke(t *testing.T) {
	db := setupRoleHandlerDB(t)
	// team roles table used via raw Table()
	if err := db.Exec(`
CREATE TABLE IF NOT EXISTS iam_team_roles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  team_id TEXT NOT NULL,
  role_id INTEGER NOT NULL,
  scope_type TEXT NOT NULL,
  scope_id INTEGER NOT NULL,
  assigned_by TEXT,
  assigned_at DATETIME,
  expires_at DATETIME,
  reason TEXT
);`).Error; err != nil {
		t.Fatal(err)
	}
	// ListTeamRoles 校验 team→org
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS teams (team_id TEXT PRIMARY KEY, org_id INTEGER, name TEXT)`)
	_ = db.Exec(`INSERT INTO teams (team_id, org_id, name) VALUES ('team-1', 1, 't1')`)

	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouterFull(h, true)

	role := &entity.Role{Name: "team-r", DisplayName: "TeamR", IsActive: true, OrgID: 1}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1,"reason":"t"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/teams/team-1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("assign team role: %d %s", w.Code, w.Body.String())
	}

	// conflict
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/teams/team-1/roles", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("dup want 409 got %d %s", w2.Code, w2.Body.String())
	}

	// list
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("GET", "/teams/team-1/roles", nil))
	if w3.Code != 200 {
		t.Fatalf("list: %d %s", w3.Code, w3.Body.String())
	}
	var list map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &list)
	if list["total"].(float64) < 1 {
		t.Fatalf("list empty: %v", list)
	}

	// get assignment id
	var assignID int
	row := db.Raw(`SELECT id FROM iam_team_roles WHERE team_id = ? LIMIT 1`, "team-1").Row()
	if err := row.Scan(&assignID); err != nil {
		t.Fatal(err)
	}

	// revoke wrong team
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, httptest.NewRequest("DELETE", "/teams/team-other/roles/"+itoa(uint(assignID)), nil))
	if w4.Code != http.StatusNotFound {
		t.Fatalf("cross team revoke want 404 got %d", w4.Code)
	}

	// revoke ok
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, httptest.NewRequest("DELETE", "/teams/team-1/roles/"+itoa(uint(assignID)), nil))
	if w5.Code != http.StatusNoContent {
		t.Fatalf("revoke want 204 got %d %s", w5.Code, w5.Body.String())
	}

	// unauth assign
	systemRole := &entity.Role{Name: "team-r-system", DisplayName: "Team System", IsSystem: true, IsActive: true}
	if err := db.Create(systemRole).Error; err != nil {
		t.Fatal(err)
	}
	unauthBody := `{"role_id":` + itoa(systemRole.ID) + `,"scope_type":"ORGANIZATION","scope_id":1,"reason":"t"}`
	rNo := roleRouterFull(h, false)
	w6 := httptest.NewRecorder()
	req6 := httptest.NewRequest("POST", "/teams/team-1/roles", bytes.NewBufferString(unauthBody))
	req6.Header.Set("Content-Type", "application/json")
	rNo.ServeHTTP(w6, req6)
	if w6.Code != http.StatusUnauthorized {
		t.Fatalf("unauth want 401 got %d", w6.Code)
	}

	// invalid scope
	bad := `{"role_id":` + itoa(role.ID) + `,"scope_type":"XXX","scope_id":1}`
	w7 := httptest.NewRecorder()
	req7 := httptest.NewRequest("POST", "/teams/team-1/roles", bytes.NewBufferString(bad))
	req7.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w7, req7)
	if w7.Code != http.StatusBadRequest {
		t.Fatalf("bad scope want 400 got %d", w7.Code)
	}

	// assign with expires
	exp := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	bodyExp := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1,"expires_at":"` + exp + `"}`
	w8 := httptest.NewRecorder()
	req8 := httptest.NewRequest("POST", "/teams/team-2/roles", bytes.NewBufferString(bodyExp))
	req8.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w8, req8)
	if w8.Code != 200 {
		t.Fatalf("assign exp: %d %s", w8.Code, w8.Body.String())
	}

	// role not found
	miss := `{"role_id":999,"scope_type":"ORGANIZATION","scope_id":1}`
	w9 := httptest.NewRecorder()
	req9 := httptest.NewRequest("POST", "/teams/team-1/roles", bytes.NewBufferString(miss))
	req9.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w9, req9)
	if w9.Code != http.StatusNotFound {
		t.Fatalf("role miss want 404 got %d", w9.Code)
	}

	// bad assignment id
	w10 := httptest.NewRecorder()
	r.ServeHTTP(w10, httptest.NewRequest("DELETE", "/teams/team-1/roles/abc", nil))
	if w10.Code != http.StatusBadRequest {
		t.Fatalf("bad assign id want 400 got %d", w10.Code)
	}
}

func TestRoleHandler_ApplicationRoleAssignListRevoke(t *testing.T) {
	db := setupRoleHandlerDB(t)
	if err := db.AutoMigrate(&entity.ApplicationRole{}); err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS applications (id INTEGER PRIMARY KEY, org_id INTEGER, name TEXT, app_key TEXT)`)
	_ = db.Exec(`INSERT INTO applications (id, org_id, name, app_key) VALUES (1, 1, 'ci', 'app_key_ci')`)
	_ = db.Exec(`INSERT INTO applications (id, org_id, name, app_key) VALUES (2, 2, 'other', 'app_key_other')`)

	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouterFull(h, true)

	role := &entity.Role{OrgID: 1, Name: "app-r", DisplayName: "AppR", IsActive: true}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1,"reason":"ci"}`
	// assign by numeric id → resolve to app_key
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/applications/1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("assign app role: %d %s", w.Code, w.Body.String())
	}
	var assignResp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &assignResp)
	data, _ := assignResp["data"].(map[string]interface{})
	if data == nil || data["application_principal_id"] != "app_key_ci" {
		t.Fatalf("want app_key_ci principal got %v", assignResp)
	}

	// conflict
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/applications/app_key_ci/roles", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("dup want 409 got %d %s", w2.Code, w2.Body.String())
	}

	// list by app_key
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("GET", "/applications/app_key_ci/roles", nil))
	if w3.Code != 200 {
		t.Fatalf("list: %d %s", w3.Code, w3.Body.String())
	}
	var list map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &list)
	if list["total"].(float64) < 1 {
		t.Fatalf("list empty: %v", list)
	}

	var assignID uint
	if err := db.Model(&entity.ApplicationRole{}).
		Select("id").Where("application_principal_id = ?", "app_key_ci").
		Scan(&assignID).Error; err != nil || assignID == 0 {
		t.Fatalf("load assign id: %v %d", err, assignID)
	}

	// cross-org app
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest("POST", "/applications/2/roles", bytes.NewBufferString(body))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusNotFound {
		t.Fatalf("cross-org app want 404 got %d %s", w4.Code, w4.Body.String())
	}

	// revoke
	w5 := httptest.NewRecorder()
	r.ServeHTTP(w5, httptest.NewRequest("DELETE", "/applications/1/roles/"+itoa(assignID), nil))
	if w5.Code != http.StatusNoContent {
		t.Fatalf("revoke want 204 got %d %s", w5.Code, w5.Body.String())
	}

	// not found app
	w6 := httptest.NewRecorder()
	req6 := httptest.NewRequest("POST", "/applications/nope/roles", bytes.NewBufferString(body))
	req6.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w6, req6)
	if w6.Code != http.StatusNotFound {
		t.Fatalf("missing app want 404 got %d", w6.Code)
	}
}
