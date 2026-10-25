package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupShellPlanDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:shellplan_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE workspace_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			created_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			description TEXT,
			task_type TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			execution_mode TEXT NOT NULL DEFAULT 'local',
			stage TEXT DEFAULT 'pending',
			context TEXT,
			error_code TEXT,
			error_message TEXT,
			is_background INTEGER DEFAULT 0
		)`,
		`CREATE TABLE sandboxes (
			id TEXT PRIMARY KEY, org_id INTEGER, workspace_id TEXT, sandbox_ref TEXT,
			driver TEXT, status TEXT, created_by TEXT, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE shell_sessions (
			id TEXT PRIMARY KEY, sandbox_id TEXT, org_id INTEGER, status TEXT,
			expires_at DATETIME, closed_at DATETIME, created_by TEXT, created_at DATETIME, updated_at DATETIME
		)`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestTaskExcludedFromApproval_ShellPlan(t *testing.T) {
	if !TaskExcludedFromApproval(&models.WorkspaceTask{TaskType: models.TaskTypeShellPlan}) {
		t.Fatal("shell_plan must be excluded")
	}
	if !TaskExcludedFromApproval(&models.WorkspaceTask{TaskType: models.TaskTypePlan}) {
		t.Fatal("plan excluded")
	}
	if TaskExcludedFromApproval(&models.WorkspaceTask{TaskType: models.TaskTypePlanAndApply}) {
		t.Fatal("plan_and_apply must NOT be excluded by type alone")
	}
	if TaskRequiresManifestApproval(&models.WorkspaceTask{TaskType: models.TaskTypeShellPlan}, &models.Workspace{
		ManifestDeploymentID: shellPlanStrPtr("d1"), ManifestActiveTag: shellPlanStrPtr("v1"),
	}) {
		t.Fatal("shell_plan must never require manifest approval")
	}
}

func shellPlanStrPtr(s string) *string { return &s }

func TestCreateAndListShellPlanHistory(t *testing.T) {
	db := setupShellPlanDB(t)
	now := time.Now()
	sb := &models.Sandbox{
		ID: "sb-1", OrgID: 1, WorkspaceID: "ws-1", SandboxRef: "ref",
		Driver: models.SandboxDriverShellDebug, Status: models.SandboxStatusReady, CreatedBy: "u1",
	}
	sess := &models.ShellSession{
		ID: "sess-1", SandboxID: sb.ID, OrgID: 1,
		Status: models.ShellSessionStatusActive, ExpiresAt: now.Add(10 * time.Minute), CreatedBy: "u1",
	}
	res, err := CreateShellPlanHistory(context.Background(), db, sess, sb, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.TaskType != models.TaskTypeShellPlan || res.Task.Status != models.TaskStatusPending {
		t.Fatalf("task=%+v", res.Task)
	}
	if !TaskExcludedFromApproval(res.Task) {
		t.Fatal("created shell_plan not excluded")
	}
	ctx := res.Task.Context
	if ctx[ShellPlanContextSessionKey] != "sess-1" || ctx[ShellPlanContextApprovable] != false {
		t.Fatalf("context=%v", ctx)
	}

	// Second session should not see first plan
	other := &models.ShellSession{
		ID: "sess-2", SandboxID: sb.ID, OrgID: 1,
		Status: models.ShellSessionStatusActive, ExpiresAt: now.Add(10 * time.Minute), CreatedBy: "u1",
	}
	if _, err := CreateShellPlanHistory(context.Background(), db, other, sb, "u1"); err != nil {
		t.Fatal(err)
	}
	items, err := ListShellPlanHistory(context.Background(), db, "ws-1", "sess-1")
	if err != nil || len(items) != 1 {
		t.Fatalf("list sess-1: %v len=%d", err, len(items))
	}
	items2, err := ListShellPlanHistory(context.Background(), db, "ws-1", "sess-2")
	if err != nil || len(items2) != 1 {
		t.Fatalf("list sess-2: %v len=%d", err, len(items2))
	}
}

func TestCreateShellPlanHistory_Inactive(t *testing.T) {
	db := setupShellPlanDB(t)
	sb := &models.Sandbox{
		ID: "sb-1", WorkspaceID: "ws-1", Driver: models.SandboxDriverShellDebug, CreatedBy: "u1",
	}
	closed := time.Now()
	sess := &models.ShellSession{
		ID: "sess-1", SandboxID: sb.ID, Status: models.ShellSessionStatusClosed,
		ClosedAt: &closed, ExpiresAt: time.Now().Add(time.Hour), CreatedBy: "u1",
	}
	_, err := CreateShellPlanHistory(context.Background(), db, sess, sb, "u1")
	if !errors.Is(err, ErrShellPlanSessionInactive) {
		t.Fatalf("got %v", err)
	}
}

func TestApproveManifestRun_RefusesShellPlan(t *testing.T) {
	db := setupShellPlanDB(t)
	task := &models.WorkspaceTask{ID: 1, TaskType: models.TaskTypeShellPlan, WorkspaceID: "ws-1"}
	_, err := ApproveManifestRun(context.Background(), db, task, nil, "u1")
	var ae *ApprovalError
	if !errors.As(err, &ae) || ae.Code != ApprovalCodeNotApprovable || ae.Reason != "shell_plan" {
		t.Fatalf("got %v", err)
	}
}

func TestProviderMirrorConfigured_Env(t *testing.T) {
	restore := SetProviderMirrorConfiguredForTest(nil)
	defer restore()
	t.Setenv(EnvProviderNetworkMirror, "")
	if ProviderMirrorConfigured() {
		t.Fatal("empty env must be false")
	}
	t.Setenv(EnvProviderNetworkMirror, "https://mirror.example.internal/")
	if !ProviderMirrorConfigured() {
		t.Fatal("set env must be true")
	}
}
