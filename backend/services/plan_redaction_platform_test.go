package services

import (
	"encoding/json"
	"strings"
	"testing"

	"iac-platform/internal/models"
)

const platformSecret = "varset-secret-abc123XYZ"

// a plan where api_key is NOT declared sensitive in HCL, so Terraform marks
// nothing; only the platform knows it is sensitive.
func platformPlan() map[string]interface{} {
	raw := `{
  "format_version": "1.2",
  "variables": {"api_key": {"value": "` + platformSecret + `"}, "region": {"value": "eu-west-1"}, "short": {"value": "abc"}, "ovr": {"value": "override-value-1"}},
  "planned_values": {"outputs": {"key_out": {"sensitive": false, "value": "` + platformSecret + `"}},
    "root_module": {"resources": [{"address": "x.y", "values": {"header": "Bearer ` + platformSecret + `", "region": "eu-west-1", "label": "abc"}, "sensitive_values": {}}]}},
  "resource_changes": [{"address": "x.y", "change": {"actions": ["create"], "before": null,
    "after": {"header": "Bearer ` + platformSecret + `", "nested": [{"k": "` + platformSecret + `"}], "o": "override-value-1", "region": "eu-west-1"},
    "before_sensitive": false, "after_sensitive": {}}}],
  "output_changes": {"key_out": {"actions": ["create"], "before": null, "after": "` + platformSecret + `", "before_sensitive": false, "after_sensitive": false}},
  "configuration": {"root_module": {"variables": {"api_key": {"default": "` + platformSecret + `"}, "region": {}, "short": {}, "ovr": {}}}}
}`
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		panic(err)
	}
	return m
}

func TestRedactPlanJSON_PlatformSensitiveSet(t *testing.T) {
	// api_key comes from a varset flagged sensitive; ovr is a deployment
	// override with NULL sensitive_keys (=> sensitive); short is sensitive
	// but too short to scrub by value.
	base := []models.WorkspaceVariable{
		{Key: "api_key", Value: platformSecret, VariableType: models.VariableTypeTerraform, Sensitive: true},
		{Key: "region", Value: "eu-west-1", VariableType: models.VariableTypeTerraform},
		{Key: "short", Value: "abc", VariableType: models.VariableTypeTerraform, Sensitive: true},
		{Key: "env_secret", Value: "eu-west-1", VariableType: models.VariableTypeEnvironment, Sensitive: true},
	}
	vars := ApplyVariableOverrides(base, models.VariableTypeTerraform, VariableOverrides{Values: map[string]string{"ovr": "override-value-1"}})
	ps := PlanSensitivityFromVariables(vars)

	red := RedactPlanJSON(platformPlan(), ps)
	b, _ := json.Marshal(red)
	s := string(b)
	for _, leak := range []string{platformSecret, "override-value-1"} {
		if strings.Contains(s, leak) {
			t.Fatalf("%q survived:\n%s", leak, s)
		}
	}
	vars2 := red["variables"].(map[string]interface{})
	for _, k := range []string{"api_key", "short", "ovr"} {
		if vars2[k].(map[string]interface{})["value"] != SensitivePlaceholder {
			t.Errorf("variables.%s not masked: %v", k, vars2[k])
		}
	}
	if vars2["region"].(map[string]interface{})["value"] != "eu-west-1" {
		t.Error("non-sensitive variable masked (environment variables must not count)")
	}
	cfg := red["configuration"].(map[string]interface{})["root_module"].(map[string]interface{})["variables"].(map[string]interface{})
	if cfg["api_key"].(map[string]interface{})["default"] != SensitivePlaceholder {
		t.Error("default of platform-sensitive variable not masked")
	}
	if !strings.Contains(s, `"label":"abc"`) {
		t.Error("short sensitive value must not be scrubbed from unrelated leaves")
	}
	if !strings.Contains(s, `"header":"`+SensitivePlaceholder+`"`) {
		t.Error("leaf containing the secret not replaced with the Terraform marker")
	}
	// idempotent, and nil sensitivity = HCL markers only
	again, _ := json.Marshal(RedactPlanJSON(red, ps))
	if string(again) != s {
		t.Fatal("not idempotent")
	}
	if nb, _ := json.Marshal(RedactPlanJSON(platformPlan(), nil)); !strings.Contains(string(nb), platformSecret) {
		t.Fatal("test plan should only be redacted by the platform set")
	}
}

func TestPlanSensitivity_HCLValueLeaves(t *testing.T) {
	ps := PlanSensitivityFromVariables([]models.WorkspaceVariable{{
		Key: "creds", Value: `{ user = "admin", pass = "hcl-object-secret" }`, ValueFormat: models.ValueFormatHCL,
		VariableType: models.VariableTypeTerraform, Sensitive: true,
	}})
	plan := map[string]interface{}{"resource_changes": []interface{}{map[string]interface{}{"change": map[string]interface{}{
		"after": map[string]interface{}{"p": "hcl-object-secret", "u": "admin"}}}}}
	b, _ := json.Marshal(RedactPlanJSON(plan, ps))
	if strings.Contains(string(b), "hcl-object-secret") {
		t.Fatalf("leaf of HCL value survived: %s", b)
	}
}

// PG: a variable that is not sensitive in HCL but sensitive in a varset is
// masked through the task's variable snapshot (agent upload / parser path).
func TestPlanSensitivityForTask_VarsetSensitive(t *testing.T) {
	db := setupVarsetTestDB(t)
	ws := "ws-plan-redact"
	db.Exec("INSERT INTO workspaces (workspace_id, name, execution_mode, terraform_version, workdir, state_backend) VALUES (?, 'plan-redact', 'local', 'latest', '/workspace', 'local') ON CONFLICT (workspace_id) DO NOTHING", ws)
	defer db.Exec("DELETE FROM workspaces WHERE workspace_id = ?", ws)
	wsVar := &models.WorkspaceVariable{WorkspaceID: ws, Key: "region", Value: "eu-west-1",
		VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatString, Version: 1}
	db.Create(wsVar)
	defer db.Exec("DELETE FROM workspace_variables WHERE workspace_id = ?", ws)

	vs, err := NewVariableSetService(db).Create("plan-redact-global", "test", "global", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupVarset(db, vs.VarsetID)
	if _, err := NewVarsetVariableService(db).Create(vs.VarsetID, "api_key", platformSecret, "",
		models.VariableTypeTerraform, models.ValueFormatString, true, nil); err != nil {
		t.Fatal(err)
	}
	vsnap, _, err := NewVariableSnapshotService(db).CreateSnapshot(ws, nil)
	if err != nil || vsnap == nil {
		t.Fatalf("snapshot: %v", err)
	}
	defer db.Exec("DELETE FROM variable_snapshots WHERE vsnap_id = ?", *vsnap)

	task := &models.WorkspaceTask{ID: 9001, WorkspaceID: ws, VariableSnapshotID: vsnap,
		VariableOverrides: models.JSONB{"ovr": "override-value-1"}} // sensitive_keys NULL
	ps, err := PlanSensitivityForTask(db, task)
	if err != nil {
		t.Fatal(err)
	}
	if !ps.Names["api_key"] || !ps.Names["ovr"] || ps.Names["region"] {
		t.Fatalf("names: %v", ps.Names)
	}
	b, _ := json.Marshal(RedactPlanJSON(platformPlan(), ps))
	if strings.Contains(string(b), platformSecret) || strings.Contains(string(b), "override-value-1") {
		t.Fatalf("varset-sensitive value survived: %s", b)
	}
}
