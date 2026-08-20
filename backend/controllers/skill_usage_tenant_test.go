package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSkillUsageTenantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT UNIQUE, name TEXT)`,
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER NOT NULL)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT NOT NULL, project_id INTEGER NOT NULL)`,
		`CREATE TABLE workspace_tasks (id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL)`,
		`CREATE TABLE skill_usage_logs (
			id TEXT PRIMARY KEY,
			skill_ids TEXT NOT NULL,
			capability TEXT NOT NULL,
			workspace_id TEXT,
			user_id TEXT NOT NULL,
			module_id INTEGER,
			execution_time_ms INTEGER,
			user_feedback INTEGER,
			ai_model TEXT,
			context_summary TEXT,
			response_summary TEXT,
			created_at DATETIME,
			input_snapshot BLOB,
			output_snapshot BLOB,
			skill_content_hash TEXT,
			skill_content_snapshot TEXT,
			user_action TEXT,
			user_modification_diff TEXT,
			latency_ms INTEGER,
			assessment_status TEXT
		)`,
		`INSERT INTO workspaces (id, workspace_id, name) VALUES
			(1, 'ws-org-1', 'org 1'), (2, 'ws-org-2', 'org 2'), (3, 'ws-ambiguous', 'ambiguous')`,
		`INSERT INTO projects (id, org_id) VALUES (101, 1), (202, 2), (102, 1)`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES
			('ws-org-1', 101), ('ws-org-2', 202), ('ws-ambiguous', 101), ('ws-ambiguous', 102)`,
		`INSERT INTO workspace_tasks (id, workspace_id) VALUES (1, 'ws-org-1'), (2, 'ws-org-2')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("fixture failed: %v\n%s", err, statement)
		}
	}
	return db
}

func insertSkillUsageTenantLog(t *testing.T, db *gorm.DB, id, capability, workspaceID, userID, snapshot string, action *string) {
	t.Helper()
	var inputSnapshot interface{}
	if snapshot != "" {
		inputSnapshot = []byte(snapshot)
	}
	var userAction interface{}
	if action != nil {
		userAction = *action
	}
	if err := db.Exec(`
		INSERT INTO skill_usage_logs
		(id, skill_ids, capability, workspace_id, user_id, created_at, input_snapshot, user_action, assessment_status)
		VALUES (?, '[]', ?, ?, ?, ?, ?, ?, 'pending')`,
		id, capability, workspaceID, userID, time.Now(), inputSnapshot, userAction).Error; err != nil {
		t.Fatal(err)
	}
}

func skillUsageTenantRouter(db *gorm.DB, userID string, orgID uint) *gin.Engine {
	controller := NewSkillController(db)
	router := gin.New()
	auth := func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("auth_org_id", orgID)
		c.Next()
	}
	router.PUT("/skill-usage/:id/action", auth, controller.UpdateSkillUsageAction)
	router.PUT("/skill-usage/by-capability", auth, controller.UpdateSkillUsageByCapability)
	router.GET("/skill-usage/pending-feedback", auth, controller.GetPendingFeedback)
	router.PUT("/skill-usage/:id/feedback", auth, controller.SubmitFeedback)
	return router
}

func skillUsageFeedback(t *testing.T, router *gin.Engine, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func TestSkillUsageSystemLogsAreTenantBoundAcrossFeedbackEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSkillUsageTenantDB(t)
	action := "accepted"
	insertSkillUsageTenantLog(t, db, "system-local", "plan_summary", "ws-org-1", "system", `{"workspace_id":"ws-org-1","task_id":1}`, &action)
	insertSkillUsageTenantLog(t, db, "system-foreign", "plan_summary", "ws-org-2", "system", `{"workspace_id":"ws-org-2","task_id":2}`, &action)
	insertSkillUsageTenantLog(t, db, "system-unscoped", "plan_summary", "", "system", "", &action)

	router := skillUsageTenantRouter(db, "user-org-1", 1)

	// The pending endpoint used to return every system log. It must return only
	// records whose workspace/task resolves uniquely to the selected org.
	pending := httptest.NewRecorder()
	router.ServeHTTP(pending, httptest.NewRequest(http.MethodGet, "/skill-usage/pending-feedback", nil))
	if pending.Code != http.StatusOK {
		t.Fatalf("pending feedback: want 200 got %d: %s", pending.Code, pending.Body.String())
	}
	var payload struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(pending.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "system-local" {
		t.Fatalf("foreign/unscoped system log leaked through pending feedback: %+v", payload.Items)
	}

	foreignAction := skillUsageFeedback(t, router, "/skill-usage/system-foreign/action", `{"feedback":5}`)
	if foreignAction.Code != http.StatusNotFound {
		t.Fatalf("foreign system action: want 404 got %d: %s", foreignAction.Code, foreignAction.Body.String())
	}
	foreignFeedback := skillUsageFeedback(t, router, "/skill-usage/system-foreign/feedback", `{"feedback":5}`)
	if foreignFeedback.Code != http.StatusNotFound {
		t.Fatalf("foreign system feedback: want 404 got %d: %s", foreignFeedback.Code, foreignFeedback.Body.String())
	}

	localFeedback := skillUsageFeedback(t, router, "/skill-usage/system-local/feedback", `{"feedback":4}`)
	if localFeedback.Code != http.StatusOK {
		t.Fatalf("local system feedback: want 200 got %d: %s", localFeedback.Code, localFeedback.Body.String())
	}
	var foreignRating, localRating *int
	if err := db.Table("skill_usage_logs").Select("user_feedback").Where("id = ?", "system-foreign").Scan(&foreignRating).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("skill_usage_logs").Select("user_feedback").Where("id = ?", "system-local").Scan(&localRating).Error; err != nil {
		t.Fatal(err)
	}
	if foreignRating != nil {
		t.Fatalf("foreign system log was modified: %v", *foreignRating)
	}
	if localRating == nil || *localRating != 4 {
		t.Fatalf("local system log was not updated: %v", localRating)
	}
}

func TestSkillUsageByCapabilityBindsBodyTaskToAuthenticatedOrg(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupSkillUsageTenantDB(t)
	insertSkillUsageTenantLog(t, db, "capability-local", "apply_summary", "ws-org-1", "system", `{"workspace_id":"ws-org-1","task_id":1}`, nil)
	insertSkillUsageTenantLog(t, db, "capability-foreign", "apply_summary", "ws-org-2", "system", `{"workspace_id":"ws-org-2","task_id":2}`, nil)
	router := skillUsageTenantRouter(db, "user-org-1", 1)

	foreign := skillUsageFeedback(t, router, "/skill-usage/by-capability", `{"capability":"apply_summary","task_id":2,"action":"accepted"}`)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("guessed foreign task: want 404 got %d: %s", foreign.Code, foreign.Body.String())
	}
	local := skillUsageFeedback(t, router, "/skill-usage/by-capability", `{"capability":"apply_summary","task_id":1,"action":"accepted"}`)
	if local.Code != http.StatusOK {
		t.Fatalf("local task: want 200 got %d: %s", local.Code, local.Body.String())
	}
}
