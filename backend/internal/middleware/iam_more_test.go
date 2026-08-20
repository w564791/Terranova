package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
)

func TestRequireWorkspacePermission_Denied(t *testing.T) {
	mock := &mockPermChecker{allowed: false, level: valueobject.PermissionLevelNone, reason: "No permission"}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.POST("/do", func(c *gin.Context) {
		c.Set("user_id", "u1")
		if m.RequireWorkspacePermission(c, "ws-x", "WRITE") {
			c.Status(200)
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/do", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", w.Code)
	}
}

func TestRequireWorkspacePermission_SystemAdminNoBypass(t *testing.T) {
	mock := &mockPermChecker{allowed: false, reason: "No permission"}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.POST("/do", func(c *gin.Context) {
		c.Set("user_id", "admin")
		c.Set("is_system_admin", true)
		if m.RequireWorkspacePermission(c, "ws-x", "READ") {
			c.Status(200)
			return
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/do", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("admin must not bypass: %d", w.Code)
	}
	if mock.last == nil {
		t.Fatal("checker must run")
	}
}

func TestRequirePermission_InvalidResource(t *testing.T) {
	m := &IAMPermissionMiddleware{permissionChecker: &mockPermChecker{allowed: true}}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequirePermission("NOT_A_REAL_TYPE_XYZ", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", w.Code)
	}
}

func TestPrincipalFromContext_Team(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "team:t1")
	c.Set("principal_type", "TEAM")
	c.Set("principal_id", "t1")
	uid, pt, pid, ok := principalFromContext(c)
	if !ok || uid != "team:t1" || pt != valueobject.PrincipalTypeTeam || pid != "t1" {
		t.Fatalf("%v %s %s %s", ok, uid, pt, pid)
	}
}

func TestPrincipalFromContext_Application(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "app:app_key_1")
	c.Set("principal_type", "APPLICATION")
	c.Set("principal_id", "app_key_1")
	uid, pt, pid, ok := principalFromContext(c)
	if !ok || uid != "app:app_key_1" || pt != valueobject.PrincipalTypeApplication || pid != "app_key_1" {
		t.Fatalf("%v %s %s %s", ok, uid, pt, pid)
	}
}

func TestRequirePermission_ApplicationPrincipal(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelRead}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "app:k1")
		c.Set("principal_type", "APPLICATION")
		c.Set("principal_id", "k1")
		c.Next()
	}, m.RequirePermission("MODULES", "ORGANIZATION", "READ"), func(c *gin.Context) {
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x?org_id=1", nil))
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
	if mock.last == nil || mock.last.PrincipalType != valueobject.PrincipalTypeApplication {
		t.Fatalf("checker should see APPLICATION: %+v", mock.last)
	}
}
