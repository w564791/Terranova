package services

import (
	"errors"
	"fmt"
	"hash/fnv"
	"log"

	"iac-platform/internal/models"
)

// Runner is the single dispatch seam of task execution. The task queue picks
// a Runner from the workspace execution mode and hands it one RunRequest; the
// runner owns where Terraform runs:
//
//   - LocalRunner: the existing in-process TerraformExecutor (CAS to running,
//     then executeTask in a goroutine);
//   - AgentRunner: the existing agent / K8s driver (pushTaskToAgent over the
//     C&C channel; the agent runs the same TerraformExecutor through
//     RemoteDataAccessor and receives the manifest bundle, the override
//     snapshot and the variables through the task-data channel);
//   - SandboxRunner: step-6 stub (preview runs in an isolated sandbox).
//
// Every runner receives a manifest bundle the same way (ManifestBundleHandoff
// unpacked by manifestbundle.Unpack, hash checked before init) and renders
// variables with the same RenderTFVars (terranova.auto.tfvars.json) / VariablesTFJSON.
type Runner interface {
	Kind() RunnerKind
	// Supports reports whether the runner may execute runs of purpose p.
	Supports(p RunPurpose) bool
	// Start dispatches the run and returns once it is handed off (execution
	// is asynchronous). It does not take locks itself: StartRun enforces the
	// purpose's locking contract before calling it.
	Start(req RunRequest) error
}

// RunnerKind identifies a Runner implementation.
type RunnerKind string

const (
	RunnerKindLocal   RunnerKind = "local"
	RunnerKindAgent   RunnerKind = "agent" // agent and K8s execution modes
	RunnerKindSandbox RunnerKind = "sandbox"
)

// RunPurpose separates read-only previews from runs that lead to an apply.
type RunPurpose string

const (
	// RunPurposePreview plan-only and drift-check runs: never change state;
	// they run without the workspace lock and without the Terraform state
	// lock, so they can run concurrently.
	RunPurposePreview RunPurpose = "preview"
	// RunPurposeApproval plan_and_apply (both its plan and its apply phase)
	// and apply runs: dispatched only while the queue holds the workspace
	// lock, and Terraform runs with the state lock (no -lock=false).
	RunPurposeApproval RunPurpose = "approval"
)

// PurposeOfTask the run purpose of a task.
func PurposeOfTask(task *models.WorkspaceTask) RunPurpose {
	if task == nil {
		return RunPurposeApproval
	}
	switch task.TaskType {
	case models.TaskTypePlan, models.TaskTypeDriftCheck:
		return RunPurposePreview
	default:
		return RunPurposeApproval
	}
}

// skipStateLock reports whether Terraform may run with -lock=false: only
// preview runs against the HTTP state backend.
func skipStateLock(task *models.WorkspaceTask, httpBackend bool) bool {
	return httpBackend && PurposeOfTask(task) == RunPurposePreview
}

// RunRequest one dispatch.
type RunRequest struct {
	Task      *models.WorkspaceTask
	Workspace *models.Workspace
	// Action "plan" or "apply" (apply = the apply phase of an approved run).
	Action string
	// WorkspaceLocked the caller holds the workspace lock
	// (WorkspaceLockKey) for the duration of Start.
	WorkspaceLocked bool
}

// Purpose of the request.
func (r RunRequest) Purpose() RunPurpose { return PurposeOfTask(r.Task) }

var (
	// ErrApprovalRequiresWorkspaceLock an approval run dispatched without the
	// workspace lock.
	ErrApprovalRequiresWorkspaceLock = errors.New("approval runs require the workspace lock")
	// ErrSandboxRunnerNotImplemented the sandbox runner arrives in step 6.
	ErrSandboxRunnerNotImplemented = errors.New("sandbox runner is not implemented yet (manifest-sandbox step 6)")
)

// StartRun enforces the runner contract and dispatches: the runner must
// support the purpose, and approval runs must hold the workspace lock.
func StartRun(r Runner, req RunRequest) error {
	if req.Task == nil || req.Workspace == nil {
		return fmt.Errorf("run request without task or workspace")
	}
	p := req.Purpose()
	if !r.Supports(p) {
		return fmt.Errorf("%s runner does not support %s runs", r.Kind(), p)
	}
	if p == RunPurposeApproval && !req.WorkspaceLocked {
		return fmt.Errorf("task %d (%s): %w", req.Task.ID, req.Task.TaskType, ErrApprovalRequiresWorkspaceLock)
	}
	return r.Start(req)
}

// WorkspaceLockKey the advisory-lock key of a workspace (FNV-64a of the
// workspace id), shared by every dispatcher of approval runs.
func WorkspaceLockKey(workspaceID string) int64 {
	h := fnv.New64a()
	h.Write([]byte(workspaceID))
	return int64(h.Sum64())
}

// LocalRunner wraps the in-process executor of the task queue.
type LocalRunner struct{ m *TaskQueueManager }

func (LocalRunner) Kind() RunnerKind         { return RunnerKindLocal }
func (LocalRunner) Supports(RunPurpose) bool { return true }

// Start marks the task running (CAS, so no other replica picks it up) and
// runs it in a goroutine. A lost CAS is not an error (someone else owns it).
func (r LocalRunner) Start(req RunRequest) error {
	if err := r.m.casTaskStatus(req.Task, req.Action); err != nil {
		log.Printf("[Runner] local: CAS failed for task %d: %v", req.Task.ID, err)
		return nil
	}
	go r.m.executeTask(req.Task, req.Action)
	return nil
}

// AgentRunner wraps the agent / K8s driver.
type AgentRunner struct{ m *TaskQueueManager }

func (AgentRunner) Kind() RunnerKind         { return RunnerKindAgent }
func (AgentRunner) Supports(RunPurpose) bool { return true }
func (r AgentRunner) Start(req RunRequest) error {
	return r.m.pushTaskToAgent(req.Task, req.Workspace)
}

// SandboxRunner step-6 stub: will run preview runs in an isolated sandbox
// (no credentials, no network but the provider mirror). Approval runs never
// go to the sandbox.
type SandboxRunner struct{}

func (SandboxRunner) Kind() RunnerKind { return RunnerKindSandbox }
func (SandboxRunner) Supports(p RunPurpose) bool {
	return p == RunPurposePreview
}
func (SandboxRunner) Start(RunRequest) error { return ErrSandboxRunnerNotImplemented }

// runnerFor the runner of a workspace's execution mode. The sandbox runner is
// not selected by any mode yet.
func (m *TaskQueueManager) runnerFor(ws *models.Workspace) Runner {
	switch ws.ExecutionMode {
	case models.ExecutionModeK8s, models.ExecutionModeAgent:
		return AgentRunner{m: m}
	default:
		return LocalRunner{m: m}
	}
}
