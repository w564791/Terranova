package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// Manifest [Run] (editor "Run" = plan-only task with external_files).
//
// The files come from the client (the current draft, or a published version
// opened with ?version=). They are never trusted as-is:
//   - at task creation PrepareManifestRunFiles applies the publish rules to a
//     draft (bundle rules + HCL static check with the publish module-source
//     policy: a Run can only execute what could be published), or, when the
//     client says the files are a published version (manifest_version_id),
//     verifies that version (RequireValidForRun: re-hash, NULL hash refused)
//     and requires the files to hash to its bundle_hash;
//   - the bundle hash of the accepted files is stored with them
//     (external_files.bundle_hash) and the executor unpacks them through the
//     same archive + manifestbundle.Unpack hand-off as deployed bundles, so
//     the files on disk must hash to it before init.
//
// Editor reads (ListFiles / ReadFile ?version=) stay unverified by design;
// runs are where verification happens.

// ManifestRunFileInput one external_files entry of the task request.
type ManifestRunFileInput struct {
	Path       string `json:"path"`
	ContentB64 string `json:"content_b64"`
}

// ErrRunFilesMalformed external_files that cannot be decoded (400).
var ErrRunFilesMalformed = errors.New("malformed external_files")

// ErrRunFilesVersionMismatch the files are not the published version they
// claim to be (409).
var ErrRunFilesVersionMismatch = errors.New("external_files do not match the manifest version's bundle")

// PrepareManifestRunFiles validates Run files and returns the external_files
// JSONB to store ({"files": [...], "bundle_hash": ..., "manifest_version_id"?}).
// problems (422 bundle_rules_violated) is set when a draft breaks the publish
// rules. Errors: ErrRunFilesMalformed, manifestbundle.ErrVersionNotFound,
// *manifestbundle.RepublishRequiredError, ErrRunFilesVersionMismatch.
//
// ev carries request_id / user_id for the hash_mismatch record of the version.
func PrepareManifestRunFiles(ctx context.Context, db *gorm.DB, in []ManifestRunFileInput, versionID string, ev manifestbundle.MismatchEvent) (models.JSONB, []manifestbundle.Problem, error) {
	files := make([]manifestbundle.File, 0, len(in))
	for _, f := range in {
		content, err := base64.StdEncoding.DecodeString(f.ContentB64)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s is not valid base64", ErrRunFilesMalformed, f.Path)
		}
		files = append(files, manifestbundle.File{Path: f.Path, Content: content, Mode: manifestbundle.ModeRegular})
	}

	if versionID != "" {
		b, err := manifestbundle.OpenVersion(ctx, db, "", versionID)
		if err != nil {
			return nil, nil, err
		}
		ev.VersionID, ev.ManifestID, ev.Source = versionID, b.ManifestID, "manifest-run"
		if err := manifestbundle.RequireValidForRun(ctx, db, b, ev); err != nil {
			return nil, nil, err
		}
		// modes are not sent by the editor: compare against the version
		// with its stored modes applied by path
		modes := make(map[string]int, len(b.Files))
		for _, f := range b.Files {
			modes[f.Path] = f.Mode
		}
		for i := range files {
			if m, ok := modes[files[i].Path]; ok {
				files[i].Mode = m
			}
		}
		h, err := manifestbundle.Hash(files)
		if err != nil || h != b.Hash {
			return nil, nil, ErrRunFilesVersionMismatch
		}
		return runFilesJSONB(in, files, h, versionID), nil, nil
	}

	problems, err := manifestbundle.ValidateForPublish(files, manifestbundle.PublishModuleSourcePolicy(ctx, db))
	if err != nil {
		return nil, nil, err
	}
	if len(problems) > 0 {
		return nil, problems, nil
	}
	h, err := manifestbundle.Hash(files)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrRunFilesMalformed, err)
	}
	return runFilesJSONB(in, files, h, ""), nil, nil
}

func runFilesJSONB(in []ManifestRunFileInput, files []manifestbundle.File, hash, versionID string) models.JSONB {
	list := make([]interface{}, len(in))
	for i, f := range in {
		list[i] = map[string]interface{}{"path": f.Path, "content_b64": f.ContentB64, "mode": files[i].Mode}
	}
	out := models.JSONB{"files": list, "bundle_hash": hash}
	if versionID != "" {
		out["manifest_version_id"] = versionID
	}
	return out
}

// externalFilesHandoff turns a task's stored external_files into the same
// hand-off as a deployed bundle. Tasks created before Run files were
// validated carry no bundle_hash and are refused.
func externalFilesHandoff(task *models.WorkspaceTask) (*ManifestBundleHandoff, error) {
	hash, _ := task.ExternalFiles["bundle_hash"].(string)
	if hash == "" {
		return nil, fmt.Errorf("manifest run files of task %d carry no bundle_hash (created before Run files were validated); start the Run again", task.ID)
	}
	raw, _ := task.ExternalFiles["files"].([]interface{})
	files := make([]manifestbundle.File, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: entry is not an object", ErrRunFilesMalformed)
		}
		path, _ := entry["path"].(string)
		contentB64, _ := entry["content_b64"].(string)
		content, err := base64.StdEncoding.DecodeString(contentB64)
		if err != nil {
			return nil, fmt.Errorf("%w: %s is not valid base64", ErrRunFilesMalformed, path)
		}
		mode := manifestbundle.ModeRegular
		if m, ok := entry["mode"].(float64); ok {
			mode = int(m)
		} else if m, ok := entry["mode"].(int); ok {
			mode = m
		}
		files = append(files, manifestbundle.File{Path: path, Content: content, Mode: mode})
	}
	archive, err := manifestbundle.ArchiveBytes(files)
	if err != nil {
		return nil, err
	}
	versionID, _ := task.ExternalFiles["manifest_version_id"].(string)
	return &ManifestBundleHandoff{VersionID: versionID, BundleHash: hash, Archive: archive}, nil
}
