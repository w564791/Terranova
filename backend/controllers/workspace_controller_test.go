package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWorkspaceCtrlDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	// 覆盖 list/detail 查询所需最小列（含 updated_at 排序）
	_ = db.Exec(`CREATE TABLE IF NOT EXISTS workspaces (
		id INTEGER PRIMARY KEY,
		workspace_id TEXT,
		name TEXT,
		description TEXT,
		state_backend TEXT,
		terraform_version TEXT,
		execution_mode TEXT,
		state TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	return db
}

func TestWorkspaceController_GetWorkspaces_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWorkspaceCtrlDB(t)
	ctrl := NewWorkspaceController(
		services.NewWorkspaceService(db),
		services.NewWorkspaceOverviewService(db),
		nil,
	)
	r := gin.New()
	r.GET("/workspaces", ctrl.GetWorkspaces)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces", nil))
	// 禁止 nil gorm.DB panic；空表应 200 或业务 4xx，5xx 视为失败
	if w.Code == 0 {
		t.Fatal("no response (possible panic)")
	}
	if w.Code >= 500 {
		t.Fatalf("list workspaces 5xx: %d %s", w.Code, w.Body.String())
	}
}

func TestWorkspaceController_GetWorkspaces_UsesScopedIAMAllowList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWorkspaceCtrlDB(t)
	now := time.Now()
	for _, stmt := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`,
		`INSERT INTO projects (id, org_id) VALUES (1, 1), (2, 1)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id, name, state_backend, created_at, updated_at) VALUES
		(10, 'ws-allowed', 'allowed', 'local', ?, ?),
		(20, 'ws-sibling', 'sibling', 'local', ?, ?)`, now, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES
		('ws-allowed', 1), ('ws-sibling', 2)`).Error; err != nil {
		t.Fatal(err)
	}

	ctrl := NewWorkspaceController(
		services.NewWorkspaceService(db),
		services.NewWorkspaceOverviewService(db),
		nil,
	)
	r := gin.New()
	r.GET("/workspaces", func(c *gin.Context) {
		c.Set("auth_org_id", uint(1))
		c.Set(service.WorkspaceListAccessContextKey, &service.WorkspaceListAccess{
			WorkspaceIDs: []string{"ws-allowed"},
		})
		c.Next()
	}, ctrl.GetWorkspaces)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list failed: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Items []struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Total != 1 || len(body.Data.Items) != 1 || body.Data.Items[0].WorkspaceID != "ws-allowed" {
		t.Fatalf("scoped list leaked a sibling: %+v", body.Data)
	}
}

func TestWorkspaceController_GetWorkspace_NoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWorkspaceCtrlDB(t)
	ctrl := NewWorkspaceController(
		services.NewWorkspaceService(db),
		services.NewWorkspaceOverviewService(db),
		nil,
	)
	r := gin.New()
	r.GET("/workspaces/:id", ctrl.GetWorkspace)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces/ws-x", nil))
	if w.Code == 0 {
		t.Fatal("no response")
	}
	// 不存在时不得 200
	if w.Code == http.StatusOK {
		t.Fatalf("missing workspace must not 200: %s", w.Body.String())
	}
	if w.Code >= 500 {
		t.Fatalf("get workspace 5xx: %d %s", w.Code, w.Body.String())
	}
}
