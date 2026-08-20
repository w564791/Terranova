package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("deny non-admin", func(t *testing.T) {
		r := gin.New()
		r.GET("/p", func(c *gin.Context) {
			c.Set("user_id", "u1")
			c.Set("is_system_admin", false)
			c.Next()
		}, RequireSystemAdmin(), func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403 got %d", w.Code)
		}
	})

	t.Run("allow admin", func(t *testing.T) {
		r := gin.New()
		r.GET("/p", func(c *gin.Context) {
			c.Set("user_id", "admin")
			c.Set("is_system_admin", true)
			c.Next()
		}, RequireSystemAdmin(), func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
		if w.Code != 200 {
			t.Fatalf("want 200 got %d", w.Code)
		}
	})
}

func TestRequireAnyPermission_InvalidOrgID(t *testing.T) {
	mock := &mockPermChecker{allowed: true}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.GET("/x", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequireAnyPermission([]PermissionRequirement{
		{ResourceType: "MODULES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
	}), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x?org_id=not-a-number", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d body=%s", w.Code, w.Body.String())
	}
}
