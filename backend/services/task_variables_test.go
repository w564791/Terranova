package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iac-platform/internal/models"
)

func overrideTestVars() []models.WorkspaceVariable {
	return []models.WorkspaceVariable{
		{VariableID: "var-3", Key: "region", Value: "us-east-1", VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatString},
		{VariableID: "var-1", Key: "db_password", Value: "s3cr\"et\nline2", VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatString, Sensitive: true, Description: "db"},
		{VariableID: "var-2", Key: "tags", Value: `{ team = "infra" }`, VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatHCL},
		{VariableID: "var-4", Key: "count", Value: "3", VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatHCL},
		{VariableID: "var-5", Key: "name_hcl", Value: "plain", VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatHCL},
		{VariableID: "var-6", Key: "AWS_SECRET_ACCESS_KEY", Value: "env-secret", VariableType: models.VariableTypeEnvironment, Sensitive: true},
	}
}

func overrideTestTask() *models.WorkspaceTask {
	return &models.WorkspaceTask{
		ID: 42, WorkspaceID: "ws-1", TaskType: models.TaskTypePlan,
		VariableOverrides: models.JSONB{"region": "eu-west-1", "api_token": "tok-123", "replicas": float64(5)},
		SensitiveKeys:     json.RawMessage(`["api_token"]`),
	}
}

func TestApplyVariableOverrides_SensitivityAndFormat(t *testing.T) {
	vars := overrideTestVars()
	// db_password sensitive in the base, overridden by a non-sensitive key:
	// stays sensitive (an override never downgrades)
	o := VariableOverrides{Values: map[string]string{"db_password": "new", "region": "eu", "api_token": "t", "tags": `{ a = 1 }`},
		SensitiveKeys: json.RawMessage(`["api_token"]`)}
	out := ApplyVariableOverrides(vars, models.VariableTypeTerraform, o)
	byKey := map[string]models.WorkspaceVariable{}
	for _, v := range out {
		byKey[v.Key] = v
	}
	if v := byKey["db_password"]; v.Value != "new" || !v.Sensitive {
		t.Fatalf("db_password: %+v", v)
	}
	if v := byKey["region"]; v.Value != "eu" || v.Sensitive {
		t.Fatalf("region: %+v", v)
	}
	if v := byKey["api_token"]; v.Value != "t" || !v.Sensitive || v.VariableType != models.VariableTypeTerraform {
		t.Fatalf("api_token: %+v", v)
	}
	if v := byKey["tags"]; v.ValueFormat != models.ValueFormatHCL || v.Value != `{ a = 1 }` {
		t.Fatalf("tags keeps HCL format: %+v", v)
	}
	if vars[0].Value != "us-east-1" {
		t.Fatal("input modified")
	}
	// NULL sensitive_keys (not computed yet) => every override key sensitive
	out = ApplyVariableOverrides(vars, models.VariableTypeTerraform, VariableOverrides{Values: map[string]string{"region": "x"}})
	for _, v := range out {
		if v.Key == "region" && !v.Sensitive {
			t.Fatal("NULL sensitive_keys must mark overrides sensitive")
		}
	}
	// environment variables are never overridden
	env := ApplyVariableOverrides(vars, models.VariableTypeEnvironment, o)
	if len(env) != len(vars) {
		t.Fatal("overrides applied to environment variables")
	}
}

