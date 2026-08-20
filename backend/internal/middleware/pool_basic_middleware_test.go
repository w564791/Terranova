package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPoolTokenAuthMiddleware_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	plain := "apt_pool-basic_secretvalue"
	insertPoolToken(t, db, plain, "pool-b", true, nil)

	r := gin.New()
	r.GET("/agent", PoolTokenAuthMiddleware(db), func(c *gin.Context) {
		if c.GetString("pool_id") != "pool-b" {
			t.Errorf("pool_id=%s", c.GetString("pool_id"))
		}
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agent", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
}

func TestPoolTokenAuthMiddleware_Missing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	r := gin.New()
	r.GET("/agent", PoolTokenAuthMiddleware(db), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/agent", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
}

func TestPoolTokenAuthMiddleware_Inactive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	plain := "apt_pool-inactive_secret"
	insertPoolToken(t, db, plain, "pool-i", false, nil)
	r := gin.New()
	r.GET("/agent", PoolTokenAuthMiddleware(db), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agent", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("inactive want 401 got %d", w.Code)
	}
}

func TestPoolTokenAuthWithWorkspaceCheck_MissingWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	plain := "apt_pool-mws_secretvalue"
	insertPoolToken(t, db, plain, "pool-m", true, nil)
	mw := PoolTokenAuthWithWorkspaceCheck(db)
	r := gin.New()
	r.GET("/x", mw, func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing ws want 400 got %d %s", w.Code, w.Body.String())
	}
}

func TestNewIAMPermissionMiddleware_Constructs(t *testing.T) {
	db := setupPoolTokenDB(t)
	m := NewIAMPermissionMiddleware(db)
	if m == nil || m.permissionChecker == nil {
		t.Fatal("expected middleware with checker")
	}
}
