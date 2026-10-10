package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/gorm"

	"iac-platform/internal/crypto"
	"iac-platform/internal/keys"
	"iac-platform/internal/models"
)

// plan_data (the binary plan.out apply needs) holds every value of the plan
// in clear, sensitive ones included. At rest it is only ever an envelope
// (crypto.SealPlanData: per-plan data key wrapped by a KEK derived from DATA_ENCRYPTION_KEY,
// bound to the task ID, with an expiry). Only execution decrypts it: the
// local executor (apply restore), the plan parser fallback (terraform show),
// and the agent plan-task endpoint, which hands the plan to the agent that
// executes the task (pool token + task check). No other API returns it
// (models.WorkspaceTask.PlanData is json:"-").
//
// Lifetime: a plan is only applied by its own plan_and_apply task while that
// task waits (apply_pending / decision_required) or runs. plan_data is
// deleted right after a successful apply, when the task reaches any terminal
// status, and when its envelope expires (PLAN_DATA_TTL, default 7 days after
// the plan). An expired plan cannot be applied: the apply fails with
// error_code plan_expired and the plan must be re-run.

// PlanDataTTLEnv overrides DefaultPlanDataTTL (Go duration, e.g. "72h").
const PlanDataTTLEnv = "PLAN_DATA_TTL"

// DefaultPlanDataTTL how long a stored plan stays applicable.
const DefaultPlanDataTTL = 7 * 24 * time.Hour

// ErrPlanDataMissing the task has no stored plan (expired and deleted, or
// never saved).
var ErrPlanDataMissing = errors.New("plan data is empty (expired or not saved); re-run the plan")

// PlanDataTTL the configured plan lifetime.
func PlanDataTTL() time.Duration {
	if v := os.Getenv(PlanDataTTLEnv); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		log.Printf("[WARN] invalid %s=%q, using %s", PlanDataTTLEnv, v, DefaultPlanDataTTL)
	}
	return DefaultPlanDataTTL
}

// SealTaskPlanData the at-rest form of plan for task taskID.
func SealTaskPlanData(taskID uint, plan []byte) ([]byte, error) {
	return crypto.SealPlanData(taskID, plan, time.Now().Add(PlanDataTTL()))
}

// OpenTaskPlanData decrypts task.PlanData (as stored). Legacy plaintext is
// refused (crypto.ErrPlanDataNotSealed): the startup cleanup seals it first.
func OpenTaskPlanData(task *models.WorkspaceTask) ([]byte, error) {
	if task == nil || len(task.PlanData) == 0 {
		return nil, ErrPlanDataMissing
	}
	return crypto.OpenPlanData(task.ID, task.PlanData, time.Now())
}

// planDataTerminalStatuses statuses after which a plan is never applied.
var planDataTerminalStatuses = []models.TaskStatus{
	models.TaskStatusFailed, models.TaskStatusCancelled, models.TaskStatusApplied,
	models.TaskStatusSuccess, models.TaskStatusPlannedAndFinished,
}

// ClearPlanData deletes the stored plan of the given tasks.
func ClearPlanData(db *gorm.DB, taskIDs ...uint) error {
	if db == nil || len(taskIDs) == 0 {
		return nil
	}
	return db.Model(&models.WorkspaceTask{}).Where("id IN ?", taskIDs).
		UpdateColumn("plan_data", nil).Error
}

// PlanDataCleanupResult what one cleanup pass did.
type PlanDataCleanupResult struct {
	Sealed, Purged int
	// Reencrypted legacy (key version 0, JWT_SECRET-rooted) envelopes
	// re-sealed under DATA_ENCRYPTION_KEY
	Reencrypted int
}

