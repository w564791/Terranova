package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"
	"iac-platform/services"
)

func planDataTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "plan-data-handler-test")
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.WorkspaceTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// Agent channel: the uploaded plan is stored as an envelope, handed back
// decrypted only through the agent plan-task endpoint, and deleted when the
// task reaches a terminal status.
func TestAgentPlanData_EncryptedAtRestAndDeletedAfterApply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := planDataTestDB(t)
	task := models.WorkspaceTask{WorkspaceID: "ws-1", TaskType: models.TaskTypePlanAndApply, Status: models.TaskStatusRunning, ExecutionMode: models.ExecutionModeAgent}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	h := NewAgentHandler(db, nil, nil)
	r := gin.New()
	r.POST("/tasks/:task_id/plan-data", h.UploadPlanData)
	r.GET("/tasks/:task_id/plan-task", h.GetPlanTask)
	r.PUT("/tasks/:task_id/status", h.UpdateTaskStatus)
	do := func(method, path string, body interface{}) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	plan := []byte("PK\x03\x04 plan with db_password=hunter2-secret")
	id := func() string { return "/tasks/" + jsonNum(task.ID) }

	if w := do("POST", id()+"/plan-data", map[string]string{"plan_data": base64.StdEncoding.EncodeToString(plan)}); w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	var stored []byte
	db.Raw(`SELECT plan_data FROM workspace_tasks WHERE id = ?`, task.ID).Row().Scan(&stored)
	if sealed, _ := crypto.IsSealedPlanData(stored); !sealed || bytes.Contains(stored, []byte("hunter2")) {
		t.Fatalf("plan_data stored in clear: %q", stored[:16])
	}

	w := do("GET", id()+"/plan-task", nil)
	var resp struct {
		PlanData string `json:"plan_data"`
		Task     map[string]interface{}
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if got, _ := base64.StdEncoding.DecodeString(resp.PlanData); !bytes.Equal(got, plan) {
		t.Fatalf("agent did not get the plan back: %d %s", w.Code, w.Body)
	}
	if _, leaked := resp.Task["plan_data"]; leaked {
		t.Fatal("task object carries plan_data")
	}

	if w := do("PUT", id()+"/status", map[string]string{"status": string(models.TaskStatusApplied)}); w.Code != 200 {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}
	stored = nil
	db.Raw(`SELECT plan_data FROM workspace_tasks WHERE id = ?`, task.ID).Row().Scan(&stored)
	if len(stored) != 0 {
		t.Fatal("plan_data kept after the task was applied")
	}
}

func jsonNum(id uint) string { b, _ := json.Marshal(id); return string(b) }

// No API response may carry plan_data: the model never serializes it, and the
// only handler that emits the key is the agent plan-task endpoint above.
func TestPlanData_NeverSerialized(t *testing.T) {
	b, _ := json.Marshal(models.WorkspaceTask{ID: 1, PlanData: []byte("PK\x03\x04secret")})
	if strings.Contains(string(b), "plan_data") || strings.Contains(string(b), "c2VjcmV0") {
		t.Fatalf("WorkspaceTask JSON exposes plan_data: %s", b)
	}
	allowed := map[string]bool{filepath.Join("internal", "handlers", "agent_handler.go"): true}
	for _, dir := range []string{filepath.Join("..", "..", "controllers"), filepath.Join("..", "..", "internal", "handlers")} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			rel, _ := filepath.Rel(filepath.Join("..", ".."), f)
			src, _ := os.ReadFile(f)
			if (strings.Contains(string(src), `"plan_data"`) || strings.Contains(string(src), ".PlanData")) && !allowed[rel] {
				t.Errorf("%s references plan_data; only the agent plan-task / plan-data endpoints may", rel)
			}
		}
	}
}

var _ = services.OpenTaskPlanData
