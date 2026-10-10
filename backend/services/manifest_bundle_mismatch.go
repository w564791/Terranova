package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// AgentBundleMismatchSource the audit source of an executor-reported bundle
// hash mismatch.
const AgentBundleMismatchSource = "agent"

// RecordAgentBundleHashMismatch handles an executor's report that the
// manifest bundle it received did not hash to the bundle_hash it was given
// (task error_code bundle_hash_mismatch).
//
// The agent is not trusted to mark the version: the report is written as an
// audit row (MANIFEST_VERSION, version.bundle_hash_mismatch, source agent,
// version_marked false) plus a WARN security log, then the platform re-hashes
// the stored files of the version (manifestbundle.VerifyForUse). Only if its
// own check fails is the version marked hash_mismatch (VerifyForUse writes its
// own audit row, source platform_recheck). The version is the task's Run
// version (external_files.manifest_version_id) or the workspace's deployed
// version; a draft Run has none (audit only).
//
// Returns whether the platform's own check failed.
func RecordAgentBundleHashMismatch(ctx context.Context, db *gorm.DB, task *models.WorkspaceTask, agentID string) (bool, error) {
	versionID, manifestID, err := bundleVersionOfTask(ctx, db, task)
	if err != nil {
		log.Printf("[WARN] [security] agent bundle mismatch for task %d: version lookup failed: %v", task.ID, err)
	}
	log.Printf("[WARN] [security] agent %q reported manifest bundle hash_mismatch for task %d (workspace %s, version %s); re-verifying stored files",
		agentID, task.ID, task.WorkspaceID, versionID)
	raw, _ := json.Marshal(map[string]interface{}{
		"level": "WARN", "source": AgentBundleMismatchSource, "agent_id": agentID,
		"task_id": task.ID, "workspace_id": task.WorkspaceID,
		"manifest_id": manifestID, "version_id": versionID,
		"reason": manifestbundle.ReasonHashMismatch, "version_marked": false,
	})
	if err := db.WithContext(ctx).Create(&models.AuditLog{
		Action: "version.bundle_hash_mismatch", ResourceType: "MANIFEST_VERSION", NewValues: string(raw),
	}).Error; err != nil {
		log.Printf("[WARN] [security] could not write agent bundle mismatch audit row (task %d): %v", task.ID, err)
	}
	if versionID == "" {
		return false, err
	}
	bundle, err := manifestbundle.OpenVersion(ctx, db, "", versionID)
	if err != nil {
		return false, fmt.Errorf("re-verify version %s: %w", versionID, err)
	}
	verr := manifestbundle.VerifyForUse(ctx, db, bundle, manifestbundle.MismatchEvent{
		ManifestID: manifestID, VersionID: versionID,
		Source: fmt.Sprintf("platform_recheck:agent_report task=%d", task.ID),
	})
	if verr != nil {
		log.Printf("[WARN] [security] platform re-check of version %s after agent report (task %d): %v", versionID, task.ID, verr)
		return true, nil
	}
	log.Printf("[security] platform re-check of version %s after agent report (task %d): stored files intact; version not marked", versionID, task.ID)
	return false, nil
}

func bundleVersionOfTask(ctx context.Context, db *gorm.DB, task *models.WorkspaceTask) (versionID, manifestID string, err error) {
	if taskUsesExternalFiles(task) {
		versionID, _ = task.ExternalFiles["manifest_version_id"].(string)
	} else {
		var ws models.Workspace
		if err := db.WithContext(ctx).Select("workspace_id", "manifest_deployment_id", "manifest_active_tag").
			Where("workspace_id = ?", task.WorkspaceID).Take(&ws).Error; err != nil {
			return "", "", err
		}
		if ws.ManifestDeploymentID == nil || ws.ManifestActiveTag == nil {
			return "", "", nil
		}
		if err := db.WithContext(ctx).Raw(`
			SELECT mv.id FROM manifest_versions mv
			  JOIN manifest_deployments md ON md.version_id = mv.id
			 WHERE md.id = ? AND mv.version = ?`, *ws.ManifestDeploymentID, *ws.ManifestActiveTag).
			Scan(&versionID).Error; err != nil {
			return "", "", err
		}
	}
	if versionID == "" {
		return "", "", nil
	}
	err = db.WithContext(ctx).Table("manifest_versions").Select("manifest_id").Where("id = ?", versionID).Scan(&manifestID).Error
	return versionID, manifestID, err
}
