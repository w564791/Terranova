package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"iac-platform/internal/models"
)

const rcSecret = "override-secret-value-77"

func rcUpload() map[string]interface{} {
	return map[string]interface{}{"resource_changes": []map[string]interface{}{{
		"resource_address": "aws_s3_bucket.b", "resource_type": "aws_s3_bucket", "resource_name": "b", "action": "update",
		"changes_before": map[string]interface{}{"tag": "old"},
		"changes_after":  map[string]interface{}{"tag": rcSecret, "injected": "agent-only-value"},
		"after_unknown":  map[string]interface{}{"x": rcSecret},
	}}}
}

func rcPost(t *testing.T, h *AgentHandler, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.POST("/tasks/:task_id/parse-plan-changes", h.ParsePlanChanges)
	r.POST("/tasks/:task_id/plan-json", h.UploadPlanJSON)
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func rcStored(t *testing.T, h *AgentHandler, taskID uint) (string, []models.WorkspaceTaskResourceChange) {
	t.Helper()
	var rows []models.WorkspaceTaskResourceChange
	h.db.Where("task_id = ?", taskID).Order("id").Find(&rows)
	b, _ := json.Marshal(rows)
	return string(b), rows
}

// Uploaded resource changes are discarded: values are derived by the platform
// from the (redacted) plan_json, or not stored at all without one.
func TestParsePlanChanges_DiscardsUploadedValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := planDataTestDB(t)
	if err := db.AutoMigrate(&models.WorkspaceTaskResourceChange{}); err != nil {
		t.Fatal(err)
	}
	h := NewAgentHandler(db, nil, nil)
	newTask := func() *models.WorkspaceTask {
		task := &models.WorkspaceTask{WorkspaceID: "ws-1", TaskType: models.TaskTypePlan, Status: models.TaskStatusRunning,
			ExecutionMode: models.ExecutionModeAgent, VariableOverrides: models.JSONB{"ovr": rcSecret}} // sensitive_keys NULL => sensitive
		if err := db.Create(task).Error; err != nil {
			t.Fatal(err)
		}
		return task
	}

	// no plan_json: address/action only, no values
	t1 := newTask()
	if w := rcPost(t, h, "/tasks/"+jsonNum(t1.ID)+"/parse-plan-changes", rcUpload()); w.Code != 200 || !strings.Contains(w.Body.String(), "metadata_only") {
		t.Fatalf("metadata-only: %d %s", w.Code, w.Body)
	}
	s, rows := rcStored(t, h, t1.ID)
	if len(rows) != 1 || rows[0].ResourceAddress != "aws_s3_bucket.b" || rows[0].Action != "update" ||
		rows[0].ChangesBefore != nil || rows[0].ChangesAfter != nil || rows[0].AfterUnknown != nil ||
		strings.Contains(s, rcSecret) || strings.Contains(s, "agent-only-value") {
		t.Fatalf("metadata-only rows: %s", s)
	}

	// plan-json upload derives the changes (redacted); a later upload of the
	// agent's own parse is discarded and re-derived
	t2 := newTask()
	plan := map[string]interface{}{"plan_json": map[string]interface{}{
		"variables": map[string]interface{}{"ovr": map[string]interface{}{"value": rcSecret}},
		"resource_changes": []interface{}{map[string]interface{}{
			"address": "aws_s3_bucket.b", "type": "aws_s3_bucket", "name": "b",
			"change": map[string]interface{}{"actions": []interface{}{"update"}, "before": map[string]interface{}{"tag": "old"},
				"after": map[string]interface{}{"tag": rcSecret, "region": "eu-west-1"}, "after_sensitive": map[string]interface{}{}},
		}},
	}}
	if w := rcPost(t, h, "/tasks/"+jsonNum(t2.ID)+"/plan-json", plan); w.Code != 200 {
		t.Fatalf("plan-json: %d %s", w.Code, w.Body)
	}
	s, rows = rcStored(t, h, t2.ID)
	if len(rows) != 1 || strings.Contains(s, rcSecret) || !strings.Contains(s, "eu-west-1") {
		t.Fatalf("derived on plan-json upload: %s", s)
	}
	if w := rcPost(t, h, "/tasks/"+jsonNum(t2.ID)+"/parse-plan-changes", rcUpload()); w.Code != 200 ||
		!strings.Contains(w.Body.String(), `"uploaded_changes_discarded":1`) {
		t.Fatalf("parse-plan-changes: %d %s", w.Code, w.Body)
	}
	s, rows = rcStored(t, h, t2.ID)
	if len(rows) != 1 || strings.Contains(s, rcSecret) || strings.Contains(s, "agent-only-value") || !strings.Contains(s, "eu-west-1") {
		t.Fatalf("re-derived rows: %s", s)
	}
	if !strings.Contains(s, `"details_purged":false`) {
		t.Fatalf("details_purged must be in the API shape: %s", s)
	}
}

// C&C task messages are only accepted for tasks assigned to the connection's
// agent.
func TestRawCC_TaskMessagesBoundToAgent(t *testing.T) {
	db := planDataTestDB(t)
	agentB := "agent-b"
	task := models.WorkspaceTask{WorkspaceID: "ws-1", TaskType: models.TaskTypePlan, Status: models.TaskStatusRunning,
		ExecutionMode: models.ExecutionModeAgent, AgentID: &agentB}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	h := NewRawAgentCCHandler(db, nil)
	payload := map[string]interface{}{"task_id": float64(task.ID)}
	if h.agentOwnsTaskMessage(&RawAgentConnection{AgentID: "agent-a"}, "log_stream", payload) {
		t.Fatal("agent-a must not report on agent-b's task")
	}
	connB := &RawAgentConnection{AgentID: agentB}
	if !h.agentOwnsTaskMessage(connB, "task_completed", payload) {
		t.Fatal("agent-b owns the task")
	}
	db.Model(&task).Updates(map[string]interface{}{"status": models.TaskStatusSuccess, "completed_at": hoursAgo(2)})
	if !h.agentOwnsTaskMessage(connB, "log_stream", payload) {
		t.Fatal("cached ownership within TTL")
	}
	if h.agentOwnsTaskMessage(&RawAgentConnection{AgentID: agentB}, "log_stream", payload) {
		t.Fatal("task ended 2h ago: messages dropped")
	}
}

func hoursAgo(h int) interface{} { return time.Now().Add(-time.Duration(h) * time.Hour) }
