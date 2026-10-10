package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Approval and apply double hash check (docs/manifest/manifest-sandbox-spec.md
// §1, §9 step 7).
//
// Every plan_and_apply task of a manifest-managed workspace (deployment + tag,
// not a Manifest [Run] draft) runs as one purpose=approval, runner=agent
// manifest run (manifest_runs.task_id). The run is created when the plan phase
// is dispatched, with the bundle_hash of the deployed version, and assigned to
// the agent the phase is pushed to (Local mode leaves agent_id NULL: the
// platform is the runner). Its run token replaces the task's state token.
//
// Approval (ConfirmApply) only accepts such a run and records:
//   - approved_bundle_hash = the run's bundle_hash: the bundle the plan was
//     computed from (still the deployment's current, non-NULL bundle);
//   - approved_plan_hash = SHA-256 of the binary plan.out (workspace_tasks.
//     plan_hash, re-computed by the platform from the stored plan data).
//     plan.out is what `terraform apply plan.out` executes - it embeds the
//     configuration snapshot and the exact changes - so this is what the
//     approval binds to. The redacted plan JSON the approver looked at is
//     lossy (sensitive values removed); its hash is kept in
//     manifest_runs.plan_hash as the record of what was shown.
//
// Before apply the executor (agent or Local) refuses with error_code
// approval_hash_mismatch unless the hand-off bundle and the files in the work
// directory hash to approved_bundle_hash and the plan.out it is about to apply
// hashes to approved_plan_hash.

// Approval mismatch reasons (workspace_tasks.error_reason).
const (
	ApprovalReasonNotApproved   = "not_approved"
	ApprovalReasonBundleChanged = "bundle_changed"
	ApprovalReasonPlanChanged   = "plan_changed"
)

// ApprovalMismatchError an apply refused because it is not what was approved
// (TaskErrorCode approval_hash_mismatch, reason Reason).
type ApprovalMismatchError struct {
	Reason string
	Detail string
}

func (e *ApprovalMismatchError) Error() string {
	msg := models.TaskErrorCodeApprovalHashMismatch + ": " + e.Reason
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

// ManifestApproval what the executor needs to check a manifest task against
// its approval run. Agent mode: GetTaskData "manifest_approval".
type ManifestApproval struct {
	RunID              string
	BundleHash         string // bundle the run (plan) was created for
	ApprovedBundleHash string // "" until approved
	ApprovedPlanHash   string // "" until approved
}

// Approved whether both approved hashes are recorded.
func (a *ManifestApproval) Approved() bool {
	return a != nil && a.ApprovedBundleHash != "" && a.ApprovedPlanHash != ""
}

// TaskDataManifestApproval the agent task-data key of the approval binding.
const TaskDataManifestApproval = "manifest_approval"

// TaskDataPayload the "manifest_approval" object of the agent task data.
func (a *ManifestApproval) TaskDataPayload() map[string]interface{} {
	return map[string]interface{}{
		"run_id":               a.RunID,
		"bundle_hash":          a.BundleHash,
		"approved_bundle_hash": a.ApprovedBundleHash,
		"approved_plan_hash":   a.ApprovedPlanHash,
	}
}

// manifestApprovalFromTaskData decodes the agent task-data form; nil when the
// task has no approval run.
func manifestApprovalFromTaskData(data map[string]interface{}) *ManifestApproval {
	p, ok := data[TaskDataManifestApproval].(map[string]interface{})
	if !ok {
		return nil
	}
	return &ManifestApproval{
		RunID:              getString(p, "run_id"),
		BundleHash:         getString(p, "bundle_hash"),
		ApprovedBundleHash: getString(p, "approved_bundle_hash"),
		ApprovedPlanHash:   getString(p, "approved_plan_hash"),
	}
}

// ManifestApprovalForRun the executor's view of run.
func ManifestApprovalForRun(run *models.ManifestRun) *ManifestApproval { return approvalFromRun(run) }

func approvalFromRun(run *models.ManifestRun) *ManifestApproval {
	if run == nil {
		return nil
	}
	a := &ManifestApproval{RunID: run.ID, BundleHash: run.BundleHash}
	if run.ApprovedBundleHash != nil {
		a.ApprovedBundleHash = *run.ApprovedBundleHash
	}
	if run.ApprovedPlanHash != nil {
		a.ApprovedPlanHash = *run.ApprovedPlanHash
	}
	return a
}

// ApprovalRunTaskData the approval part of an agent's task data: when task
// has an approval run, its binding (task data "manifest_approval") and a run
// token issued to agentID (the run's assigned agent; ErrRunNotAssigned
// otherwise), which replaces the task state token. (nil, "", nil) when the
// task has no approval run.
func (s *StateTokenService) ApprovalRunTaskData(ctx context.Context, taskID uint, agentID string) (*ManifestApproval, string, error) {
	run, err := ApprovalRunForTask(ctx, s.db, taskID)
	if err != nil || run == nil {
		return nil, "", err
	}
	tok, _, err := s.IssueRunToken(ctx, run.ID, agentID)
	if err != nil {
		return nil, "", err
	}
	return approvalFromRun(run), tok, nil
}

// workspaceUsesManifestDeployment the workspace runs a deployed manifest
// version (deployment + tag).
func workspaceUsesManifestDeployment(ws *models.Workspace) bool {
	return ws != nil && ws.ManifestDeploymentID != nil && *ws.ManifestDeploymentID != "" &&
		ws.ManifestActiveTag != nil && *ws.ManifestActiveTag != ""
}

// TaskRequiresManifestApproval a task that applies a deployed manifest
// version: its apply needs a recorded approval (plan_and_apply through an
// approval run; standalone apply tasks are refused).
func TaskRequiresManifestApproval(task *models.WorkspaceTask, ws *models.Workspace) bool {
	return task != nil && workspaceUsesManifestDeployment(ws) && !taskUsesExternalFiles(task) &&
		(task.TaskType == models.TaskTypePlanAndApply || task.TaskType == models.TaskTypeApply)
}

// ApprovalRunForTask the manifest run of task (nil when it has none).
func ApprovalRunForTask(ctx context.Context, db *gorm.DB, taskID uint) (*models.ManifestRun, error) {
	var run models.ManifestRun
	err := db.WithContext(ctx).Where("task_id = ?", taskID).Take(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ManifestApprovalForTask the approval binding of task for the executor (nil
// when the task has no approval run).
func ManifestApprovalForTask(ctx context.Context, db *gorm.DB, taskID uint) (*ManifestApproval, error) {
	run, err := ApprovalRunForTask(ctx, db, taskID)
	if err != nil || run == nil {
		return nil, err
	}
	return approvalFromRun(run), nil
}

type deployedVersion struct {
	VersionID  string
	ManifestID string
	BundleHash *string
}

func lookupDeployedVersion(ctx context.Context, db *gorm.DB, deploymentID, tag string) (*deployedVersion, error) {
	var v deployedVersion
	err := db.WithContext(ctx).Raw(`
		SELECT mv.id AS version_id, mv.manifest_id, mv.bundle_hash
		  FROM manifest_versions mv
		  JOIN manifest_deployments md ON md.version_id = mv.id
		 WHERE md.id = ? AND mv.version = ?
	`, deploymentID, tag).Scan(&v).Error
	if err != nil {
		return nil, err
	}
	if v.VersionID == "" {
		return nil, nil
	}
	return &v, nil
}

func newManifestRunID() string {
	return "mfr-" + strings.ReplaceAll(uuid.New().String(), "-", "") // 36 chars
}

// EnsureApprovalRun creates (or returns) the approval run of a manifest
// plan_and_apply task before its plan phase is dispatched. Returns nil, nil
// for tasks that need none, and when the deployment does not resolve to a
// version with a bundle_hash (NULL: the executor fails the task with
// bundle_republish_required at hand-off; there is nothing to bind to).
func EnsureApprovalRun(ctx context.Context, db *gorm.DB, task *models.WorkspaceTask, ws *models.Workspace) (*models.ManifestRun, error) {
	if task == nil || task.TaskType != models.TaskTypePlanAndApply || !TaskRequiresManifestApproval(task, ws) {
		return nil, nil
	}
	if run, err := ApprovalRunForTask(ctx, db, task.ID); err != nil || run != nil {
		return run, err
	}
	v, err := lookupDeployedVersion(ctx, db, *ws.ManifestDeploymentID, *ws.ManifestActiveTag)
	if err != nil {
		return nil, fmt.Errorf("resolve deployed manifest version: %w", err)
	}
	if v == nil || v.BundleHash == nil || *v.BundleHash == "" {
		return nil, nil
	}
	createdBy := "system"
	if task.CreatedBy != nil && *task.CreatedBy != "" {
		createdBy = *task.CreatedBy
	}
	taskID := task.ID
	versionID := v.VersionID
	run := &models.ManifestRun{
		ID: newManifestRunID(), ManifestID: v.ManifestID, VersionID: &versionID, BundleHash: *v.BundleHash,
		WorkspaceID: task.WorkspaceID, Runner: models.ManifestRunRunnerAgent, Purpose: models.ManifestRunPurposeApproval,
		Status: models.ManifestRunStatusRunning, TaskID: &taskID, CreatedBy: createdBy,
	}
	res := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Omit("PlanRedacted").Create(run)
	if res.Error != nil {
		return nil, fmt.Errorf("create approval run: %w", res.Error)
	}
	if res.RowsAffected == 0 { // concurrent dispatcher created it
		return ApprovalRunForTask(ctx, db, task.ID)
	}
	log.Printf("[ManifestRun] task %d: approval run %s created (version=%s, bundle_hash=%s)", task.ID, run.ID, versionID, run.BundleHash)
	return run, nil
}

// AssignApprovalRunAgent records the agent a phase of task was pushed to on
// its approval run (only that agent obtains the run's tokens).
func AssignApprovalRunAgent(ctx context.Context, db *gorm.DB, taskID uint, agentID string) error {
	return db.WithContext(ctx).Model(&models.ManifestRun{}).
		Where("task_id = ? AND status IN ?", taskID, []string{models.ManifestRunStatusPending, models.ManifestRunStatusRunning}).
		Updates(map[string]interface{}{"agent_id": agentID, "updated_at": time.Now()}).Error
}

// ApprovalRunStatusForTask the run status a task status ends its approval run
// with ("" when the run continues: running phases, and apply_pending /
// decision_required, where only the phase ends).
func ApprovalRunStatusForTask(status models.TaskStatus) string {
	switch status {
	case models.TaskStatusApplied, models.TaskStatusSuccess, models.TaskStatusPlannedAndFinished:
		return models.ManifestRunStatusSucceeded
	case models.TaskStatusFailed:
		return models.ManifestRunStatusFailed
	case models.TaskStatusCancelled:
		return models.ManifestRunStatusCancelled
	}
	return ""
}

// EndApprovalRunPhase is called whenever a phase of task stops: a final
// task status ends the approval run (EndManifestRun: final status, its
// tokens revoked); the end of the plan phase (apply_pending /
// decision_required) revokes the plan phase's run tokens, the apply phase
// obtains new ones. No-op for tasks without an approval run.
func EndApprovalRunPhase(ctx context.Context, db *gorm.DB, taskID uint, status models.TaskStatus) error {
	run, err := ApprovalRunForTask(ctx, db, taskID)
	if err != nil || run == nil {
		return err
	}
	if final := ApprovalRunStatusForTask(status); final != "" {
		if !runActive(run.Status) {
			return RevokeRunTokens(ctx, db, run.ID)
		}
		return EndManifestRun(ctx, db, run.ID, final)
	}
	if status == models.TaskStatusApplyPending || status == models.TaskStatusDecisionRequired {
		return RevokeRunTokens(ctx, db, run.ID)
	}
	return nil
}

// ApprovalError a refused approval: HTTP status, error_code, reason.
type ApprovalError struct {
	Status  int
	Code    string
	Reason  string
	Message string
}

func (e *ApprovalError) Error() string {
	if e.Reason != "" {
		return e.Code + ": " + e.Reason + ": " + e.Message
	}
	return e.Code + ": " + e.Message
}

// Approval refusal codes besides approval_hash_mismatch / plan_expired /
// bundle_republish_required.
const (
	ApprovalCodeRunRequired   = "approval_run_required" // manifest task without an approval run (planned before step 7: re-run the plan)
	ApprovalCodeNotApprovable = "run_not_approvable"    // linked run is not purpose=approval, runner=agent (preview / sandbox)
	ApprovalCodeRunNotActive  = "run_not_active"        // run already ended
	ApprovalCodeAlreadyDone   = "already_approved"
)

func approvalRefused(status int, code, reason, msg string) *ApprovalError {
	return &ApprovalError{Status: status, Code: code, Reason: reason, Message: msg}
}

// ApproveManifestRun records the approval of task's manifest run (run in
// ConfirmApply's transaction, before the task is confirmed). Returns nil, nil
// for tasks that are not manifest deployments and have no run (the ordinary
// confirm-apply flow). Refusals are *ApprovalError.
func ApproveManifestRun(ctx context.Context, tx *gorm.DB, task *models.WorkspaceTask, ws *models.Workspace, approver string) (*models.ManifestRun, error) {
	run, err := ApprovalRunForTask(ctx, tx, task.ID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		if TaskRequiresManifestApproval(task, ws) {
			return nil, approvalRefused(http.StatusConflict, ApprovalCodeRunRequired, "",
				"this manifest task has no approval run; run the plan again")
		}
		return nil, nil
	}
	// Only approval runs on the agent runner can be approved: sandbox /
	// preview runs never (chk_manifest_runs_approval enforces it in the DB).
	if run.Purpose != models.ManifestRunPurposeApproval || run.Runner != models.ManifestRunRunnerAgent {
		return nil, approvalRefused(http.StatusConflict, ApprovalCodeNotApprovable, run.Runner+"_"+run.Purpose,
			"only purpose=approval runs on the agent runner can be approved")
	}
	if run.WorkspaceID != task.WorkspaceID {
		return nil, approvalRefused(http.StatusConflict, ApprovalCodeNotApprovable, "workspace", "run belongs to another workspace")
	}
	if approver == "" {
		return nil, approvalRefused(http.StatusForbidden, ApprovalCodeNotApprovable, "approver_unknown", "the approver is unknown")
	}
	if run.ApprovedAt != nil {
		return nil, approvalRefused(http.StatusConflict, ApprovalCodeAlreadyDone, "", "this run is already approved")
	}
	if !runActive(run.Status) {
		return nil, approvalRefused(http.StatusConflict, ApprovalCodeRunNotActive, run.Status, "the run has ended")
	}

	// Bundle: the version the plan ran on still has a bundle_hash equal to
	// the run's, and the deployment still points to that bundle.
	var versionHash *string
	if run.VersionID != nil {
		if err := tx.WithContext(ctx).Raw(`SELECT bundle_hash FROM manifest_versions WHERE id = ?`, *run.VersionID).
			Scan(&versionHash).Error; err != nil {
			return nil, err
		}
	}
	if versionHash == nil || *versionHash == "" {
		return nil, approvalRefused(http.StatusConflict, models.TaskErrorCodeBundleRepublishRequired, "bundle_hash_null",
			"the manifest version has no bundle hash; republish it and run the plan again")
	}
	if *versionHash != run.BundleHash {
		return nil, approvalRefused(http.StatusConflict, models.TaskErrorCodeApprovalHashMismatch, ApprovalReasonBundleChanged,
			"the manifest version's bundle changed after the plan")
	}
	if workspaceUsesManifestDeployment(ws) {
		cur, err := lookupDeployedVersion(ctx, tx, *ws.ManifestDeploymentID, *ws.ManifestActiveTag)
		if err != nil {
			return nil, err
		}
		if cur == nil || cur.BundleHash == nil || *cur.BundleHash != run.BundleHash {
			return nil, approvalRefused(http.StatusConflict, models.TaskErrorCodeApprovalHashMismatch, ApprovalReasonBundleChanged,
				"the deployment no longer runs the bundle that was planned; run the plan again")
		}
	}

	// Plan: the platform re-hashes the stored plan.out and requires the hash
	// the executor reported.
	if !isSHA256Hex(task.PlanHash) {
		return nil, approvalRefused(http.StatusConflict, models.TaskErrorCodeApprovalHashMismatch, ApprovalReasonPlanChanged,
			"the plan has no recorded hash; run the plan again")
	}
	planTask := task
	if len(planTask.PlanData) == 0 {
		var t models.WorkspaceTask
		if err := tx.WithContext(ctx).Select("id", "plan_data").Take(&t, task.ID).Error; err != nil {
			return nil, err
		}
		planTask = &t
	}
	plan, err := openPlanDataForApproval(planTask)
	if err != nil {
		if errors.Is(err, crypto.ErrPlanDataExpired) || errors.Is(err, ErrPlanDataMissing) {
			return nil, approvalRefused(http.StatusConflict, models.TaskErrorCodePlanExpired, TaskErrorReason(err),
				"the plan has expired; run the plan again")
		}
		return nil, fmt.Errorf("open plan data: %w", err)
	}
	sum := sha256.Sum256(plan)
	if hex.EncodeToString(sum[:]) != task.PlanHash {
		log.Printf("[SECURITY] task %d: stored plan does not hash to the reported plan_hash, approval refused", task.ID)
		return nil, approvalRefused(http.StatusConflict, models.TaskErrorCodeApprovalHashMismatch, ApprovalReasonPlanChanged,
			"the stored plan does not match the plan that was run")
	}

	now := time.Now()
	updates := map[string]interface{}{
		"approved_bundle_hash": run.BundleHash,
		"approved_plan_hash":   task.PlanHash,
		"approved_by":          approver,
		"approved_at":          now,
		"updated_at":           now,
	}
	if len(task.PlanJSON) > 0 {
		if h, err := RedactedPlanHash(task.PlanJSON); err == nil {
			updates["plan_hash"] = h
		}
	}
	res := tx.WithContext(ctx).Model(&models.ManifestRun{}).
		Where("id = ? AND approved_at IS NULL AND purpose = ? AND runner = ? AND status IN ?", run.ID,
			models.ManifestRunPurposeApproval, models.ManifestRunRunnerAgent,
			[]string{models.ManifestRunStatusPending, models.ManifestRunStatusRunning}).
		Updates(updates)
	if res.Error != nil {
		return nil, fmt.Errorf("record approval: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return nil, approvalRefused(http.StatusConflict, ApprovalCodeAlreadyDone, "", "the run changed concurrently")
	}
	if workspaceUsesManifestDeployment(ws) {
		if err := tx.WithContext(ctx).Model(&models.ManifestDeployment{}).Where("id = ?", *ws.ManifestDeploymentID).
			Updates(map[string]interface{}{"approved_bundle_hash": run.BundleHash, "approved_plan_hash": task.PlanHash}).Error; err != nil {
			return nil, fmt.Errorf("record deployment approval: %w", err)
		}
	}
	return ApprovalRunForTask(ctx, tx, task.ID)
}

// openPlanDataForApproval the stored binary plan (sealed plan data only).
func openPlanDataForApproval(task *models.WorkspaceTask) ([]byte, error) {
	if len(task.PlanData) == 0 {
		return nil, ErrPlanDataMissing
	}
	if sealed, _ := crypto.IsSealedPlanData(task.PlanData); !sealed {
		return nil, crypto.ErrPlanDataNotSealed
	}
	return OpenTaskPlanData(task)
}

func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

// EndFinishedApprovalRuns ends the approval runs still pending / running
// whose task already reached a final status (task ended by a path that did
// not call EndApprovalRunPhase, e.g. a cancel or a stale-task cleanup).
// Returns how many were ended.
func EndFinishedApprovalRuns(ctx context.Context, db *gorm.DB) (int, error) {
	var rows []struct {
		ID     string
		Status models.TaskStatus
	}
	if err := db.WithContext(ctx).Table("manifest_runs AS r").
		Select("r.id, wt.status").
		Joins("JOIN workspace_tasks wt ON wt.id = r.task_id").
		Where("r.status IN ?", []string{models.ManifestRunStatusPending, models.ManifestRunStatusRunning}).
		Where("wt.status IN ?", []models.TaskStatus{models.TaskStatusFailed, models.TaskStatusCancelled,
			models.TaskStatusApplied, models.TaskStatusSuccess, models.TaskStatusPlannedAndFinished}).
		Limit(500).Scan(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		if err := EndManifestRun(ctx, db, r.ID, ApprovalRunStatusForTask(r.Status)); err != nil {
			log.Printf("[ManifestRun] end %s: %v", r.ID, err)
			continue
		}
		n++
	}
	return n, nil
}
