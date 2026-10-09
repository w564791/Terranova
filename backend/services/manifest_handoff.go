package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// ManifestBundleHandoff is what a runner receives for a manifest-managed
// workspace: the hand-off archive of the deployed version's bundle and the
// bundle_hash the platform verified it against (RequireValidForRun). The
// runner unpacks it with manifestbundle.Unpack, which re-checks every entry
// and the hash before terraform init (unpackManifestHandoff).
//
// Local mode builds it in-process (LocalDataAccessor.GetManifestBundleByTag);
// agent / K8s mode receives the same structure through the agent task-data
// channel (GetTaskData "manifest_bundle", RemoteDataAccessor).
type ManifestBundleHandoff struct {
	DeploymentID string
	Tag          string
	VersionID    string
	BundleHash   string
	Archive      []byte

	// Files the verified bundle files (platform side only; not part of the
	// agent payload, where only Archive travels).
	Files []manifestbundle.File
}

// LoadRunnableManifestBundle resolves deployment + tag to the deployed version
// and builds its hand-off. The version must pass RequireValidForRun: files
// re-hashed against bundle_hash (a mismatch is recorded on recordDB as the
// sticky hash_mismatch with its audit / security log) and bundle_hash not
// NULL; otherwise *manifestbundle.RepublishRequiredError. Returns (nil, nil)
// when the deployment / tag no longer resolves to a version.
func LoadRunnableManifestBundle(ctx context.Context, readDB, recordDB *gorm.DB, deploymentID, tag string) (*ManifestBundleHandoff, error) {
	var versionID string
	if err := readDB.WithContext(ctx).Raw(`
		SELECT mv.id
		  FROM manifest_versions mv
		  JOIN manifest_deployments md ON md.version_id = mv.id
		 WHERE md.id = ? AND mv.version = ?
	`, deploymentID, tag).Scan(&versionID).Error; err != nil {
		return nil, err
	}
	if versionID == "" {
		return nil, nil
	}
	bundle, err := manifestbundle.OpenVersion(ctx, readDB, "", versionID)
	if err != nil {
		return nil, err
	}
	if err := manifestbundle.RequireValidForRun(ctx, recordDB, bundle, manifestbundle.MismatchEvent{
		VersionID: versionID, Source: "runner:deployment=" + deploymentID,
	}); err != nil {
		return nil, err
	}
	archive, err := manifestbundle.ArchiveBytes(bundle.Files)
	if err != nil {
		return nil, fmt.Errorf("pack bundle of version %s: %w", versionID, err)
	}
	return &ManifestBundleHandoff{
		DeploymentID: deploymentID, Tag: tag,
		VersionID: versionID, BundleHash: bundle.Hash, Archive: archive, Files: bundle.Files,
	}, nil
}

// Agent task-data payload keys (GetTaskData "manifest_bundle" /
// "manifest_bundle_error").
const (
	TaskDataManifestBundle      = "manifest_bundle"
	TaskDataManifestBundleError = "manifest_bundle_error"
)

// TaskDataPayload the "manifest_bundle" object of the agent task data.
func (h *ManifestBundleHandoff) TaskDataPayload() map[string]interface{} {
	return map[string]interface{}{
		"deployment_id": h.DeploymentID,
		"tag":           h.Tag,
		"version_id":    h.VersionID,
		"bundle_hash":   h.BundleHash,
		"archive_b64":   base64.StdEncoding.EncodeToString(h.Archive),
	}
}

// ManifestBundleErrorPayload the "manifest_bundle_error" object: the platform
// refused to hand the bundle out. Only rule names / hash_mismatch travel.
func ManifestBundleErrorPayload(err error) map[string]interface{} {
	var rr *manifestbundle.RepublishRequiredError
	if errors.As(err, &rr) {
		return map[string]interface{}{"republish_required": true, "reason": rr.Invalid.Reason}
	}
	return map[string]interface{}{"message": err.Error()}
}

