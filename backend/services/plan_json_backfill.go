package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"gorm.io/gorm"

	"iac-platform/internal/models"
)

// PlanJSONRedactionVersion the redaction rules a plan_json row was last
// rewritten with (workspace_tasks.plan_json_redaction_version). Bump it when
// RedactPlanJSON gains rules, and the backfill rewrites every row again.
const PlanJSONRedactionVersion = 1

// PlanJSONBackfillResult what a backfill run did.
type PlanJSONBackfillResult struct {
	Scanned, Rewritten, Unchanged, Skipped int
}

// BackfillPlanJSONRedaction rewrites historical workspace_tasks.plan_json
// with RedactPlanJSON (HCL markers + the task's platform-side sensitive set,
// best effort: PlanSensitivityForTask from the task's variable snapshot and
// override snapshot; when they cannot be resolved the HCL markers and the
// overrides still apply), overwriting the original. Batched by id (keyset),
// one row per UPDATE.
//
// Safe to re-run and to run alongside the executor:
//   - rows whose plan_json_redaction_version >= PlanJSONRedactionVersion are
//     skipped (the marker); RedactPlanJSON is idempotent anyway, so a row
//     written already-redacted by the current executor is only marked;
//   - every UPDATE is a compare-and-set on the plan_json that was read: a
//     plan rewritten concurrently is left alone (Skipped) and picked up by
//     the next run.
func BackfillPlanJSONRedaction(ctx context.Context, db *gorm.DB, batchSize int) (PlanJSONBackfillResult, error) {
	var res PlanJSONBackfillResult
	if db == nil {
		return res, nil
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	var lastID uint
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		var ids []uint
		if err := db.WithContext(ctx).Raw(`SELECT id FROM workspace_tasks
			WHERE plan_json IS NOT NULL AND id > ?
			  AND (plan_json_redaction_version IS NULL OR plan_json_redaction_version < ?)
			ORDER BY id LIMIT ?`, lastID, PlanJSONRedactionVersion, batchSize).Scan(&ids).Error; err != nil {
			return res, fmt.Errorf("list plan_json rows: %w", err)
		}
		if len(ids) == 0 {
			return res, nil
		}
		for _, id := range ids {
			lastID = id
			res.Scanned++
			outcome, err := redactStoredPlanJSON(ctx, db, id)
			if err != nil {
				return res, err
			}
			switch outcome {
			case backfillRewritten:
				res.Rewritten++
			case backfillUnchanged:
				res.Unchanged++
			default:
				res.Skipped++
			}
		}
	}
}

type backfillOutcome int

const (
	backfillSkipped backfillOutcome = iota
	backfillRewritten
	backfillUnchanged
)

func redactStoredPlanJSON(ctx context.Context, db *gorm.DB, id uint) (backfillOutcome, error) {
	var task models.WorkspaceTask
	if err := db.WithContext(ctx).
		Select("id", "workspace_id", "plan_json", "variable_snapshot_id", "variable_overrides", "sensitive_keys").
		First(&task, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return backfillSkipped, nil
		}
		return backfillSkipped, fmt.Errorf("load task %d: %w", id, err)
	}
	var original []byte
	if err := db.WithContext(ctx).Raw(`SELECT plan_json::text FROM workspace_tasks WHERE id = ?`, id).Row().Scan(&original); err != nil {
		return backfillSkipped, fmt.Errorf("read plan_json of task %d: %w", id, err)
	}
	if task.PlanJSON == nil {
		return backfillSkipped, nil
	}
	ps, psErr := PlanSensitivityForTask(db.WithContext(ctx), &task)
	if psErr != nil {
		log.Printf("[plan_json backfill] task %d: platform sensitivity incomplete (HCL markers + overrides only): %v", id, psErr)
	}
	redacted := RedactPlanJSON(task.PlanJSON, ps)
	newJSON, err := json.Marshal(redacted)
	if err != nil {
		return backfillSkipped, fmt.Errorf("encode plan_json of task %d: %w", id, err)
	}
	oldJSON, _ := json.Marshal(map[string]interface{}(task.PlanJSON))
	outcome := backfillRewritten
	if bytes.Equal(oldJSON, newJSON) {
		outcome = backfillUnchanged
	}
	q := db.WithContext(ctx).Exec(`UPDATE workspace_tasks
		SET plan_json = CAST(? AS jsonb), plan_json_redaction_version = ?
		WHERE id = ? AND plan_json = CAST(? AS jsonb)`, string(newJSON), PlanJSONRedactionVersion, id, string(original))
	if q.Error != nil {
		return backfillSkipped, fmt.Errorf("rewrite plan_json of task %d: %w", id, q.Error)
	}
	if q.RowsAffected == 0 {
		return backfillSkipped, nil
	}
	return outcome, nil
}

// ResourceChangeBackfillResult what a resource-change backfill run did.
type ResourceChangeBackfillResult struct {
	Tasks, Rederived, Purged int // Rederived / Purged count rows
}

