package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"iac-platform/internal/models"
	"iac-platform/services"
)

// agentTaskDataFor builds the manifest part of GetTaskData for task / ws and
// decodes it on the agent side (JSON round trip like the HTTP channel).
func agentTaskDataFor(t *testing.T, e *bundleEnv, task *models.WorkspaceTask, ws *models.Workspace) *services.RemoteDataAccessor {
	t.Helper()
	taskData := map[string]interface{}{"id": task.ID, "workspace_id": ws.WorkspaceID, "task_type": task.TaskType}
	wsData := map[string]interface{}{"workspace_id": ws.WorkspaceID, "name": "ws"}
	resp := map[string]interface{}{"task": taskData, "workspace": wsData}
	services.AddManifestTaskData(context.Background(), e.db, task, ws, taskData, wsData, resp)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	json.Unmarshal(raw, &decoded)
	return services.NewRemoteDataAccessorFromTaskData(decoded)
}

// Agent / K8s hand-off: the agent receives the manifest fields and the
// verified bundle through task data and unpacks exactly the published files;
// a NULL-hash or tampered version is refused on the platform side and the
// agent fails with the same bundle_republish_required error.
func TestAgentTaskData_ManifestBundleHandoff(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	e.putDraft(t, "u1", "modules/x/main.tf", `resource "null_resource" "x" {}`)
	id, _ := e.publish(t, "v2.0.0")
	e.db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id, status) VALUES ('mfd-pub', 'mf-1', ?, 'ws-new', 'active')`, id)
	dep, tag := "mfd-pub", "v2.0.0"
	ws := &models.Workspace{WorkspaceID: "ws-new", ManifestDeploymentID: &dep, ManifestActiveTag: &tag}
	task := &models.WorkspaceTask{ID: 7, WorkspaceID: "ws-new", TaskType: models.TaskTypePlanAndApply}

	remote := agentTaskDataFor(t, e, task, ws)
	got, err := remote.GetWorkspace("ws-new")
	if err != nil || got.ManifestDeploymentID == nil || *got.ManifestDeploymentID != dep || *got.ManifestActiveTag != tag {
		t.Fatalf("agent sees no manifest workspace: %+v %v", got, err)
	}
	h, err := remote.GetManifestBundleByTag(dep, tag)
	if err != nil {
		t.Fatal(err)
	}
	local, err := services.NewLocalDataAccessor(e.db).GetManifestBundleByTag(dep, tag)
	if err != nil || local.BundleHash != h.BundleHash || string(local.Archive) != string(h.Archive) {
		t.Fatalf("agent and local hand-off differ: %v", err)
	}
	var stored string
	e.db.Raw(`SELECT bundle_hash FROM manifest_versions WHERE id = ?`, id).Scan(&stored)
	if h.BundleHash != stored {
		t.Fatalf("hand-off hash %s != stored %s", h.BundleHash, stored)
	}

	// through the executor's config generation (agent executor)
	work := t.TempDir()
	exec := services.NewTerraformExecutorWithAccessor(remote, nil)
	if err := exec.GenerateConfigFilesForTask(got, task, work); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(work, "modules/x/main.tf")); err != nil || string(b) != `resource "null_resource" "x" {}` {
		t.Fatalf("bundle file: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(work, "main.tf.json")); err == nil {
		t.Fatal("manifest workspace generated a UI main.tf.json on the agent")
	}

	// Run task (external files): no bundle shipped
	run := &models.WorkspaceTask{ID: 8, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan,
		ExternalFiles: models.JSONB{"files": []interface{}{map[string]interface{}{"path": "main.tf", "content_b64": ""}}}}
	if _, err := agentTaskDataFor(t, e, run, ws).GetManifestBundleByTag(dep, tag); err == nil {
		t.Fatal("Run task must not receive the deployment bundle")
	}

	// NULL bundle_hash: refused with the structured error
	markInvalid(t, e.db, id)
	_, err = agentTaskDataFor(t, e, task, ws).GetManifestBundleByTag(dep, tag)
	if err == nil || err.Error() != "bundle_republish_required: denylisted_file @ prod.tfvars" ||
		services.TaskErrorCode(err) != models.TaskErrorCodeBundleRepublishRequired {
		t.Fatalf("NULL hash: %v", err)
	}
}

func TestAgentTaskData_TamperedBundleRecordedAndRefused(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, _ := e.publish(t, "v2.0.0")
	e.db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id, status) VALUES ('mfd-pub', 'mf-1', ?, 'ws-new', 'active')`, id)
	e.db.Exec(`UPDATE manifest_files SET content = CAST('resource "null_resource" "evil" {}' AS BLOB) WHERE version_id = ?`, id)
	dep, tag := "mfd-pub", "v2.0.0"
	ws := &models.Workspace{WorkspaceID: "ws-new", ManifestDeploymentID: &dep, ManifestActiveTag: &tag}
	_, err := agentTaskDataFor(t, e, &models.WorkspaceTask{ID: 9, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan}, ws).
		GetManifestBundleByTag(dep, tag)
	if err == nil || err.Error() != "bundle_republish_required: hash_mismatch" {
		t.Fatalf("tampered: %v", err)
	}
	if h, r := versionRow(t, e.db, id); h != nil || r == nil || *r != "hash_mismatch" {
		t.Fatalf("hash_mismatch not recorded: %v %v", h, r)
	}
}

// Overrides reach the agent through task data (never env / logs).
func TestAgentTaskData_CarriesOverrideSnapshot(t *testing.T) {
	e := newBundleEnv(t)
	ws := &models.Workspace{WorkspaceID: "ws-new"}
	task := &models.WorkspaceTask{ID: 10, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan,
		VariableOverrides: models.JSONB{"region": "eu-west-1", "token": "t"},
		SensitiveKeys:     json.RawMessage(`["token"]`)}
	remote := agentTaskDataFor(t, e, task, ws)
	got, err := remote.GetTask(10)
	if err != nil {
		t.Fatal(err)
	}
	o := services.TaskVariableOverrides(got)
	if o.Values["region"] != "eu-west-1" || o.Values["token"] != "t" || !o.IsSensitive("token") || o.IsSensitive("region") {
		t.Fatalf("override snapshot: %+v", o)
	}
}
