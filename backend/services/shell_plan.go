package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// Shell plan history (§10.2): record-only preview tasks from a Shell session.
// Hard rules: never enter approval hashes, never apply, -lock=false (preview
// purpose). Cloud identity is user-prepared externally — platform does not
// inject STS. Missing provider mirror → caller returns 503 provider_unreachable.

const (
	// ShellPlanContextSessionKey links a workspace_tasks.context entry to its shell session.
	ShellPlanContextSessionKey = "shell_session_id"
	ShellPlanContextSandboxKey = "sandbox_id"
	ShellPlanContextSourceKey  = "source"
	ShellPlanContextSourceVal  = "shell"
	ShellPlanContextHistoryKey = "history_only"
	ShellPlanContextLockKey    = "lock"
	ShellPlanContextApprovable = "approvable"

	// ShellPlanDescriptionPrefix labels history rows in UI / list.
	ShellPlanDescriptionPrefix = "Shell preview (not for apply)"

	// ErrProviderMirrorUnavailable no internal network_mirror configured (§12 / §10.2).
	ErrProviderMirrorUnavailable = "provider_unreachable"

	// EnvProviderNetworkMirror platform mirror base URL (required before shell_plan create).
	EnvProviderNetworkMirror = "IAC_PROVIDER_NETWORK_MIRROR"
)

// ErrShellPlanSessionInactive the shell session cannot accept a new plan.
var ErrShellPlanSessionInactive = errors.New("shell session is not active")

// ErrShellPlanNotShellDebug driver does not support Shell plans.
var ErrShellPlanNotShellDebug = errors.New("shell plans require driver=shell_debug")

// providerMirrorConfigured reports whether the platform has a provider
// network_mirror. Overridable in tests via SetProviderMirrorConfiguredForTest.
var providerMirrorConfigured = func() bool {
	return strings.TrimSpace(os.Getenv(EnvProviderNetworkMirror)) != ""
}

// SetProviderMirrorConfiguredForTest overrides the mirror probe (tests only).
func SetProviderMirrorConfiguredForTest(fn func() bool) (restore func()) {
	prev := providerMirrorConfigured
	if fn == nil {
		providerMirrorConfigured = func() bool {
			return strings.TrimSpace(os.Getenv(EnvProviderNetworkMirror)) != ""
		}
	} else {
		providerMirrorConfigured = fn
	}
	return func() { providerMirrorConfigured = prev }
}

// ProviderMirrorConfigured exposes the mirror probe for handlers.
func ProviderMirrorConfigured() bool { return providerMirrorConfigured() }

// TaskExcludedFromApproval reports task types that must never feed Deploy
// approval hashes (§10.2 hard rule 1 / 7). shell_plan and ordinary plan /
// drift_check previews are excluded; only plan_and_apply (via approval run)
// may enter the approval pipeline.
func TaskExcludedFromApproval(task *models.WorkspaceTask) bool {
	if task == nil {
		return true
	}
	switch task.TaskType {
	case models.TaskTypeShellPlan, models.TaskTypePlan, models.TaskTypeDriftCheck:
		return true
	default:
		return false
	}
}

// IsShellPlanTask reports task_type=shell_plan.
func IsShellPlanTask(task *models.WorkspaceTask) bool {
	return task != nil && task.TaskType == models.TaskTypeShellPlan
}

// ShellPlanContext builds the history metadata stamped onto workspace_tasks.context.
func ShellPlanContext(shellSessionID, sandboxID string) models.JSONB {
	return models.JSONB{
		ShellPlanContextSessionKey: shellSessionID,
		ShellPlanContextSandboxKey: sandboxID,
		ShellPlanContextSourceKey:  ShellPlanContextSourceVal,
		ShellPlanContextHistoryKey: true,
		ShellPlanContextLockKey:    false,
		ShellPlanContextApprovable: false,
	}
}

// ShellPlanCreateResult is the persisted history row (never an approval input).
type ShellPlanCreateResult struct {
	Task *models.WorkspaceTask
}

// CreateShellPlanHistory records a shell_plan workspace task for the active
// session. Does not mint approval runs, does not apply, does not inject cloud
// STS. Caller must already have verified ProviderMirrorConfigured(); when the
// mirror is missing the handler returns 503 without calling this.
//
// Status stays pending: the task queue never selects shell_plan (history
// read-only). Future execution knives may advance status; they must surface
// cloud-identity / mirror failures and never silent-success.
func CreateShellPlanHistory(ctx context.Context, db *gorm.DB, sess *models.ShellSession, sb *models.Sandbox, createdBy string) (*ShellPlanCreateResult, error) {
	if sess == nil || sb == nil {
		return nil, fmt.Errorf("session and sandbox required")
	}
	if sess.Status != models.ShellSessionStatusActive || sess.ClosedAt != nil {
		return nil, ErrShellPlanSessionInactive
	}
	if !sess.ExpiresAt.After(time.Now()) {
		return nil, ErrShellPlanSessionInactive
	}
	if sb.Driver != models.SandboxDriverShellDebug {
		return nil, ErrShellPlanNotShellDebug
	}
	if createdBy == "" {
		createdBy = "unknown"
	}
	uid := createdBy
	desc := ShellPlanDescriptionPrefix
	task := &models.WorkspaceTask{
		WorkspaceID:   sb.WorkspaceID,
		TaskType:      models.TaskTypeShellPlan,
		Status:        models.TaskStatusPending,
		ExecutionMode: models.ExecutionModeLocal, // history row; not dispatched by queue
		CreatedBy:     &uid,
		Stage:         "pending",
		Description:   desc,
		Context:       ShellPlanContext(sess.ID, sb.ID),
	}
	// Persist only history columns (avoid depending on every workspace_tasks
	// column existing in unit-test SQLite fixtures).
	if err := db.WithContext(ctx).Select(
		"WorkspaceID", "TaskType", "Status", "ExecutionMode", "CreatedBy",
		"Stage", "Description", "Context", "CreatedAt", "UpdatedAt",
	).Create(task).Error; err != nil {
		return nil, fmt.Errorf("create shell_plan history: %w", err)
	}
	if !TaskExcludedFromApproval(task) {
		return nil, fmt.Errorf("shell_plan must be excluded from approval")
	}
	return &ShellPlanCreateResult{Task: task}, nil
}

// ListShellPlanHistory returns shell_plan tasks for a shell session (newest first).
// Filters in Go so SQLite tests and Postgres share the same path (context JSON).
func ListShellPlanHistory(ctx context.Context, db *gorm.DB, workspaceID, shellSessionID string) ([]models.WorkspaceTask, error) {
	var rows []models.WorkspaceTask
	q := db.WithContext(ctx).
		Select("id", "workspace_id", "task_type", "status", "description", "context",
			"created_at", "completed_at", "error_code", "error_message", "created_by").
		Where("workspace_id = ? AND task_type = ?", workspaceID, models.TaskTypeShellPlan).
		Order("created_at DESC")
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]models.WorkspaceTask, 0, len(rows))
	for _, t := range rows {
		if shellPlanSessionID(&t) == shellSessionID {
			out = append(out, t)
		}
	}
	return out, nil
}

func shellPlanSessionID(t *models.WorkspaceTask) string {
	if t == nil || t.Context == nil {
		return ""
	}
	v, _ := t.Context[ShellPlanContextSessionKey].(string)
	return v
}