// BackfillResourceChangeRedaction rewrites historical
// workspace_task_resource_changes rows (redaction_version NULL or lower than
// ResourceChangesRedactionVersion), which older agents may have uploaded
// with raw before/after values:
//   - task has plan_json: before / after / after_unknown are re-derived from
//     it, redacted again with RedactPlanJSON and the task's platform-side
//     sensitive set (best effort, PlanSensitivityForTask), matched by
//     resource address; apply status, resource id etc. are kept. A row whose
//     address is not in the plan is purged as below.
//   - no plan_json: before / after / after_unknown are set to NULL and
//     details_purged = true (address / type / name / action kept).
//
// Batched by task_id (keyset). Safe to re-run and to run alongside new
// writes: only unmarked rows are updated (each UPDATE re-checks the marker),
// rows written by the platform derivation already carry the current version.
func BackfillResourceChangeRedaction(ctx context.Context, db *gorm.DB, batchSize int) (ResourceChangeBackfillResult, error) {
	var res ResourceChangeBackfillResult
	if db == nil {
		return res, nil
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	var lastTask uint
	for {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		var taskIDs []uint
		if err := db.WithContext(ctx).Raw(`SELECT DISTINCT task_id FROM workspace_task_resource_changes
			WHERE task_id > ? AND (redaction_version IS NULL OR redaction_version < ?)
			ORDER BY task_id LIMIT ?`, lastTask, ResourceChangesRedactionVersion, batchSize).Scan(&taskIDs).Error; err != nil {
			return res, fmt.Errorf("list resource change tasks: %w", err)
		}
		if len(taskIDs) == 0 {
			return res, nil
		}
		for _, id := range taskIDs {
			lastTask = id
			res.Tasks++
			re, pu, err := redactTaskResourceChanges(ctx, db, id)
			if err != nil {
				return res, err
			}
			res.Rederived += re
			res.Purged += pu
		}
	}
}

func redactTaskResourceChanges(ctx context.Context, db *gorm.DB, taskID uint) (rederived, purged int, err error) {
	db = db.WithContext(ctx)
	var rows []models.WorkspaceTaskResourceChange
	if err := db.Select("id", "resource_address").
		Where("task_id = ? AND (redaction_version IS NULL OR redaction_version < ?)", taskID, ResourceChangesRedactionVersion).
		Find(&rows).Error; err != nil {
		return 0, 0, fmt.Errorf("load resource changes of task %d: %w", taskID, err)
	}

	derived := map[string]*models.WorkspaceTaskResourceChange{}
	var task models.WorkspaceTask
	err = db.Select("id", "workspace_id", "task_type", "plan_json", "variable_snapshot_id", "variable_overrides", "sensitive_keys").
		First(&task, taskID).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return 0, 0, fmt.Errorf("load task %d: %w", taskID, err)
	}
	if err == nil && len(task.PlanJSON) > 0 {
		ps, psErr := PlanSensitivityForTask(db, &task)
		if psErr != nil {
			log.Printf("[resource_changes backfill] task %d: platform sensitivity incomplete (HCL markers + overrides only): %v", taskID, psErr)
		}
		changes, perr := NewPlanParserService(db).parseResourceChanges(RedactPlanJSON(task.PlanJSON, ps), task.TaskType == models.TaskTypeDriftCheck)
		if perr != nil {
			return 0, 0, fmt.Errorf("derive resource changes of task %d: %w", taskID, perr)
		}
		for _, ch := range changes {
			derived[ch.ResourceAddress] = ch
		}
	}

	marker := "(redaction_version IS NULL OR redaction_version < ?)"
	for _, row := range rows {
		var q *gorm.DB
		if d, ok := derived[row.ResourceAddress]; ok {
			q = db.Model(&models.WorkspaceTaskResourceChange{}).
				Where("id = ? AND "+marker, row.ID, ResourceChangesRedactionVersion).
				Updates(map[string]interface{}{
					"changes_before":    jsonbOrNull(d.ChangesBefore),
					"changes_after":     jsonbOrNull(d.ChangesAfter),
					"after_unknown":     jsonbOrNull(d.AfterUnknown),
					"details_purged":    false,
					"redaction_version": ResourceChangesRedactionVersion,
				})
			if q.Error == nil && q.RowsAffected > 0 {
				rederived++
			}
		} else {
			q = db.Model(&models.WorkspaceTaskResourceChange{}).
				Where("id = ? AND "+marker, row.ID, ResourceChangesRedactionVersion).
				Updates(map[string]interface{}{
					"changes_before":    gorm.Expr("NULL"),
					"changes_after":     gorm.Expr("NULL"),
					"after_unknown":     gorm.Expr("NULL"),
					"details_purged":    true,
					"redaction_version": ResourceChangesRedactionVersion,
				})
			if q.Error == nil && q.RowsAffected > 0 {
				purged++
			}
		}
		if q.Error != nil {
			return rederived, purged, fmt.Errorf("rewrite resource change %d of task %d: %w", row.ID, taskID, q.Error)
		}
	}
	return rederived, purged, nil
}

// jsonbOrNull a JSONB value for a map update (SQL NULL for nil).
func jsonbOrNull(v models.JSONB) interface{} {
	if v == nil {
		return gorm.Expr("NULL")
	}
	return v
}