// CleanupPlanData one pass over stored plans (idempotent; run at startup
// before pending tasks are recovered, then periodically):
//   - tasks in a terminal status: plan_data deleted;
//   - expired envelopes: deleted;
//   - legacy plaintext of a task that may still apply: sealed in place
//     (expiry = now + TTL). This is the data migration of existing rows: the
//     data key lives in the application, so SQL cannot do it; terminal
//     rows are deleted rather than encrypted because nothing reads them;
//   - legacy envelopes (key version 0): re-sealed under the current
//     DATA_ENCRYPTION_KEY with the same expiry (compare-and-set on the old
//     bytes, so idempotent and safe concurrently; only key-version-0 rows are
//     touched). Not in development legacy mode.
func CleanupPlanData(ctx context.Context, db *gorm.DB) (PlanDataCleanupResult, error) {
	var res PlanDataCleanupResult
	if db == nil {
		return res, nil
	}
	db = db.WithContext(ctx)

	purge := db.Model(&models.WorkspaceTask{}).
		Where("plan_data IS NOT NULL AND status IN ?", planDataTerminalStatuses).
		UpdateColumn("plan_data", nil)
	if purge.Error != nil {
		return res, fmt.Errorf("purge terminal plan_data: %w", purge.Error)
	}
	res.Purged += int(purge.RowsAffected)

	var rows []struct {
		ID     uint
		Prefix []byte
	}
	if err := db.Raw(`SELECT id, substr(plan_data, 1, ?) AS prefix FROM workspace_tasks WHERE plan_data IS NOT NULL`,
		crypto.PlanDataHeaderLen).Scan(&rows).Error; err != nil {
		return res, fmt.Errorf("scan plan_data: %w", err)
	}
	now := time.Now()
	for _, r := range rows {
		sealed, expires := crypto.ParsePlanDataHeader(r.Prefix)
		if sealed {
			if !now.Before(expires) {
				q := db.Model(&models.WorkspaceTask{}).Where("id = ? AND substr(plan_data, 1, ?) = ?", r.ID, crypto.PlanDataHeaderLen, r.Prefix).
					UpdateColumn("plan_data", nil)
				if q.Error != nil {
					return res, fmt.Errorf("purge expired plan_data of task %d: %w", r.ID, q.Error)
				}
				res.Purged += int(q.RowsAffected)
				continue
			}
			if kv, _ := crypto.PlanDataKeyVersion(r.Prefix); kv == 0 && !keys.LegacyEncryptionMode() {
				n, err := reencryptPlanDataRow(db, r.ID, now)
				if err != nil {
					return res, err
				}
				res.Reencrypted += n
			}
			continue
		}
		var plain []byte
		if err := db.Raw(`SELECT plan_data FROM workspace_tasks WHERE id = ?`, r.ID).Row().Scan(&plain); err != nil {
			return res, fmt.Errorf("read plan_data of task %d: %w", r.ID, err)
		}
		if len(plain) == 0 {
			continue
		}
		if ok, _ := crypto.ParsePlanDataHeader(plain); ok {
			continue // sealed concurrently
		}
		blob, err := SealTaskPlanData(r.ID, plain)
		if err != nil {
			return res, fmt.Errorf("seal plan_data of task %d: %w", r.ID, err)
		}
		q := db.Model(&models.WorkspaceTask{}).Where("id = ? AND plan_data = ?", r.ID, plain).UpdateColumn("plan_data", blob)
		if q.Error != nil {
			return res, fmt.Errorf("seal plan_data of task %d: %w", r.ID, q.Error)
		}
		res.Sealed += int(q.RowsAffected)
	}
	return res, nil
}

// reencryptPlanDataRow re-seals task id's legacy envelope (compare-and-set).
// An envelope that does not open with the legacy key is left alone (logged):
// it expires and is purged like any other.
func reencryptPlanDataRow(db *gorm.DB, id uint, now time.Time) (int, error) {
	var blob []byte
	if err := db.Raw(`SELECT plan_data FROM workspace_tasks WHERE id = ?`, id).Row().Scan(&blob); err != nil {
		return 0, fmt.Errorf("read plan_data of task %d: %w", id, err)
	}
	if kv, ok := crypto.PlanDataKeyVersion(blob); !ok || kv != 0 {
		return 0, nil // changed concurrently
	}
	sealed, err := crypto.ReencryptPlanData(id, blob, now)
	if errors.Is(err, crypto.ErrPlanDataExpired) {
		return 0, nil
	}
	if err != nil {
		log.Printf("[PlanData] task %d: legacy envelope not re-encrypted: %v", id, err)
		return 0, nil
	}
	q := db.Model(&models.WorkspaceTask{}).Where("id = ? AND plan_data = ?", id, blob).UpdateColumn("plan_data", sealed)
	if q.Error != nil {
		return 0, fmt.Errorf("re-encrypt plan_data of task %d: %w", id, q.Error)
	}
	return int(q.RowsAffected), nil
}

// CountLegacyPlanDataRows tasks whose plan_data is a legacy (key version 0,
// JWT_SECRET-rooted) envelope.
func CountLegacyPlanDataRows(ctx context.Context, db *gorm.DB) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM workspace_tasks
		WHERE plan_data IS NOT NULL AND length(plan_data) > 4
		  AND substr(plan_data, 1, 4) = 'TNPD'::bytea AND get_byte(plan_data, 4) = 1`).Scan(&n).Error
	return n, err
}
