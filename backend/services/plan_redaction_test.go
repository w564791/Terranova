package services

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const redactionSecret = "hunter2-SECRET-value"

// samplePlan a hand-written plan in the `terraform show -json` shape with a
// secret in every place the format can carry one.
func samplePlan() map[string]interface{} {
	raw := `{
  "format_version": "1.2",
  "variables": {"db_password": {"value": "` + redactionSecret + `"}, "region": {"value": "eu-west-1"}},
  "planned_values": {
    "outputs": {"pw": {"sensitive": true, "value": "` + redactionSecret + `"}, "name": {"sensitive": false, "value": "app"}},
    "root_module": {
      "resources": [{"address": "aws_db_instance.db", "values": {"password": "` + redactionSecret + `", "name": "app", "tags": {"k": "v"}}, "sensitive_values": {"password": true, "tags": {}}}],
      "child_modules": [{"resources": [{"address": "module.m.x.y", "values": {"list": ["a", "` + redactionSecret + `"]}, "sensitive_values": {"list": [false, true]}}]}]
    }
  },
  "prior_state": {"values": {"outputs": {"pw": {"sensitive": true, "value": "` + redactionSecret + `"}},
    "root_module": {"resources": [{"address": "aws_db_instance.db", "values": {"password": "` + redactionSecret + `"}, "sensitive_values": {"password": true}}]}}},
  "resource_changes": [{
    "address": "aws_db_instance.db", "type": "aws_db_instance", "name": "db",
    "change": {"actions": ["update"],
      "before": {"password": "` + redactionSecret + `", "name": "app", "nested": {"token": "` + redactionSecret + `", "ok": 1}},
      "after": {"password": "` + redactionSecret + `x", "name": "app2", "nested": {"token": "` + redactionSecret + `", "ok": 2}},
      "before_sensitive": {"password": true, "nested": {"token": true}},
      "after_sensitive": {"password": true, "nested": {"token": true}},
      "after_unknown": {}}
  }],
  "resource_drift": [{"address": "aws_db_instance.db", "change": {"actions": ["update"], "before": {"password": "` + redactionSecret + `"}, "after": {"password": "y` + redactionSecret + `"}, "before_sensitive": {"password": true}, "after_sensitive": {"password": true}}}],
  "output_changes": {"pw": {"actions": ["create"], "before": null, "after": "` + redactionSecret + `", "before_sensitive": false, "after_sensitive": true},
                     "name": {"actions": ["create"], "before": null, "after": "app", "before_sensitive": false, "after_sensitive": false}},
  "configuration": {
    "provider_config": {"aws": {"name": "aws", "expressions": {"access_key": {"constant_value": "` + redactionSecret + `"}, "assume_role": [{"role_arn": {"constant_value": "` + redactionSecret + `"}}], "region": {"references": ["var.region"]}}}},
    "root_module": {"variables": {"db_password": {"sensitive": true, "default": "` + redactionSecret + `"}, "region": {"default": "eu-west-1"}}}
  }
}`
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		panic(err)
	}
	return m
}

func TestRedactPlanJSON_AllSensitivePlaces(t *testing.T) {
	plan := samplePlan()
	before, _ := json.Marshal(plan)
	red := RedactPlanJSON(plan, nil)
	b, _ := json.Marshal(red)
	if strings.Contains(string(b), redactionSecret) {
		t.Fatalf("secret survived redaction:\n%s", b)
	}
	if after, _ := json.Marshal(plan); string(after) != string(before) {
		t.Fatal("input plan modified")
	}
	// non-sensitive data kept
	for _, keep := range []string{`"name":"app2"`, `"ok":2`, `"region":{"value":"eu-west-1"}`, `"references":["var.region"]`, `"tags":{"k":"v"}`, `"actions":["update"]`} {
		if !strings.Contains(string(b), keep) {
			t.Errorf("lost non-sensitive %s", keep)
		}
	}
	if !strings.Contains(string(b), `"after":"app"`) {
		t.Error("non-sensitive output redacted")
	}
	// idempotent, and the hash is over the redacted form
	again := RedactPlanJSON(red, nil)
	h1, _ := RedactedPlanHash(red)
	h2, _ := RedactedPlanHash(again)
	if h1 != h2 || len(h1) != 64 {
		t.Fatalf("hash not stable: %s %s", h1, h2)
	}
	stored, h3, err := RedactPlanForStorage(plan, nil)
	if err != nil || h3 != h1 {
		t.Fatalf("RedactPlanForStorage hash %s != %s (%v)", h3, h1, err)
	}
	if sb, _ := json.Marshal(stored); strings.Contains(string(sb), redactionSecret) {
		t.Fatal("stored plan not redacted")
	}
	// a different secret gives the same redacted plan => same plan_hash (the
	// hash never encodes secret material)
	other := samplePlan()
	other["variables"].(map[string]interface{})["db_password"].(map[string]interface{})["value"] = "another"
	if h4, _ := RedactedPlanHash(RedactPlanJSON(other, nil)); h4 != h1 {
		t.Fatal("plan_hash depends on a sensitive value")
	}
	if RedactPlanJSON(nil, nil) != nil {
		t.Fatal("nil plan")
	}
}

// Real Terraform: a sensitive variable flowing into a resource and an output
// never survives redaction of the actual `terraform show -json` output.
func TestRedactPlanJSON_RealTerraformPlan(t *testing.T) {
	bin := terraformBinary(t)
	dir := t.TempDir()
	cfg := `variable "secret" {
  type      = string
  sensitive = true
}
variable "plain" {
  type    = string
  default = "visible-value"
}
resource "terraform_data" "x" {
  input = { secret = var.secret, plain = var.plain }
}
output "s" {
  value     = var.secret
  sensitive = true
}
output "p" {
  value = var.plain
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "v.tfvars"), []byte(`secret = "`+redactionSecret+`"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(sanitizeTerraformEnv(os.Environ()), "TF_IN_AUTOMATION=1", "CHECKPOINT_DISABLE=1")
	run := func(args ...string) []byte {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			stderr := ""
			if ee, ok := err.(*exec.ExitError); ok {
				stderr = string(ee.Stderr)
			}
			t.Fatalf("terraform %v: %v\n%s\n%s", args, err, out, stderr)
		}
		return out
	}
	run("init", "-no-color", "-input=false")
	run("plan", "-no-color", "-input=false", "-out=plan.out", "-var-file=v.tfvars")
	raw := run("show", "-json", "plan.out")
	if !strings.Contains(string(raw), redactionSecret) {
		t.Fatal("fixture broken: raw plan JSON does not contain the secret")
	}
	var plan map[string]interface{}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(RedactPlanJSON(plan, nil))
	if strings.Contains(string(b), redactionSecret) {
		t.Fatalf("secret survived redaction of a real plan:\n%s", b)
	}
	if !strings.Contains(string(b), "visible-value") {
		t.Fatal("non-sensitive value lost")
	}
}
