package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
)

func TestPoolTokenAuthWithTaskCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	_ = db.Exec(`CREATE TABLE workspace_tasks (
		id INTEGER PRIMARY KEY,
		workspace_id TEXT
	)`)
	plain := "apt_pool-task_secretvalue"
	insertPoolToken(t, db, plain, "pool-t", true, nil)
	_ = db.Exec(`INSERT INTO pool_allowed_workspaces (pool_id, workspace_id, status) VALUES ('pool-t','ws-ok','active')`)
	_ = db.Exec(`INSERT INTO workspace_tasks (id, workspace_id) VALUES (10,'ws-ok'),(11,'ws-other')`)

	mw := PoolTokenAuthWithTaskCheck(db)

	t.Run("allowed task workspace", func(t *testing.T) {
		r := gin.New()
		r.GET("/tasks/:task_id/logs", mw, func(c *gin.Context) {
			if c.GetUint("authorized_task_id") != 10 {
				t.Errorf("task id %v", c.GetUint("authorized_task_id"))
			}
			if c.GetString("authorized_workspace_id") != "ws-ok" {
				t.Errorf("ws %s", c.GetString("authorized_workspace_id"))
			}
			c.Status(200)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/tasks/10/logs", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("task workspace not allowed", func(t *testing.T) {
		r := gin.New()
		r.GET("/tasks/:task_id/logs", mw, func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/tasks/11/logs", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("task not found", func(t *testing.T) {
		r := gin.New()
		r.GET("/tasks/:task_id/logs", mw, func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/tasks/999/logs", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404 got %d", w.Code)
		}
	})

	t.Run("invalid task id", func(t *testing.T) {
		r := gin.New()
		r.GET("/tasks/:task_id/logs", mw, func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/tasks/abc/logs", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400 got %d", w.Code)
		}
	})

	t.Run("missing auth", func(t *testing.T) {
		r := gin.New()
		r.GET("/tasks/:task_id/logs", mw, func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/10/logs", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("want 401 got %d", w.Code)
		}
	})
}

func TestRequireWorkspacePermission_AllowedAndInvalidLevel(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelRead}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	r := setupGin()
	r.POST("/do", func(c *gin.Context) {
		c.Set("user_id", "u1")
		if m.RequireWorkspacePermission(c, "ws-ok", "READ") {
			c.Status(200)
			return
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/do", nil))
	if w.Code != 200 {
		t.Fatalf("allowed want 200 got %d", w.Code)
	}

	r2 := setupGin()
	r2.POST("/bad", func(c *gin.Context) {
		c.Set("user_id", "u1")
		if m.RequireWorkspacePermission(c, "ws-ok", "NOT_A_LEVEL") {
			c.Status(200)
		}
	})
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest("POST", "/bad", nil))
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("invalid level want 400 got %d", w2.Code)
	}

	r3 := setupGin()
	r3.POST("/unauth", func(c *gin.Context) {
		if m.RequireWorkspacePermission(c, "ws-ok", "READ") {
			c.Status(200)
		}
	})
	w3 := httptest.NewRecorder()
	r3.ServeHTTP(w3, httptest.NewRequest("POST", "/unauth", nil))
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("unauth want 401 got %d", w3.Code)
	}
}
