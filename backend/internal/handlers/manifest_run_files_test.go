package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"
	"iac-platform/services"
)

func runInput(path, content string) services.ManifestRunFileInput {
	return services.ManifestRunFileInput{Path: path, ContentB64: base64.StdEncoding.EncodeToString([]byte(content))}
}

// storedExternalFiles round-trips the JSONB like the workspace_tasks row.
func storedExternalFiles(t *testing.T, j models.JSONB) models.JSONB {
	t.Helper()
	b, _ := json.Marshal(j)
	var out models.JSONB
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestManifestRunFiles_DraftAndVersionHandoff(t *testing.T) {
	e := newBundleEnv(t)
	ctx := context.Background()
	in := []services.ManifestRunFileInput{
		runInput("envs/prod/main.tf", "module \"x\" {\n  source = \"../../modules/x\"\n}\n"),
		runInput("modules/x/main.tf", `resource "null_resource" "x" {}`),
	}

	// draft: accepted, hash frozen with the files
	efs, problems, err := services.PrepareManifestRunFiles(ctx, e.db, in, "", manifestbundle.MismatchEvent{})
	if err != nil || len(problems) != 0 {
		t.Fatalf("draft: %v %v", problems, err)
	}
	want, _ := manifestbundle.Hash([]manifestbundle.File{
		{Path: "envs/prod/main.tf", Content: []byte("module \"x\" {\n  source = \"../../modules/x\"\n}\n"), Mode: 0o644},
		{Path: "modules/x/main.tf", Content: []byte(`resource "null_resource" "x" {}`), Mode: 0o644},
	})
	if efs["bundle_hash"] != want {
		t.Fatalf("bundle_hash %v want %s", efs["bundle_hash"], want)
	}

	// executor (local and agent use the same writeExternalFiles): unpacked
	// and verified against the frozen hash
	sub := "envs/prod"
	ws := &models.Workspace{WorkspaceID: "ws-new", ManifestSubpath: &sub}
	task := &models.WorkspaceTask{ID: 1, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan, ExternalFiles: storedExternalFiles(t, efs)}
	exec := services.NewTerraformExecutorWithAccessor(services.NewLocalDataAccessor(e.db), nil)
	work := t.TempDir()
	if err := exec.GenerateConfigFilesForTask(ws, task, work); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(work, "modules/x/main.tf")); err != nil || string(b) != `resource "null_resource" "x" {}` {
		t.Fatalf("run file: %q %v", b, err)
	}

	// files changed after validation (e.g. the task row edited): refused
	tampered := storedExternalFiles(t, efs)
	tampered["files"].([]interface{})[1].(map[string]interface{})["content_b64"] =
		base64.StdEncoding.EncodeToString([]byte(`data "external" "x" { program = ["sh"] }`))
	err = exec.GenerateConfigFilesForTask(ws, &models.WorkspaceTask{ID: 2, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan, ExternalFiles: tampered}, t.TempDir())
	if !errors.Is(err, manifestbundle.ErrIntegrity) {
		t.Fatalf("tampered run files: %v", err)
	}
	// legacy Run task without a frozen hash: refused
	legacy := storedExternalFiles(t, efs)
	delete(legacy, "bundle_hash")
	if err := exec.GenerateConfigFilesForTask(ws, &models.WorkspaceTask{ID: 3, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan, ExternalFiles: legacy}, t.TempDir()); err == nil {
		t.Fatal("Run task without bundle_hash accepted")
	}

	// published version (?version=): verified against the stored bundle_hash
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, _ := e.publish(t, "v2.0.0")
	var stored string
	e.db.Raw(`SELECT bundle_hash FROM manifest_versions WHERE id = ?`, id).Scan(&stored)
	efs, _, err = services.PrepareManifestRunFiles(ctx, e.db, []services.ManifestRunFileInput{runInput("main.tf", pubMain)}, id, manifestbundle.MismatchEvent{})
	if err != nil || efs["bundle_hash"] != stored || efs["manifest_version_id"] != id {
		t.Fatalf("version run: %v %v", efs, err)
	}
	// stored version tampered: hash_mismatch recorded, run refused
	e.db.Exec(`UPDATE manifest_files SET content = CAST('resource "null_resource" "evil" {}' AS BLOB) WHERE version_id = ?`, id)
	_, _, err = services.PrepareManifestRunFiles(ctx, e.db, []services.ManifestRunFileInput{runInput("main.tf", pubMain)}, id, manifestbundle.MismatchEvent{})
	var rr *manifestbundle.RepublishRequiredError
	if !errors.As(err, &rr) || rr.Invalid.Reason != "hash_mismatch" {
		t.Fatalf("tampered version: %v", err)
	}
	if h, r := versionRow(t, e.db, id); h != nil || r == nil || *r != "hash_mismatch" {
		t.Fatalf("hash_mismatch not recorded: %v %v", h, r)
	}
}