func TestRenderTFVars_TerraformOnlySortedStable(t *testing.T) {
	got, err := RenderTFVars(overrideTestVars())
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "count": 3,
  "db_password": "s3cr\"et\nline2",
  "name_hcl": "plain",
  "region": "us-east-1",
  "tags": {
    "team": "infra"
  }
}
`
	if string(got) != want {
		t.Fatalf("tfvars:\n%s\nwant:\n%s", got, want)
	}
	masked, _ := RenderTFVarsMasked(overrideTestVars())
	if strings.Contains(string(masked), "s3cr") || !strings.Contains(string(masked), `"db_password": "***SENSITIVE***"`) {
		t.Fatalf("masked render leaks: %s", masked)
	}
}

// agentTaskData builds the agent task-data document the way GetTaskData
// does (variables = the snapshot slice, task overrides through
// AddManifestTaskData) and round-trips it through JSON like the HTTP channel.
func agentTaskData(t *testing.T, task *models.WorkspaceTask, vars []models.WorkspaceVariable) map[string]interface{} {
	t.Helper()
	taskData := map[string]interface{}{"id": task.ID, "workspace_id": task.WorkspaceID, "task_type": task.TaskType}
	wsData := map[string]interface{}{"workspace_id": task.WorkspaceID, "name": "ws"}
	resp := map[string]interface{}{"task": taskData, "workspace": wsData, "variables": vars}
	AddManifestTaskData(context.Background(), nil, task, &models.Workspace{WorkspaceID: task.WorkspaceID}, taskData, wsData, resp)
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

// Local and agent runners must render byte-identical variable files from the
// same snapshot + override snapshot, sensitive values included.
func TestTFVars_AgentAndLocalByteIdentical(t *testing.T) {
	vars := overrideTestVars()
	task := overrideTestTask()

	// local: snapshot cache + overrides from the task row
	local := &LocalDataAccessor{snapshotVars: append([]models.WorkspaceVariable(nil), vars...)}
	localExec := &TerraformExecutor{dataAccessor: local}
	localExec.dataAccessor.SetVariableOverrides(TaskVariableOverrides(task))

	// agent: everything through the task-data channel
	remote := NewRemoteDataAccessorFromTaskData(agentTaskData(t, task, vars))
	agentTask, err := remote.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	agentExec := &TerraformExecutor{dataAccessor: remote}
	agentExec.dataAccessor.SetVariableOverrides(TaskVariableOverrides(agentTask))

	ws := &models.Workspace{WorkspaceID: task.WorkspaceID}
	ld, ad := t.TempDir(), t.TempDir()
	for _, e := range []struct {
		exec *TerraformExecutor
		dir  string
	}{{localExec, ld}, {agentExec, ad}} {
		if err := e.exec.generateVariablesTFVars(ws, e.dir); err != nil {
			t.Fatal(err)
		}
		if err := e.exec.generateVariablesTFJSON(ws, e.dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{TFVarsFileName, "variables.tf.json"} {
		lb, _ := os.ReadFile(filepath.Join(ld, name))
		ab, _ := os.ReadFile(filepath.Join(ad, name))
		if string(lb) != string(ab) {
			t.Fatalf("%s differs:\nlocal:\n%s\nagent:\n%s", name, lb, ab)
		}
	}
	tfvars, _ := os.ReadFile(filepath.Join(ld, TFVarsFileName))
	want := `{
  "api_token": "tok-123",
  "count": 3,
  "db_password": "s3cr\"et\nline2",
  "name_hcl": "plain",
  "region": "eu-west-1",
  "replicas": "5",
  "tags": {
    "team": "infra"
  }
}
`
	if string(tfvars) != want {
		t.Fatalf("tfvars:\n%s\nwant:\n%s", tfvars, want)
	}
	var decl struct {
		Variable map[string]map[string]interface{} `json:"variable"`
	}
	b, _ := os.ReadFile(filepath.Join(ld, "variables.tf.json"))
	json.Unmarshal(b, &decl)
	if decl.Variable["api_token"]["sensitive"] != true || decl.Variable["db_password"]["sensitive"] != true {
		t.Fatalf("sensitive declarations lost: %s", b)
	}
	if _, ok := decl.Variable["replicas"]["sensitive"]; ok {
		t.Fatalf("non-sensitive override declared sensitive: %s", b)
	}
	if _, ok := decl.Variable["AWS_SECRET_ACCESS_KEY"]; ok {
		t.Fatal("environment variable declared as a Terraform variable")
	}

	// the apply-from-snapshot generator renders the same bytes
	snap := ApplyVariableOverrides(vars, models.VariableTypeTerraform, TaskVariableOverrides(task))
	if got, err := RenderTFVars(snap); err != nil || string(got) != want {
		t.Fatal("snapshot generator differs")
	}
}

func TestRemoteTaskData_NoOverridesMeansNone(t *testing.T) {
	task := &models.WorkspaceTask{ID: 1, WorkspaceID: "ws-1", TaskType: models.TaskTypePlan}
	remote := NewRemoteDataAccessorFromTaskData(agentTaskData(t, task, overrideTestVars()))
	got, _ := remote.GetTask(1)
	if len(got.VariableOverrides) != 0 || got.SensitiveKeys != nil {
		t.Fatalf("unexpected overrides: %+v", got)
	}
}
