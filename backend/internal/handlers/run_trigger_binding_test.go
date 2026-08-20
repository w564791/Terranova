package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// runTriggerPermMock allows WRITE only on listed workspace ids
type runTriggerPermMock struct {
	allow map[string]bool
	last  *service.CheckPermissionRequest
}

func (m *runTriggerPermMock) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	m.last = req
	ok := m.allow != nil && m.allow[req.ScopeIDStr]
	lv := valueobject.PermissionLevelNone
	if ok {
		lv = valueobject.PermissionLevelWrite
	}
	return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: lv, DenyReason: "No permission"}, nil
}
func (m *runTriggerPermMock) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *runTriggerPermMock) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (m *runTriggerPermMock) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func setupRunTriggerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE workspaces (
		id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT, auto_apply INTEGER DEFAULT 0
	)`)
	_ = db.Exec(`CREATE TABLE run_triggers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_workspace_id TEXT,
		target_workspace_id TEXT,
		enabled INTEGER,
		trigger_condition TEXT,
		created_by TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name) VALUES (1,'ws-a','A'),(2,'ws-b','B')`)
	_ = db.Exec(`INSERT INTO run_triggers (id, source_workspace_id, target_workspace_id, enabled, created_at, updated_at)
		VALUES (10,'ws-a','ws-b',1,?,?)`, time.Now(), time.Now())
	return db
}

func TestRunTrigger_UpdateDelete_BindSourceWorkspace(t *testing.T) {
	db := setupRunTriggerDB(t)
	// iam nil → create will 500; update/delete path tests don't need create
	h := NewRunTriggerHandler(db, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/workspaces/:id/run-triggers/:trigger_id", h.UpdateRunTrigger)
	r.DELETE("/workspaces/:id/run-triggers/:trigger_id", h.DeleteRunTrigger)

	// cross workspace update → 404
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/workspaces/ws-b/run-triggers/10", bytes.NewBufferString(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross update want 404 got %d %s", w.Code, w.Body.String())
	}

	// same source ok
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("PUT", "/workspaces/ws-a/run-triggers/10", bytes.NewBufferString(`{"enabled":false}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("same source update: %d %s", w2.Code, w2.Body.String())
	}

	// cross delete → 404 (re-seed)
	_ = db.Exec(`UPDATE run_triggers SET enabled = 1 WHERE id = 10`)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("DELETE", "/workspaces/ws-b/run-triggers/10", nil))
	if w3.Code != http.StatusNotFound {
		t.Fatalf("cross delete want 404 got %d", w3.Code)
	}

	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, httptest.NewRequest("DELETE", "/workspaces/ws-a/run-triggers/10", nil))
	if w4.Code != 200 {
		t.Fatalf("same source delete: %d %s", w4.Code, w4.Body.String())
	}

	_ = models.RunTrigger{}
}

func TestCreateRunTrigger_RequiresTargetWrite(t *testing.T) {
	db := setupRunTriggerDB(t)
	// 另加 ws-c 作 target，避免与 seed 的 ws-a→ws-b 冲突
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name) VALUES (3,'ws-c','C')`)
	mock := &runTriggerPermMock{allow: map[string]bool{"ws-a": true}} // source only
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(mock)
	h := NewRunTriggerHandler(db, iam)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/workspaces/:id/run-triggers", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, h.CreateRunTrigger)

	body := `{"target_workspace_id":"ws-c"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/workspaces/ws-a/run-triggers", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("no target write want 403 got %d %s", w.Code, w.Body.String())
	}
	if mock.last == nil || mock.last.ScopeIDStr != "ws-c" {
		t.Fatalf("should check target ws-c: %+v", mock.last)
	}

	// allow target → 201
	mock.allow["ws-c"] = true
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/workspaces/ws-a/run-triggers", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("with target write want 201 got %d %s", w2.Code, w2.Body.String())
	}
}

func TestCreateRunTrigger_MissingIAM(t *testing.T) {
	db := setupRunTriggerDB(t)
	h := NewRunTriggerHandler(db, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/workspaces/:id/run-triggers", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, h.CreateRunTrigger)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/workspaces/ws-a/run-triggers", bytes.NewBufferString(`{"target_workspace_id":"ws-b"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", w.Code)
	}
}
