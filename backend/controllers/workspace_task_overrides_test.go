package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTaskOverridesRouter(t *testing.T, canRead bool) *gin.Engine {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT)`,
		`CREATE TABLE workspace_tasks (
			id INTEGER PRIMARY KEY, workspace_id TEXT, status TEXT, stage TEXT, task_type TEXT, description TEXT,
			created_at DATETIME, created_by TEXT, changes_add INTEGER, changes_change INTEGER, changes_destroy INTEGER,
			started_at DATETIME, completed_at DATETIME, is_background BOOLEAN,
			variable_overrides BLOB, sensitive_keys BLOB, error_message TEXT, error_code TEXT)`,
		`INSERT INTO workspaces (id, workspace_id, name) VALUES (1, 'ws-a', 'A')`,
		// 20: computed sensitive_keys; 21: NULL sensitive_keys (all sensitive)
		`INSERT INTO workspace_tasks (id, workspace_id, status, task_type, created_at, variable_overrides, sensitive_keys) VALUES
		   (20, 'ws-a', 'success', 'plan', '2026-10-01 00:00:00', CAST('{"db_password":"task-SECRET","region":"eu-central-1"}' AS BLOB), CAST('["db_password"]' AS BLOB)),
		   (21, 'ws-a', 'success', 'plan', '2026-10-02 00:00:00', CAST('{"region":"legacy-VISIBLE"}' AS BLOB), NULL)`,
		`INSERT INTO workspace_tasks (id, workspace_id, status, task_type, created_at, error_message, error_code) VALUES
		   (22, 'ws-a', 'failed', 'plan', '2026-10-03 00:00:00', 'bundle_republish_required: denylisted_file @ prod.tfvars', 'bundle_republish_required')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	ctrl := &WorkspaceTaskController{db: db, CanReadVariableValues: func(*gin.Context, string) bool { return canRead }}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/workspaces/:id/tasks", ctrl.GetTasks)
	r.GET("/workspaces/:id/tasks/:task_id", ctrl.GetTask)
	return r
}

func getBody(t *testing.T, r http.Handler, path string) string {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestTaskDetailAndList_RedactOverrides(t *testing.T) {
	r := setupTaskOverridesRouter(t, true)
	for _, path := range []string{"/workspaces/ws-a/tasks/20", "/workspaces/ws-a/tasks/21", "/workspaces/ws-a/tasks"} {
		body := getBody(t, r, path)
		for _, leak := range []string{"task-SECRET", "legacy-VISIBLE", "variable_overrides", "sensitive_keys"} {
			if strings.Contains(body, leak) {
				t.Fatalf("%s leaks %q: %s", path, leak, body)
			}
		}
		if !strings.Contains(body, `"key":"db_password"`) && path != "/workspaces/ws-a/tasks/21" {
			t.Fatalf("%s: overrides view missing: %s", path, body)
		}
	}
	if body := getBody(t, r, "/workspaces/ws-a/tasks/20"); !strings.Contains(body, `"value":"eu-central-1"`) {
		t.Fatalf("non-sensitive value visible with READ: %s", body)
	}
}

func TestTaskDetailAndList_NoValuesWithoutRead(t *testing.T) {
	r := setupTaskOverridesRouter(t, false)
	for _, path := range []string{"/workspaces/ws-a/tasks/20", "/workspaces/ws-a/tasks"} {
		body := getBody(t, r, path)
		if strings.Contains(body, `"value"`) || strings.Contains(body, "eu-central-1") || strings.Contains(body, "task-SECRET") {
			t.Fatalf("%s: values must be hidden without WORKSPACE_VARIABLES READ: %s", path, body)
		}
		if !strings.Contains(body, `"has_value":true`) {
			t.Fatalf("%s: keys still listed: %s", path, body)
		}
	}
}

func TestTaskDetailAndList_ExposeErrorCode(t *testing.T) {
	r := setupTaskOverridesRouter(t, false)
	body := getBody(t, r, "/workspaces/ws-a/tasks/22")
	if !strings.Contains(body, `"error_code":"bundle_republish_required"`) ||
		!strings.Contains(body, `"error_message":"bundle_republish_required: denylisted_file @ prod.tfvars"`) {
		t.Fatalf("task detail must carry error_code next to error_message: %s", body)
	}
	if body := getBody(t, r, "/workspaces/ws-a/tasks"); !strings.Contains(body, `"error_code":"bundle_republish_required"`) {
		t.Fatalf("task list must carry error_code: %s", body)
	}
}
