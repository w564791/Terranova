package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWorkspaceProjectHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE organizations (id INTEGER PRIMARY KEY);
CREATE TABLE projects (
  id INTEGER PRIMARY KEY,
  org_id INTEGER,
  name TEXT,
  display_name TEXT,
  description TEXT,
  is_default INTEGER,
  is_active INTEGER,
  settings TEXT,
  created_at DATETIME,
  updated_at DATETIME
);
CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER);
CREATE TABLE workspaces (
  workspace_id TEXT PRIMARY KEY,
  name TEXT,
  description TEXT,
  execution_mode TEXT,
  state TEXT,
  created_at TEXT,
  updated_at TEXT
);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
INSERT INTO organizations (id) VALUES (1), (2);
INSERT INTO projects (id, org_id, name, display_name, description, is_default, is_active, created_at, updated_at)
VALUES (101, 1, 'default-a', 'Default A', '', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
       (202, 2, 'default-b', 'Default B', '', 1, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
INSERT INTO workspaces (workspace_id, name, description, execution_mode, state, created_at, updated_at)
VALUES
  ('ws-org-a', 'A workspace', '', 'remote', 'active', 'now', 'now'),
  ('ws-org-b', 'B workspace', '', 'remote', 'active', 'now', 'now'),
  ('ws-unbound', 'unbound workspace', '', 'remote', 'active', 'now', 'now');
INSERT INTO workspace_project_relations (workspace_id, project_id)
VALUES ('ws-org-a', 101), ('ws-org-b', 202);`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestListProjectsWithWorkspaceCountUsesAuthOrgAndExcludesUnbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWorkspaceProjectHandlerDB(t)
	h := NewWorkspaceProjectHandler(db)

	r := gin.New()
	r.GET("/projects", func(c *gin.Context) {
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.ListProjectsWithWorkspaceCount)

	t.Run("uses authenticated organization when query is omitted", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
		}
		var body struct {
			Projects []struct {
				ID             uint `json:"id"`
				WorkspaceCount int  `json:"workspace_count"`
			} `json:"projects"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Projects) != 1 || body.Projects[0].ID != 101 || body.Projects[0].WorkspaceCount != 1 {
			t.Fatalf("must return only tenant A's assigned workspace (not global unbound count): %+v", body.Projects)
		}
		if strings.Contains(w.Body.String(), "default-b") || strings.Contains(w.Body.String(), "ws-unbound") {
			t.Fatalf("foreign or unbound data leaked: %s", w.Body.String())
		}
	})

	t.Run("foreign query is concealed", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects?org_id=2", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404 got %d %s", w.Code, w.Body.String())
		}
	})
}

func TestListProjectWorkspaces_BindsProjectToAuthOrg(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupWorkspaceProjectHandlerDB(t)
	h := NewWorkspaceProjectHandler(db)

	r := gin.New()
	r.GET("/projects/:id/workspaces", func(c *gin.Context) {
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, h.ListProjectWorkspaces)

	t.Run("own organization project is listed", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/101/workspaces", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "ws-org-a") {
			t.Fatalf("expected own workspace, got %s", w.Body.String())
		}
	})

	t.Run("foreign project is concealed", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/projects/202/workspaces", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404 got %d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "ws-org-b") {
			t.Fatalf("foreign workspace leaked: %s", w.Body.String())
		}
	})
}
