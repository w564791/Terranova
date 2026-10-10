package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"
)

// approvalFixture: sqlite task DB + manifest_versions / manifest_deployments /
// manifest_runs / run_tokens; deployment mfd-1 runs version mfv-1 (tag v1)
// whose bundle is handoffFixture.
type approvalFixture struct {
	db *gorm.DB
	ws *models.Workspace
	h  *ManifestBundleHandoff
}

func setupApprovalFixture(t *testing.T) *approvalFixture {
	t.Helper()
	db := setupTestDB(t)
	for _, stmt := range []string{
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, bundle_hash TEXT)`,
		`CREATE TABLE manifest_deployments (id TEXT PRIMARY KEY, manifest_id TEXT, version_id TEXT, workspace_id TEXT, approved_bundle_hash TEXT, approved_plan_hash TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_runs (id TEXT PRIMARY KEY, manifest_id TEXT, version_id TEXT, bundle_hash TEXT, workspace_id TEXT, runner TEXT, purpose TEXT, status TEXT, plan_hash TEXT, plan_redacted TEXT, state_serial INTEGER, session_id TEXT, agent_id TEXT, task_id INTEGER UNIQUE, approved_bundle_hash TEXT, approved_plan_hash TEXT, approved_by TEXT, approved_at DATETIME, created_by TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE run_tokens (id INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT, session_id TEXT, workspace_id TEXT, purpose TEXT, token_hash TEXT, expires_at DATETIME, revoked_at DATETIME, agent_id TEXT, created_at DATETIME)`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	h := handoffFixture(t)
	require.NoError(t, db.Exec(`INSERT INTO manifest_versions (id, manifest_id, version, bundle_hash) VALUES ('mfv-1','mf-1','v1',?)`, h.BundleHash).Error)
	require.NoError(t, db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id) VALUES ('mfd-1','mf-1','mfv-1','ws-ap')`).Error)
	createTestWorkspace(t, db, "ws-ap")
	return &approvalFixture{db: db, ws: manifestWS("ws-ap", "pool-1"), h: h}
}

// plannedTask a manifest plan_and_apply task whose plan phase ran: sealed
// plan.out stored, plan_hash = its SHA-256, redacted plan JSON, apply_pending.
func (f *approvalFixture) plannedTask(t *testing.T, plan []byte) *models.WorkspaceTask {
	t.Helper()
	creator := "u-creator"
	task := createTestTask(t, f.db, "ws-ap", models.TaskTypePlanAndApply, models.TaskStatusPending)
	require.NoError(t, f.db.Model(&models.WorkspaceTask{}).Where("id = ?", task.ID).Update("created_by", creator).Error)
	task.CreatedBy = &creator
	sealed, err := SealTaskPlanData(task.ID, plan)
	require.NoError(t, err)
	sum := sha256.Sum256(plan)
	task.PlanData, task.PlanHash, task.Status = sealed, hex.EncodeToString(sum[:]), models.TaskStatusApplyPending
	task.PlanJSON = models.JSONB{"resource_changes": []interface{}{map[string]interface{}{"address": "null_resource.x"}}}
	require.NoError(t, f.db.Exec(`UPDATE workspace_tasks SET plan_data = ?, plan_hash = ?, status = ? WHERE id = ?`,
		sealed, task.PlanHash, task.Status, task.ID).Error)
	return task
}

func (f *approvalFixture) run(t *testing.T, taskID uint) *models.ManifestRun {
	t.Helper()
	run, err := ApprovalRunForTask(context.Background(), f.db, taskID)
	require.NoError(t, err)
	return run
}

func (f *approvalFixture) approve(task *models.WorkspaceTask, approver string) (*models.ManifestRun, error) {
	var run *models.ManifestRun
	err := f.db.Transaction(func(tx *gorm.DB) error {
		var err error
		run, err = ApproveManifestRun(context.Background(), tx, task, f.ws, approver)
		return err
	})
	return run, err
}