// manifestHandoffFromTaskData decodes the agent task-data form.
func manifestHandoffFromTaskData(data map[string]interface{}, deploymentID, tag string) (*ManifestBundleHandoff, error) {
	if e, ok := data[TaskDataManifestBundleError].(map[string]interface{}); ok {
		if rr, _ := e["republish_required"].(bool); rr {
			reason, _ := e["reason"].(string)
			return nil, &manifestbundle.RepublishRequiredError{
				Invalid: &manifestbundle.InvalidError{Reason: reason},
			}
		}
		msg, _ := e["message"].(string)
		return nil, fmt.Errorf("platform refused the manifest bundle: %s", msg)
	}
	p, ok := data[TaskDataManifestBundle].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("manifest bundle missing from task data (deployment=%s, tag=%s)", deploymentID, tag)
	}
	h := &ManifestBundleHandoff{
		DeploymentID: getString(p, "deployment_id"),
		Tag:          getString(p, "tag"),
		VersionID:    getString(p, "version_id"),
		BundleHash:   getString(p, "bundle_hash"),
	}
	if h.DeploymentID != deploymentID || h.Tag != tag {
		return nil, fmt.Errorf("manifest bundle in task data is for deployment=%s tag=%s, workspace wants deployment=%s tag=%s",
			h.DeploymentID, h.Tag, deploymentID, tag)
	}
	archive, err := base64.StdEncoding.DecodeString(getString(p, "archive_b64"))
	if err != nil {
		return nil, fmt.Errorf("decode manifest bundle archive: %w", err)
	}
	h.Archive = archive
	return h, nil
}

// unpackManifestHandoff unpacks a hand-off into workDir. workDir is the
// task's own directory and the bundle is the first thing written to it, so
// leftovers of an earlier attempt are removed first (Unpack creates every
// file with O_EXCL and would refuse them). On return without error the files
// on disk hash to h.BundleHash.
func unpackManifestHandoff(h *ManifestBundleHandoff, workDir string) ([]manifestbundle.File, error) {
	if h == nil {
		return nil, fmt.Errorf("no manifest bundle")
	}
	if err := resetDir(workDir); err != nil {
		return nil, err
	}
	files, err := manifestbundle.UnpackBytes(h.Archive, workDir, h.BundleHash, manifestbundle.DefaultLimits())
	if err != nil {
		if errors.Is(err, manifestbundle.ErrIntegrity) {
			log.Printf("[SECURITY] manifest bundle hand-off integrity failure: version=%s expected=%s: %v",
				h.VersionID, h.BundleHash, err)
		}
		return nil, fmt.Errorf("unpack manifest bundle (version %s): %w", h.VersionID, err)
	}
	return files, nil
}

// resetDir empties dir (creating it 0700 when missing). dir itself must not
// be a symlink.
func resetDir(dir string) error {
	fi, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(dir, 0o700)
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("work directory %s is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("clear work directory: %w", err)
		}
	}
	return nil
}

// AddManifestTaskData adds what an agent / K8s runner needs for manifest
// tasks to the GetTaskData response (taskData / workspaceData are its "task"
// and "workspace" objects, response the top level):
//   - task.variable_overrides / task.override_sensitive_keys: the task's
//     override snapshot (RemoteDataAccessor.SetVariableOverrides);
//   - task.external_files: Manifest [Run] draft files;
//   - workspace.manifest_deployment_id / manifest_active_tag /
//     manifest_subpath;
//   - manifest_bundle (verified hand-off) or manifest_bundle_error, for every
//     task of a manifest-managed workspace that does not run external files.
func AddManifestTaskData(ctx context.Context, db *gorm.DB, task *models.WorkspaceTask, ws *models.Workspace,
	taskData, workspaceData, response map[string]interface{}) {
	if len(task.VariableOverrides) > 0 {
		taskData["variable_overrides"] = task.VariableOverrides
		if len(task.SensitiveKeys) > 0 {
			taskData["override_sensitive_keys"] = task.SensitiveKeys
		}
	}
	if len(task.ExternalFiles) > 0 {
		taskData["external_files"] = task.ExternalFiles
	}
	if ws.ManifestSubpath != nil && *ws.ManifestSubpath != "" {
		workspaceData["manifest_subpath"] = *ws.ManifestSubpath
	}
	usesManifest := ws.ManifestDeploymentID != nil && *ws.ManifestDeploymentID != "" &&
		ws.ManifestActiveTag != nil && *ws.ManifestActiveTag != ""
	if !usesManifest {
		return
	}
	workspaceData["manifest_deployment_id"] = *ws.ManifestDeploymentID
	workspaceData["manifest_active_tag"] = *ws.ManifestActiveTag
	if taskUsesExternalFiles(task) {
		return // Run task: the executor ignores the deployment
	}
	h, err := LoadRunnableManifestBundle(ctx, db, db, *ws.ManifestDeploymentID, *ws.ManifestActiveTag)
	switch {
	case err != nil:
		log.Printf("[Agent] task %d: manifest bundle refused: %v", task.ID, err)
		response[TaskDataManifestBundleError] = ManifestBundleErrorPayload(err)
	case h == nil:
		response[TaskDataManifestBundleError] = map[string]interface{}{
			"message": fmt.Sprintf("no manifest version for deployment=%s tag=%s", *ws.ManifestDeploymentID, *ws.ManifestActiveTag),
		}
	default:
		response[TaskDataManifestBundle] = h.TaskDataPayload()
	}
}
