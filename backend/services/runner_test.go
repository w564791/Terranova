package services

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/models"
)

type fakeRunner struct {
	kind     RunnerKind
	purposes map[RunPurpose]bool
	started  []RunRequest
}

func (f *fakeRunner) Kind() RunnerKind           { return f.kind }
func (f *fakeRunner) Supports(p RunPurpose) bool { return f.purposes[p] }
func (f *fakeRunner) Start(req RunRequest) error { f.started = append(f.started, req); return nil }

func TestPurposeOfTaskAndStateLock(t *testing.T) {
	cases := map[models.TaskType]RunPurpose{
		models.TaskTypePlan:         RunPurposePreview,
		models.TaskTypeDriftCheck:   RunPurposePreview,
		models.TaskTypePlanAndApply: RunPurposeApproval,
		models.TaskTypeApply:        RunPurposeApproval,
		"unknown":                   RunPurposeApproval,
	}
	for tt, want := range cases {
		task := &models.WorkspaceTask{TaskType: tt}
		if got := PurposeOfTask(task); got != want {
			t.Errorf("%s: %s want %s", tt, got, want)
		}
		// -lock=false only for previews on the HTTP backend
		if got := skipStateLock(task, true); got != (want == RunPurposePreview) {
			t.Errorf("%s: skipStateLock(http)=%v", tt, got)
		}
		if skipStateLock(task, false) {
			t.Errorf("%s: skipStateLock without HTTP backend", tt)
		}
	}
	if PurposeOfTask(nil) != RunPurposeApproval {
		t.Error("nil task must be treated as approval")
	}
}

func TestStartRun_ApprovalRequiresWorkspaceLock(t *testing.T) {
	r := &fakeRunner{kind: RunnerKindLocal, purposes: map[RunPurpose]bool{RunPurposePreview: true, RunPurposeApproval: true}}
	ws := &models.Workspace{WorkspaceID: "ws-1"}

	err := StartRun(r, RunRequest{Task: &models.WorkspaceTask{ID: 1, TaskType: models.TaskTypePlanAndApply}, Workspace: ws, Action: "plan"})
	if !errors.Is(err, ErrApprovalRequiresWorkspaceLock) || len(r.started) != 0 {
		t.Fatalf("unlocked approval run dispatched: %v", err)
	}
	err = StartRun(r, RunRequest{Task: &models.WorkspaceTask{ID: 2, TaskType: models.TaskTypePlanAndApply}, Workspace: ws, Action: "apply", WorkspaceLocked: true})
	if err != nil || len(r.started) != 1 {
		t.Fatalf("locked approval run: %v", err)
	}
	// previews need no lock
	if err := StartRun(r, RunRequest{Task: &models.WorkspaceTask{ID: 3, TaskType: models.TaskTypePlan}, Workspace: ws, Action: "plan"}); err != nil {
		t.Fatal(err)
	}
	if err := StartRun(r, RunRequest{Task: &models.WorkspaceTask{ID: 4, TaskType: models.TaskTypePlan}}); err == nil {
		t.Fatal("request without workspace accepted")
	}
}

func TestSandboxRunnerStub(t *testing.T) {
	s := SandboxRunner{}
	ws := &models.Workspace{WorkspaceID: "ws-1"}
	if err := StartRun(s, RunRequest{Task: &models.WorkspaceTask{TaskType: models.TaskTypePlanAndApply}, Workspace: ws, WorkspaceLocked: true}); err == nil ||
		!strings.Contains(err.Error(), "does not support approval") {
		t.Fatalf("sandbox must refuse approval runs: %v", err)
	}
	if err := StartRun(s, RunRequest{Task: &models.WorkspaceTask{TaskType: models.TaskTypePlan}, Workspace: ws}); !errors.Is(err, ErrSandboxRunnerNotImplemented) {
		t.Fatalf("sandbox stub: %v", err)
	}
}

func TestRunnerForExecutionMode(t *testing.T) {
	m := &TaskQueueManager{}
	for mode, want := range map[models.ExecutionMode]RunnerKind{
		models.ExecutionModeLocal: RunnerKindLocal,
		models.ExecutionModeAgent: RunnerKindAgent,
		models.ExecutionModeK8s:   RunnerKindAgent,
		"":                        RunnerKindLocal,
	} {
		if got := m.runnerFor(&models.Workspace{ExecutionMode: mode}).Kind(); got != want {
			t.Errorf("%q: %s want %s", mode, got, want)
		}
	}
	if WorkspaceLockKey("ws-a") != WorkspaceLockKey("ws-a") || WorkspaceLockKey("ws-a") == WorkspaceLockKey("ws-b") {
		t.Error("lock key not stable / distinct")
	}
}

func handoffFixture(t *testing.T) *ManifestBundleHandoff {
	t.Helper()
	files := []manifestbundle.File{
		{Path: "envs/prod/main.tf", Content: []byte("module \"m\" { source = \"../../modules/m\" }\n"), Mode: 0o644},
		{Path: "modules/m/main.tf", Content: []byte("output \"x\" { value = 1 }\n"), Mode: 0o644},
	}
	h, _ := manifestbundle.Hash(files)
	a, err := manifestbundle.ArchiveBytes(files)
	if err != nil {
		t.Fatal(err)
	}
	return &ManifestBundleHandoff{DeploymentID: "mfd-1", Tag: "v1", VersionID: "mfv-1", BundleHash: h, Archive: a}
}

