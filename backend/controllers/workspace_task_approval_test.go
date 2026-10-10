package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ConfirmApply on manifest deployments is the approval (spec §9 step 7):
// only purpose=approval, runner=agent runs are accepted, and the approval
// binds the run's bundle and the plan.out hash.
type confirmApplyEnv struct {
	db   *gorm.DB
	ctrl *WorkspaceTaskController
	hash string
}

func setupConfirmApplyEnv(t *testing.T) *confirmApplyEnv {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret-for-confirm-apply-approval")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&models.Workspace{}, &models.WorkspaceTask{}); err != nil {
		t.Fatal(err)
	}
	files := []manifestbundle.File{{Path: "main.tf", Content: []byte("output \"x\" { value = 1 }\n"), Mode: 0o644}}
	hash, _ := manifestbundle.Hash(files)
	for _, stmt := range []string{
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, bundle_hash TEXT)`,
		`CREATE TABLE manifest_deployments (id TEXT PRIMARY KEY, manifest_id TEXT, version_id TEXT, workspace_id TEXT, approved_bundle_hash TEXT, approved_plan_hash TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_runs (id TEXT PRIMARY KEY, manifest_id TEXT, version_id TEXT, bundle_hash TEXT, workspace_id TEXT, runner TEXT, purpose TEXT, status TEXT, plan_hash TEXT, plan_redacted TEXT, state_serial INTEGER, session_id TEXT, agent_id TEXT, task_id INTEGER UNIQUE, approved_bundle_hash TEXT, approved_plan_hash TEXT, approved_by TEXT, approved_at DATETIME, created_by TEXT, created_at DATETIME, updated_at DATETIME)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(`INSERT INTO manifest_versions VALUES ('mfv-1','mf-1','v1',?)`, hash)
	db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id) VALUES ('mfd-1','mf-1','mfv-1','ws-m')`)
	dep, tag := "mfd-1", "v1"
	if err := db.Create(&models.Workspace{WorkspaceID: "ws-m", Name: "m", ManifestDeploymentID: &dep, ManifestActiveTag: &tag}).Error; err != nil {
		t.Fatal(err)
	}
	return &confirmApplyEnv{db: db, hash: hash, ctrl: &WorkspaceTaskController{
		db: db, executor: &services.TerraformExecutor{}, streamManager: services.NewOutputStreamManager(),
	}}
}

// plannedTask an apply_pending plan_and_apply task with a sealed plan and,
// unless runner is "", a manifest run of that runner / purpose.
func (e *confirmApplyEnv) plannedTask(t *testing.T, runner, purpose string) *models.WorkspaceTask {
	t.Helper()
	now := time.Now()
	task := &models.WorkspaceTask{WorkspaceID: "ws-m", TaskType: models.TaskTypePlanAndApply, Status: models.TaskStatusApplyPending,
		SnapshotCreatedAt: &now, SnapshotResourceVersions: models.JSONB{}}
	if err := e.db.Omit("state_token_hash").Create(task).Error; err != nil {
		t.Fatal(err)
	}
	plan := []byte("PLAN-" + t.Name() + runner + purpose)
	sealed, err := services.SealTaskPlanData(task.ID, plan)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(plan)
	e.db.Model(task).Updates(map[string]interface{}{"plan_data": sealed, "plan_hash": hex.EncodeToString(sum[:])})
	if runner != "" {
		var session interface{}
		if runner == "sandbox" {
			session = "sess-1"
		}
		if err := e.db.Exec(`INSERT INTO manifest_runs (id, manifest_id, version_id, bundle_hash, workspace_id, runner, purpose, status, session_id, task_id, created_by, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			"mfr-"+runner+purpose+strings.Repeat("0", 3), "mf-1", "mfv-1", e.hash, "ws-m", runner, purpose, "running", session, task.ID, "u1", now).Error; err != nil {
			t.Fatal(err)
		}
	}
	return task
}

func (e *confirmApplyEnv) confirm(t *testing.T, taskID uint) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"apply_description":"go"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "ws-m"}, {Key: "task_id", Value: jsonUint(taskID)}}
	c.Set("user_id", "u-approver")
	e.ctrl.ConfirmApply(c)
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func jsonUint(v uint) string { b, _ := json.Marshal(v); return string(b) }

func (e *confirmApplyEnv) confirmed(t *testing.T, taskID uint) bool {
	t.Helper()
	var task models.WorkspaceTask
	if err := e.db.Select("id", "apply_confirmed_by").First(&task, taskID).Error; err != nil {
		t.Fatal(err)
	}
	return task.ApplyConfirmedBy != nil
}

func TestConfirmApply_RejectsPreviewAndSandboxRuns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := setupConfirmApplyEnv(t)
	for _, rp := range [][2]string{{"sandbox", "preview"}, {"agent", "preview"}} {
		task := e.plannedTask(t, rp[0], rp[1])
		code, body := e.confirm(t, task.ID)
		if code != http.StatusConflict || body["error_code"] != services.ApprovalCodeNotApprovable {
			t.Fatalf("%v run: want 409 run_not_approvable, got %d %v", rp, code, body)
		}
		if e.confirmed(t, task.ID) {
			t.Fatalf("%v run: task must stay unconfirmed", rp)
		}
		var approved int64
		e.db.Raw(`SELECT count(*) FROM manifest_runs WHERE task_id = ? AND approved_at IS NOT NULL`, task.ID).Scan(&approved)
		if approved != 0 {
			t.Fatalf("%v run: approval recorded", rp)
		}
	}
	// manifest task without a run
	task := e.plannedTask(t, "", "")
	if code, body := e.confirm(t, task.ID); code != http.StatusConflict || body["error_code"] != services.ApprovalCodeRunRequired {
		t.Fatalf("no run: want 409 approval_run_required, got %d %v", code, body)
	}
}

// (queueManager is nil here: the dispatch goroutine's panic is recovered and logged.)
func TestConfirmApply_ApprovesApprovalRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := setupConfirmApplyEnv(t)
	task := e.plannedTask(t, "agent", "approval")
	code, body := e.confirm(t, task.ID)
	if code != http.StatusOK {
		t.Fatalf("want 200, got %d %v", code, body)
	}
	if !e.confirmed(t, task.ID) {
		t.Fatal("task not confirmed")
	}
	var run models.ManifestRun
	if err := e.db.Where("task_id = ?", task.ID).Take(&run).Error; err != nil {
		t.Fatal(err)
	}
	var planHash string
	e.db.Raw(`SELECT plan_hash FROM workspace_tasks WHERE id = ?`, task.ID).Scan(&planHash)
	if run.ApprovedBundleHash == nil || *run.ApprovedBundleHash != e.hash || run.ApprovedPlanHash == nil || *run.ApprovedPlanHash != planHash ||
		run.ApprovedBy == nil || *run.ApprovedBy != "u-approver" {
		t.Fatalf("approval not bound: %+v", run)
	}
	// second confirmation: already approved
	e.db.Exec(`UPDATE workspace_tasks SET status = 'apply_pending' WHERE id = ?`, task.ID)
	if code, body := e.confirm(t, task.ID); code != http.StatusConflict || body["error_code"] != services.ApprovalCodeAlreadyDone {
		t.Fatalf("re-approval: want 409 already_approved, got %d %v", code, body)
	}
}
