package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type taskLogPermMock struct {
	allow map[string]bool
	last  *service.CheckPermissionRequest
}

func (m *taskLogPermMock) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	m.last = req
	ok := m.allow != nil && m.allow[req.ScopeIDStr]
	lv := valueobject.PermissionLevelNone
	if ok {
		lv = valueobject.PermissionLevelRead
	}
	return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: lv, DenyReason: "No permission"}, nil
}
func (m *taskLogPermMock) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, taskID *uint) (*service.CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *taskLogPermMock) CheckBatchPermissions(ctx context.Context, reqs []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (m *taskLogPermMock) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func setupTaskLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE workspaces (
		id INTEGER PRIMARY KEY,
		workspace_id TEXT UNIQUE,
		name TEXT
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE projects (
		id INTEGER PRIMARY KEY,
		org_id INTEGER NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE workspace_project_relations (
		workspace_id TEXT NOT NULL,
		project_id INTEGER NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE workspace_tasks (
		id INTEGER PRIMARY KEY,
		workspace_id TEXT,
		task_type TEXT,
		status TEXT,
		plan_output TEXT,
		apply_output TEXT,
		error_message TEXT,
		created_at DATETIME,
		completed_at DATETIME,
		duration INTEGER
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id, name)
		VALUES (1, 'ws-a', 'A'), (2, 'ws-b', 'B')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO projects (id, org_id) VALUES (101, 1), (202, 2)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspace_project_relations (workspace_id, project_id)
		VALUES ('ws-a', 101), ('ws-b', 202)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspace_tasks (id, workspace_id, task_type, status, plan_output)
		VALUES (1, 'ws-a', 'plan', 'success', 'plan ok'),
		       (2, 'ws-b', 'plan', 'success', 'secret plan of b')`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTaskLogController_CrossWorkspaceDenied(t *testing.T) {
	db := setupTaskLogDB(t)
	mock := &taskLogPermMock{allow: map[string]bool{"ws-a": true}}
	iam := middleware.NewIAMPermissionMiddlewareWithChecker(mock)
	ctrl := NewTaskLogController(db, iam)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/logs", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, ctrl.GetTaskLogs)

	// task 2 is ws-b — user only has ws-a
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/2/logs", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d body=%s", w.Code, w.Body.String())
	}
	if mock.last == nil || mock.last.ScopeIDStr != "ws-b" {
		t.Fatalf("should check ws-b: %+v", mock.last)
	}

	// task 1 ws-a allowed
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/tasks/1/logs", nil))
	if w2.Code != 200 {
		t.Fatalf("want 200 got %d %s", w2.Code, w2.Body.String())
	}
}

func TestTaskLogController_MissingIAM(t *testing.T) {
	db := setupTaskLogDB(t)
	ctrl := NewTaskLogController(db, nil)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/logs", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, ctrl.GetTaskLogs)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/1/logs", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", w.Code)
	}
}

func TestTaskLogController_RejectsSelectedOrganizationMismatch(t *testing.T) {
	db := setupTaskLogDB(t)
	mock := &taskLogPermMock{allow: map[string]bool{"ws-a": true}}
	ctrl := NewTaskLogController(db, middleware.NewIAMPermissionMiddlewareWithChecker(mock))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/logs/download", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, ctrl.DownloadTaskLogs)

	// Task 1 belongs to org 1. A selected org 2 must not reveal the task,
	// even if the caller otherwise has workspace access.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/1/logs/download?org_id=2", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
	if mock.last != nil {
		t.Fatalf("IAM must not run before org binding succeeds: %+v", mock.last)
	}
}

func TestTaskLogController_RejectsContextOrganizationMismatch(t *testing.T) {
	db := setupTaskLogDB(t)
	mock := &taskLogPermMock{allow: map[string]bool{"ws-a": true}}
	ctrl := NewTaskLogController(db, middleware.NewIAMPermissionMiddlewareWithChecker(mock))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/logs", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Set("auth_org_id", uint(2))
		c.Next()
	}, ctrl.GetTaskLogs)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/1/logs", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
	if mock.last != nil {
		t.Fatalf("IAM must not run before org binding succeeds: %+v", mock.last)
	}
}

func TestTaskLogController_RejectsAmbiguousWorkspaceProjectBinding(t *testing.T) {
	db := setupTaskLogDB(t)
	if err := db.Exec(`INSERT INTO projects (id, org_id) VALUES (102, 1)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES ('ws-a', 102)`).Error; err != nil {
		t.Fatal(err)
	}
	mock := &taskLogPermMock{allow: map[string]bool{"ws-a": true}}
	ctrl := NewTaskLogController(db, middleware.NewIAMPermissionMiddlewareWithChecker(mock))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tasks/:task_id/logs", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, ctrl.GetTaskLogs)

	// Two project bindings make scope resolution ambiguous. Do not silently
	// select either project or expose task output.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/1/logs", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
	if mock.last != nil {
		t.Fatalf("IAM must not run for an ambiguous workspace binding: %+v", mock.last)
	}
}

func TestAIAnalyzeErrorBindsGlobalTaskToAuthenticatedOrganization(t *testing.T) {
	db := setupTaskLogDB(t)
	controller := NewAIController(db)
	gin.SetMode(gin.TestMode)

	// Task 2 belongs to organization 2. The AI route only has a body task ID,
	// so it must bind the ID before the analysis service reads the task output.
	crossWriter := httptest.NewRecorder()
	crossCtx, _ := gin.CreateTestContext(crossWriter)
	crossCtx.Request = httptest.NewRequest(http.MethodPost, "/ai/analyze-error", nil)
	crossCtx.Set("auth_org_id", uint(1))
	if controller.ensureTaskInAuthenticatedOrg(crossCtx, 2) {
		t.Fatal("cross-tenant global task must be rejected")
	}
	if crossWriter.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", crossWriter.Code, crossWriter.Body.String())
	}

	ownerWriter := httptest.NewRecorder()
	ownerCtx, _ := gin.CreateTestContext(ownerWriter)
	ownerCtx.Request = httptest.NewRequest(http.MethodPost, "/ai/analyze-error", nil)
	ownerCtx.Set("auth_org_id", uint(1))
	if !controller.ensureTaskInAuthenticatedOrg(ownerCtx, 1) {
		t.Fatalf("same-tenant task should pass: %d %s", ownerWriter.Code, ownerWriter.Body.String())
	}
}