func TestManifestHandoff_TaskDataRoundTrip(t *testing.T) {
	h := handoffFixture(t)
	data := map[string]interface{}{TaskDataManifestBundle: h.TaskDataPayload()}
	data = jsonRoundTrip(t, data)
	remote := NewRemoteDataAccessorFromTaskData(data)

	got, err := remote.GetManifestBundleByTag("mfd-1", "v1")
	if err != nil || got.BundleHash != h.BundleHash || string(got.Archive) != string(h.Archive) {
		t.Fatalf("round trip: %v", err)
	}
	if _, err := remote.GetManifestBundleByTag("mfd-1", "v2"); err == nil {
		t.Fatal("bundle for another tag accepted")
	}
	if _, err := NewRemoteDataAccessorFromTaskData(map[string]interface{}{}).GetManifestBundleByTag("mfd-1", "v1"); err == nil {
		t.Fatal("missing bundle must fail (never fall back to an empty main.tf.json)")
	}

	// the platform refused: the agent fails with the same structured error
	refused := jsonRoundTrip(t, map[string]interface{}{TaskDataManifestBundleError: ManifestBundleErrorPayload(
		&manifestbundle.RepublishRequiredError{Invalid: &manifestbundle.InvalidError{Reason: "hash_mismatch"}})})
	_, err = NewRemoteDataAccessorFromTaskData(refused).GetManifestBundleByTag("mfd-1", "v1")
	if err == nil || err.Error() != "bundle_republish_required: hash_mismatch" || TaskErrorCode(err) != models.TaskErrorCodeBundleRepublishRequired {
		t.Fatalf("refusal: %v (code %q)", err, TaskErrorCode(err))
	}
}

func TestUnpackManifestHandoff_ResetsAndVerifies(t *testing.T) {
	h := handoffFixture(t)
	work := t.TempDir()
	// leftovers of an earlier attempt, including a planted symlink
	os.WriteFile(filepath.Join(work, "stale.tf"), []byte("resource \"x\" \"y\" {}"), 0o644)
	os.Symlink("/etc", filepath.Join(work, "modules"))
	files, err := unpackManifestHandoff(h, work)
	if err != nil || len(files) != 2 {
		t.Fatalf("unpack: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(work, "stale.tf")); err == nil {
		t.Fatal("stale file kept")
	}
	if fi, err := os.Lstat(filepath.Join(work, "modules")); err != nil || !fi.IsDir() {
		t.Fatal("modules must be a real directory")
	}

	bad := *h
	bad.BundleHash = strings.Repeat("a", 64)
	if _, err := unpackManifestHandoff(&bad, t.TempDir()); !errors.Is(err, manifestbundle.ErrIntegrity) {
		t.Fatalf("hash mismatch: %v", err)
	} else if code, reason, msg := classifyTaskFailure(err, "x"); code != models.TaskErrorCodeBundleHashMismatch ||
		reason != manifestbundle.ReasonHashMismatch || !strings.HasPrefix(msg, "bundle_hash_mismatch: hash_mismatch") {
		t.Fatalf("executor-side hash mismatch must be reported as bundle_hash_mismatch/hash_mismatch: %q %q %q", code, reason, msg)
	}
	null := *h
	null.BundleHash = ""
	if _, err := unpackManifestHandoff(&null, t.TempDir()); TaskErrorCode(err) != models.TaskErrorCodeBundleRepublishRequired {
		t.Fatalf("NULL hash: %v", err)
	}
}

// Through the executor: the manifest branch writes exactly the bundle (no
// generated main.tf.json) and a refused bundle fails config generation.
func TestExecutorManifestBranchUsesHandoff(t *testing.T) {
	h := handoffFixture(t)
	dep, tag, sub := "mfd-1", "v1", "envs/prod"
	remote := NewRemoteDataAccessorFromTaskData(jsonRoundTrip(t, map[string]interface{}{
		TaskDataManifestBundle: h.TaskDataPayload(),
		"workspace": map[string]interface{}{"workspace_id": "ws-1", "manifest_deployment_id": dep,
			"manifest_active_tag": tag, "manifest_subpath": sub},
	}))
	got, err := remote.GetWorkspace("ws-1")
	if err != nil || got.ManifestDeploymentID == nil || *got.ManifestDeploymentID != dep || *got.ManifestActiveTag != tag || *got.ManifestSubpath != sub {
		t.Fatalf("manifest fields not parsed: %+v %v", got, err)
	}
	exec := &TerraformExecutor{dataAccessor: remote}
	work := t.TempDir()
	if err := exec.writeManifestFiles(got, work); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(work, "modules/m/main.tf"))
	if err != nil || !strings.Contains(string(b), "output") {
		t.Fatalf("bundle not unpacked: %v", err)
	}

	refused := NewRemoteDataAccessorFromTaskData(jsonRoundTrip(t, map[string]interface{}{
		TaskDataManifestBundleError: ManifestBundleErrorPayload(&manifestbundle.RepublishRequiredError{
			Invalid: &manifestbundle.InvalidError{Reason: "denylisted_file @ prod.tfvars"}}),
	}))
	err = (&TerraformExecutor{dataAccessor: refused}).writeManifestFiles(got, t.TempDir())
	if code, _, msg := classifyTaskFailure(err, "x"); code != models.TaskErrorCodeBundleRepublishRequired || msg != "bundle_republish_required: denylisted_file @ prod.tfvars" {
		t.Fatalf("refused bundle: %q %q (%v)", code, msg, err)
	}
}

func jsonRoundTrip(t *testing.T, v map[string]interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