func approvalCode(t *testing.T, err error) (string, string) {
	t.Helper()
	var ae *ApprovalError
	require.True(t, errors.As(err, &ae), "want *ApprovalError, got %v", err)
	return ae.Code, ae.Reason
}

// Happy path: the plan dispatch creates the approval run (agent runner,
// purpose approval, deployed bundle), the push assigns it, approval binds
// both hashes, the run token outlives the plan by counting from approval,
// and the final task status ends the run and revokes its tokens.
func TestManifestApproval_RunLifecycle(t *testing.T) {
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	f := setupApprovalFixture(t)
	ctx := context.Background()
	task := f.plannedTask(t, []byte("PLAN-OUT-v1"))
	task.Status = models.TaskStatusPending

	run, err := EnsureApprovalRun(ctx, f.db, task, f.ws)
	require.NoError(t, err)
	require.NotNil(t, run)
	assert.Equal(t, models.ManifestRunPurposeApproval, run.Purpose)
	assert.Equal(t, models.ManifestRunRunnerAgent, run.Runner)
	assert.Equal(t, models.ManifestRunStatusRunning, run.Status)
	assert.Equal(t, f.h.BundleHash, run.BundleHash)
	assert.Equal(t, "u-creator", run.CreatedBy)
	assert.Len(t, run.ID, 36)
	again, err := EnsureApprovalRun(ctx, f.db, task, f.ws)
	require.NoError(t, err)
	assert.Equal(t, run.ID, again.ID, "one run per task")

	// non-manifest and plan-only tasks get none
	plain := &models.Workspace{WorkspaceID: "ws-ap"}
	none, err := EnsureApprovalRun(ctx, f.db, task, plain)
	assert.NoError(t, err)
	assert.Nil(t, none)
	planOnly := *task
	planOnly.TaskType = models.TaskTypePlan
	none, _ = EnsureApprovalRun(ctx, f.db, &planOnly, f.ws)
	assert.Nil(t, none)

	require.NoError(t, AssignApprovalRunAgent(ctx, f.db, task.ID, "agent-1"))
	assert.Equal(t, "agent-1", *f.run(t, task.ID).AgentID)

	// plan phase ends: its run tokens are revoked, the run continues
	f.db.Exec(`INSERT INTO run_tokens (run_id, workspace_id, purpose, token_hash, expires_at, agent_id) VALUES (?,?,?,?,?,?)`,
		run.ID, "ws-ap", "approval", "h1", time.Now().Add(time.Hour), "agent-1")
	require.NoError(t, EndApprovalRunPhase(ctx, f.db, task.ID, models.TaskStatusApplyPending))
	var active int64
	f.db.Table("run_tokens").Where("run_id = ? AND revoked_at IS NULL", run.ID).Count(&active)
	assert.Zero(t, active)
	assert.Equal(t, models.ManifestRunStatusRunning, f.run(t, task.ID).Status)

	// approval
	task.Status = models.TaskStatusApplyPending
	approved, err := f.approve(task, "u-approver")
	require.NoError(t, err)
	assert.Equal(t, f.h.BundleHash, *approved.ApprovedBundleHash)
	assert.Equal(t, task.PlanHash, *approved.ApprovedPlanHash, "approval binds the plan.out hash")
	assert.Equal(t, "u-approver", *approved.ApprovedBy)
	require.NotNil(t, approved.ApprovedAt)
	redacted, _ := RedactedPlanHash(task.PlanJSON)
	assert.Equal(t, redacted, *approved.PlanHash, "manifest_runs.plan_hash = redacted plan hash")
	var dep struct{ ApprovedBundleHash, ApprovedPlanHash string }
	f.db.Raw(`SELECT approved_bundle_hash, approved_plan_hash FROM manifest_deployments WHERE id = 'mfd-1'`).Scan(&dep)
	assert.Equal(t, f.h.BundleHash, dep.ApprovedBundleHash)
	assert.Equal(t, task.PlanHash, dep.ApprovedPlanHash)
	a, err := ManifestApprovalForTask(ctx, f.db, task.ID)
	require.NoError(t, err)
	assert.True(t, a.Approved())

	_, err = f.approve(task, "u-approver")
	code, _ := approvalCode(t, err)
	assert.Equal(t, ApprovalCodeAlreadyDone, code)

	// local run token: issued in process (no agent on the run), expiry
	// counted from the approval
	f.db.Exec(`UPDATE manifest_runs SET agent_id = NULL, created_at = ?, approved_at = ? WHERE id = ?`,
		time.Now().Add(-3*time.Hour), time.Now(), run.ID)
	tok, exp, err := NewStateTokenService(f.db).IssueRunToken(ctx, run.ID, "")
	require.NoError(t, err, "late approval must still get a token")
	assert.NotEmpty(t, tok)
	assert.WithinDuration(t, time.Now().Add(ManifestRunTimeout()), exp, time.Minute)

	// the task's final status ends the run and revokes its tokens
	require.NoError(t, EndApprovalRunPhase(ctx, f.db, task.ID, models.TaskStatusApplied))
	assert.Equal(t, models.ManifestRunStatusSucceeded, f.run(t, task.ID).Status)
	f.db.Table("run_tokens").Where("run_id = ? AND revoked_at IS NULL", run.ID).Count(&active)
	assert.Zero(t, active)
}

