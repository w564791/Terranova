package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
)

func TestTerraformOutputController_CrossWorkspaceDeniedBeforeUpgrade(t *testing.T) {
	db := setupTaskLogDB(t)
	mock := &taskLogPermMock{allow: map[string]bool{"ws-a": true}}
	ctrl := NewTerraformOutputController(nil, db, middleware.NewIAMPermissionMiddlewareWithChecker(mock))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/output/stream", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, ctrl.StreamTaskOutput)

	// Task 2 belongs to ws-b. The controller must stop at IAM before trying a
	// WebSocket upgrade, so an ordinary HTTP request receives 403 rather than
	// a stream containing another workspace's output.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/2/output/stream", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d body=%s", w.Code, w.Body.String())
	}
	if mock.last == nil || mock.last.ScopeIDStr != "ws-b" {
		t.Fatalf("should check ws-b before websocket upgrade: %+v", mock.last)
	}
}

func TestTerraformOutputController_FailsClosedWithoutDatabase(t *testing.T) {
	mock := &taskLogPermMock{allow: map[string]bool{"ws-a": true}}
	ctrl := NewTerraformOutputController(nil, nil, middleware.NewIAMPermissionMiddlewareWithChecker(mock))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/output/stream", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, ctrl.StreamTaskOutput)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/1/output/stream", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d body=%s", w.Code, w.Body.String())
	}
	if mock.last != nil {
		t.Fatalf("IAM must not receive an unbound task: %+v", mock.last)
	}
}

func TestTerraformOutputController_RejectsSelectedOrganizationMismatch(t *testing.T) {
	db := setupTaskLogDB(t)
	mock := &taskLogPermMock{allow: map[string]bool{"ws-a": true}}
	ctrl := NewTerraformOutputController(nil, db, middleware.NewIAMPermissionMiddlewareWithChecker(mock))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/output/stream", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, ctrl.StreamTaskOutput)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/1/output/stream?org_id=2", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
}
