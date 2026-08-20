package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLoadTaskInPathWorkspace_CrossWorkspaceDenied(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	// minimal tables
	_ = db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT)`)
	_ = db.Exec(`CREATE TABLE workspace_tasks (
		id INTEGER PRIMARY KEY, workspace_id TEXT, status TEXT, stage TEXT
	)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name) VALUES (1,'ws-a','A'),(2,'ws-b','B')`)
	_ = db.Exec(`INSERT INTO workspace_tasks (id, workspace_id, status) VALUES (10,'ws-b','pending')`)

	ctrl := &WorkspaceTaskController{db: db}
	gin.SetMode(gin.TestMode)

	// path claims ws-a but task belongs to ws-b
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/workspaces/ws-a/tasks/10", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-a"}}

	_, _, ok := ctrl.loadTaskInPathWorkspace(c, 10)
	if ok {
		t.Fatal("cross-workspace task must be rejected")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 got %d body=%s", w.Code, w.Body.String())
	}
}

func TestLoadTaskInPathWorkspace_SameWorkspaceOK(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT)`)
	_ = db.Exec(`CREATE TABLE workspace_tasks (
		id INTEGER PRIMARY KEY, workspace_id TEXT, status TEXT, stage TEXT
	)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name) VALUES (1,'ws-a','A')`)
	_ = db.Exec(`INSERT INTO workspace_tasks (id, workspace_id, status) VALUES (11,'ws-a','pending')`)

	ctrl := &WorkspaceTaskController{db: db}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/workspaces/ws-a/tasks/11", nil)
	c.Params = gin.Params{{Key: "id", Value: "ws-a"}}

	ws, task, ok := ctrl.loadTaskInPathWorkspace(c, 11)
	if !ok {
		t.Fatalf("expected ok, code=%d body=%s", w.Code, w.Body.String())
	}
	if ws.WorkspaceID != "ws-a" || task.ID != 11 {
		t.Fatalf("ws=%+v task=%+v", ws, task)
	}
}

func TestAIChildEndpointsRejectTaskOutsidePathWorkspace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT)`)
	_ = db.Exec(`CREATE TABLE workspace_tasks (
		id INTEGER PRIMARY KEY, workspace_id TEXT, status TEXT, stage TEXT
	)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id, name) VALUES (1,'ws-a','A'),(2,'ws-b','B')`)
	_ = db.Exec(`INSERT INTO workspace_tasks (id, workspace_id, status) VALUES (10,'ws-b','pending')`)

	gin.SetMode(gin.TestMode)
	for name, invoke := range map[string]func(*gin.Context){
		"error-analysis":     NewAIController(db).GetTaskAnalysis,
		"retry-plan-summary": NewAISummaryController(db).RetryPlanSummary,
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/workspaces/ws-a/tasks/10", nil)
			c.Params = gin.Params{{Key: "id", Value: "ws-a"}, {Key: "task_id", Value: "10"}}

			invoke(c)
			if w.Code != http.StatusNotFound {
				t.Fatalf("cross-workspace task must be rejected before child lookup: got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// ensure models package referenced for compile stability if fields change
var _ = models.Workspace{}