// The approval API only accepts purpose=approval, runner=agent runs.
func TestManifestApproval_RejectsPreviewAndSandboxRuns(t *testing.T) {
	f := setupApprovalFixture(t)
	for _, rp := range [][2]string{{"agent", "preview"}, {"sandbox", "preview"}} {
		task := f.plannedTask(t, []byte("PLAN"))
		require.NoError(t, f.db.Exec(`INSERT INTO manifest_runs (id, manifest_id, version_id, bundle_hash, workspace_id, runner, purpose, status, session_id, task_id, created_by, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			"mfr-"+rp[0]+"-"+rp[1], "mf-1", "mfv-1", f.h.BundleHash, "ws-ap", rp[0], rp[1], "running", "sess-1", task.ID, "u1", time.Now()).Error)
		_, err := f.approve(task, "u-approver")
		code, reason := approvalCode(t, err)
		assert.Equal(t, ApprovalCodeNotApprovable, code, rp)
		assert.Equal(t, rp[0]+"_"+rp[1], reason)
		run := f.run(t, task.ID)
		assert.Nil(t, run.ApprovedAt, "nothing recorded")
		assert.Nil(t, run.ApprovedPlanHash)
	}

	// a manifest task without a run (planned before approval runs existed)
	task := f.plannedTask(t, []byte("PLAN"))
	_, err := f.approve(task, "u-approver")
	code, _ := approvalCode(t, err)
	assert.Equal(t, ApprovalCodeRunRequired, code)

	// a non-manifest task keeps the ordinary confirm flow
	run, err := ApproveManifestRun(context.Background(), f.db, task, &models.Workspace{WorkspaceID: "ws-ap"}, "u")
	assert.NoError(t, err)
	assert.Nil(t, run)
}

// Approval refuses when the stored plan or the bundle no longer match what
// the plan run produced, and on a NULL-hash version.
func TestManifestApproval_RefusesChangedPlanOrBundle(t *testing.T) {
	f := setupApprovalFixture(t)
	ctx := context.Background()
	newRun := func(t *testing.T) *models.WorkspaceTask {
		task := f.plannedTask(t, []byte("PLAN-"+t.Name()))
		pending := *task
		pending.Status = models.TaskStatusPending
		_, err := EnsureApprovalRun(ctx, f.db, &pending, f.ws)
		require.NoError(t, err)
		return task
	}

	t.Run("plan_changed", func(t *testing.T) {
		task := newRun(t)
		other, _ := SealTaskPlanData(task.ID, []byte("ANOTHER PLAN"))
		task.PlanData = other
		_, err := f.approve(task, "u")
		code, reason := approvalCode(t, err)
		assert.Equal(t, models.TaskErrorCodeApprovalHashMismatch, code)
		assert.Equal(t, ApprovalReasonPlanChanged, reason)
	})
	t.Run("plan_expired", func(t *testing.T) {
		task := newRun(t)
		task.PlanData = nil
		f.db.Exec(`UPDATE workspace_tasks SET plan_data = NULL WHERE id = ?`, task.ID)
		_, err := f.approve(task, "u")
		code, _ := approvalCode(t, err)
		assert.Equal(t, models.TaskErrorCodePlanExpired, code)
	})
	t.Run("bundle_changed", func(t *testing.T) {
		task := newRun(t)
		f.db.Exec(`INSERT INTO manifest_versions (id, manifest_id, version, bundle_hash) VALUES ('mfv-2','mf-1','v1',?)`, strings.Repeat("b", 64))
		f.db.Exec(`UPDATE manifest_deployments SET version_id = 'mfv-2' WHERE id = 'mfd-1'`)
		defer f.db.Exec(`UPDATE manifest_deployments SET version_id = 'mfv-1' WHERE id = 'mfd-1'`)
		_, err := f.approve(task, "u")
		code, reason := approvalCode(t, err)
		assert.Equal(t, models.TaskErrorCodeApprovalHashMismatch, code)
		assert.Equal(t, ApprovalReasonBundleChanged, reason)
	})
	t.Run("null_bundle_hash", func(t *testing.T) {
		task := newRun(t)
		f.db.Exec(`UPDATE manifest_versions SET bundle_hash = NULL WHERE id = 'mfv-1'`)
		defer f.db.Exec(`UPDATE manifest_versions SET bundle_hash = ? WHERE id = 'mfv-1'`, f.h.BundleHash)
		_, err := f.approve(task, "u")
		code, _ := approvalCode(t, err)
		assert.Equal(t, models.TaskErrorCodeBundleRepublishRequired, code)
	})
	t.Run("null_hash_version_gets_no_run", func(t *testing.T) {
		f.db.Exec(`UPDATE manifest_versions SET bundle_hash = NULL WHERE id = 'mfv-1'`)
		defer f.db.Exec(`UPDATE manifest_versions SET bundle_hash = ? WHERE id = 'mfv-1'`, f.h.BundleHash)
		task := createTestTask(t, f.db, "ws-ap", models.TaskTypePlanAndApply, models.TaskStatusPending)
		run, err := EnsureApprovalRun(ctx, f.db, task, f.ws)
		assert.NoError(t, err)
		assert.Nil(t, run)
	})
}

// A run whose task ended by another path (cancel, cleanup) is ended by the
// sweep, and its tokens stop validating at once.
func TestManifestApproval_FinishedTaskSweep(t *testing.T) {
	f := setupApprovalFixture(t)
	ctx := context.Background()
	task := createTestTask(t, f.db, "ws-ap", models.TaskTypePlanAndApply, models.TaskStatusPending)
	run, err := EnsureApprovalRun(ctx, f.db, task, f.ws)
	require.NoError(t, err)
	f.db.Exec(`UPDATE workspace_tasks SET status = 'cancelled' WHERE id = ?`, task.ID)
	n, err := EndFinishedApprovalRuns(ctx, f.db)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, models.ManifestRunStatusCancelled, f.run(t, *run.TaskID).Status)
}

// ---- executor side ----

func approvedWorkDir(t *testing.T, h *ManifestBundleHandoff, plan []byte) (string, string, *ManifestApproval) {
	t.Helper()
	work := t.TempDir()
	_, err := unpackManifestHandoff(h, work)
	require.NoError(t, err)
	planFile := filepath.Join(work, "plan.out")
	require.NoError(t, os.WriteFile(planFile, plan, 0o600))
	sum := sha256.Sum256(plan)
	return work, planFile, &ManifestApproval{RunID: "mfr-1", BundleHash: h.BundleHash,
		ApprovedBundleHash: h.BundleHash, ApprovedPlanHash: hex.EncodeToString(sum[:])}
}

func remoteExecutor(t *testing.T, h *ManifestBundleHandoff, approval *ManifestApproval) (*TerraformExecutor, *models.Workspace) {
	t.Helper()
	data := map[string]interface{}{
		TaskDataManifestBundle: h.TaskDataPayload(),
		"workspace": map[string]interface{}{"workspace_id": "ws-1", "manifest_deployment_id": "mfd-1",
			"manifest_active_tag": "v1", "manifest_subpath": "envs/prod"},
	}
	if approval != nil {
		data[TaskDataManifestApproval] = approval.TaskDataPayload()
	}
	remote := NewRemoteDataAccessorFromTaskData(jsonRoundTrip(t, data))
	ws, err := remote.GetWorkspace("ws-1")
	require.NoError(t, err)
	return &TerraformExecutor{dataAccessor: remote}, ws
}

func requireApprovalMismatch(t *testing.T, err error, reason string) {
	t.Helper()
	require.Error(t, err)
	code, gotReason, msg := classifyTaskFailure(err, "x")
	assert.Equal(t, models.TaskErrorCodeApprovalHashMismatch, code)
	assert.Equal(t, reason, gotReason)
	assert.True(t, strings.HasPrefix(msg, "approval_hash_mismatch: "+reason), msg)
}

func TestManifestApply_HappyPath(t *testing.T) {
	h := handoffFixture(t)
	work, planFile, approval := approvedWorkDir(t, h, []byte("PLAN-OUT"))
	exec, ws := remoteExecutor(t, h, approval)
	got, err := exec.dataAccessor.GetManifestApproval(42)
	require.NoError(t, err)
	assert.Equal(t, *approval, *got, "task data round trip")
	assert.NoError(t, exec.verifyManifestApply(ws, work, planFile, got))

	none, _ := NewRemoteDataAccessorFromTaskData(map[string]interface{}{}).GetManifestApproval(42)
	assert.Nil(t, none)
}

// The plan.out was replaced between approval and apply.
func TestManifestApply_RefusesChangedPlan(t *testing.T) {
	h := handoffFixture(t)
	work, planFile, approval := approvedWorkDir(t, h, []byte("PLAN-OUT"))
	exec, ws := remoteExecutor(t, h, approval)
	require.NoError(t, os.WriteFile(planFile, []byte("PLAN-OUT-OTHER"), 0o600))
	requireApprovalMismatch(t, exec.verifyManifestApply(ws, work, planFile, approval), ApprovalReasonPlanChanged)
	os.Remove(planFile)
	requireApprovalMismatch(t, exec.verifyManifestApply(ws, work, planFile, approval), ApprovalReasonPlanChanged)
}

// The deployment moved to another bundle after approval, or the bundle in
// the work directory was modified.
func TestManifestApply_RefusesChangedBundle(t *testing.T) {
	approvedBundle := handoffFixture(t)
	work, planFile, approval := approvedWorkDir(t, approvedBundle, []byte("PLAN-OUT"))

	files := []manifestbundle.File{{Path: "envs/prod/main.tf", Content: []byte("resource \"null_resource\" \"evil\" {}\n"), Mode: 0o644}}
	nh, _ := manifestbundle.Hash(files)
	na, _ := manifestbundle.ArchiveBytes(files)
	upgraded := &ManifestBundleHandoff{DeploymentID: "mfd-1", Tag: "v1", VersionID: "mfv-2", BundleHash: nh, Archive: na}
	exec, ws := remoteExecutor(t, upgraded, approval)
	requireApprovalMismatch(t, exec.verifyManifestApply(ws, work, planFile, approval), ApprovalReasonBundleChanged)

	exec, ws = remoteExecutor(t, approvedBundle, approval)
	p := filepath.Join(work, "modules/m/main.tf")
	require.NoError(t, os.WriteFile(p, []byte("output \"x\" { value = 2 }\n"), 0o644))
	requireApprovalMismatch(t, exec.verifyManifestApply(ws, work, planFile, approval), ApprovalReasonBundleChanged)
	require.NoError(t, os.Remove(p))
	require.NoError(t, os.Symlink("/etc/hostname", p))
	requireApprovalMismatch(t, exec.verifyManifestApply(ws, work, planFile, approval), ApprovalReasonBundleChanged)
}

func TestManifestApply_RefusesUnapproved(t *testing.T) {
	h := handoffFixture(t)
	work, planFile, approval := approvedWorkDir(t, h, []byte("PLAN-OUT"))
	approval.ApprovedBundleHash, approval.ApprovedPlanHash = "", ""
	exec, ws := remoteExecutor(t, h, approval)
	requireApprovalMismatch(t, exec.verifyManifestApply(ws, work, planFile, approval), ApprovalReasonNotApproved)
	requireApprovalMismatch(t, exec.verifyManifestApply(ws, work, planFile, nil), ApprovalReasonNotApproved)
	assert.True(t, TaskRequiresManifestApproval(&models.WorkspaceTask{TaskType: models.TaskTypeApply}, ws),
		"standalone apply tasks on a manifest workspace need an approval too")
	assert.False(t, TaskRequiresManifestApproval(&models.WorkspaceTask{TaskType: models.TaskTypePlan}, ws))
}

// PostgreSQL: chk_manifest_runs_approval (migration
// 20261010_13_manifest_approval) refuses an approval on any run that is not
// purpose=approval, runner=agent, and an approved bundle other than the
// run's own. Skipped when the test database lacks the constraint.
func TestManifestApproval_DBCheck_PG(t *testing.T) {
	db := testDBOrSkip(t)
	var n int64
	db.Raw(`SELECT count(*) FROM pg_constraint WHERE conname = 'chk_manifest_runs_approval'`).Scan(&n)
	if n == 0 {
		t.Skip("test database has no chk_manifest_runs_approval (apply migration 20261010_13_manifest_approval)")
	}
	hash := strings.Repeat("a", 64)
	insert := func(id, runner, purpose, approvedBundle string) error {
		return db.Transaction(func(tx *gorm.DB) error {
			// FK triggers off (no manifests / sessions fixtures); CHECKs stay on
			if err := tx.Exec(`SET LOCAL session_replication_role = replica`).Error; err != nil {
				return err
			}
			var session interface{}
			if runner == "sandbox" {
				session = "sess-x"
			}
			if err := tx.Exec(`INSERT INTO manifest_runs (id, manifest_id, bundle_hash, workspace_id, runner, purpose, status, session_id, created_by,
				approved_bundle_hash, approved_plan_hash, approved_by, approved_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,now())`,
				id, "mf-x", hash, "ws-x", runner, purpose, "running", session, "u", approvedBundle, strings.Repeat("c", 64), "u").Error; err != nil {
				return err
			}
			return errors.New("rollback")
		})
	}
	for _, rp := range [][2]string{{"sandbox", "preview"}, {"agent", "preview"}} {
		err := insert("mfr-chk-"+rp[0], rp[0], rp[1], hash)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chk_manifest_runs_approval", rp)
	}
	err := insert("mfr-chk-other", "agent", "approval", strings.Repeat("b", 64))
	assert.Contains(t, err.Error(), "chk_manifest_runs_approval", "approved bundle must be the run's own")
	assert.Equal(t, "rollback", insert("mfr-chk-ok", "agent", "approval", hash).Error())
}
