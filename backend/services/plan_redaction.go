package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"iac-platform/internal/models"
)

// SensitivePlaceholder replaces every sensitive value in a stored plan. It is
// the string Terraform's own CLI renders for sensitive values, and what the
// frontend shows (PlanCompleteView / ApplyingView / StateResourceViewer), so a
// redacted plan displays like a Terraform plan.
const SensitivePlaceholder = "(sensitive value)"

// Value-scrub thresholds (see PlanSensitivity): a string leaf equal to a
// platform-sensitive value is replaced when the value has at least
// minExactScrubLen characters; a leaf containing it when it has at least
// minSubstringScrubLen. Shorter values ("1", "true") would erase unrelated
// data; their variables are still masked by name.
const (
	minExactScrubLen     = 4
	minSubstringScrubLen = 8
)

// PlanSensitivity the platform-side sensitivity of one task's variables, the
// union RedactPlanJSON applies on top of the plan's own HCL `sensitive =
// true` markers:
//   - workspace variables flagged sensitive;
//   - variable-set variables flagged sensitive (including the varsets of the
//     active manifest deployment, folded into the snapshot);
//   - manifest deployment overrides listed in sensitive_keys, every override
//     when sensitive_keys is NULL (ApplyVariableOverrides).
//
// Names masks variables[name].value and the root-module default of name.
// Values are those variables' raw values: any string leaf of the plan that
// equals (or contains) one of them is replaced as well, which covers the
// places a variable not declared sensitive in HCL surfaces in resource
// attributes / outputs. Values derived from a secret (encoded, hashed,
// split) are not detectable this way — declaring the variable sensitive in
// HCL remains the reliable mechanism.
type PlanSensitivity struct {
	Names  map[string]bool
	Values []string
}

// PlanSensitivityFromVariables the sensitivity of resolved variables (the
// executor's GetWorkspaceVariables result: snapshot + overrides, sensitivity
// already union-ed by ApplyVariableOverrides).
func PlanSensitivityFromVariables(vars []models.WorkspaceVariable) *PlanSensitivity {
	ps := &PlanSensitivity{Names: map[string]bool{}}
	seen := map[string]bool{}
	add := func(s string) {
		if utf8.RuneCountInString(s) >= minExactScrubLen && !seen[s] {
			seen[s] = true
			ps.Values = append(ps.Values, s)
		}
	}
	for _, v := range vars {
		if !v.Sensitive || (v.VariableType != "" && v.VariableType != models.VariableTypeTerraform) {
			continue
		}
		ps.Names[v.Key] = true
		add(v.Value)
		if isHCLTypedValue(v) {
			if raw, err := tfvarJSONValue(v); err == nil {
				var decoded interface{}
				if json.Unmarshal(raw, &decoded) == nil {
					collectSensitiveLeaves(decoded, add)
				}
			}
		}
	}
	// longest first: a value containing a shorter one is matched whole
	sort.Slice(ps.Values, func(i, j int) bool {
		if len(ps.Values[i]) != len(ps.Values[j]) {
			return len(ps.Values[i]) > len(ps.Values[j])
		}
		return ps.Values[i] < ps.Values[j]
	})
	return ps
}

// PlanSensitivityForTask the platform-side sensitivity of task, resolved on
// the server (agent uploads, plan parser fallback, historical backfill): the
// task's variable snapshot (live resolution when it has none) plus its
// override snapshot. Best effort: on a resolution error the overrides alone
// are used and the error is returned alongside.
func PlanSensitivityForTask(db *gorm.DB, task *models.WorkspaceTask) (*PlanSensitivity, error) {
	if task == nil {
		return &PlanSensitivity{}, nil
	}
	acc := NewLocalDataAccessor(db)
	var loadErr error
	if task.VariableSnapshotID != nil && *task.VariableSnapshotID != "" {
		loadErr = acc.LoadSnapshot(*task.VariableSnapshotID, db)
		if loadErr == nil && acc.snapshotVars == nil {
			acc.snapshotVars = []models.WorkspaceVariable{}
		}
	}
	acc.SetVariableOverrides(TaskVariableOverrides(task))
	vars, err := acc.GetWorkspaceVariables(task.WorkspaceID, models.VariableTypeTerraform)
	if err != nil {
		vars = ApplyVariableOverrides(nil, models.VariableTypeTerraform, TaskVariableOverrides(task))
		if loadErr == nil {
			loadErr = err
		}
	}
	return PlanSensitivityFromVariables(vars), loadErr
}

func collectSensitiveLeaves(v interface{}, add func(string)) {
	switch t := v.(type) {
	case string:
		add(t)
	case map[string]interface{}:
		for _, x := range t {
			collectSensitiveLeaves(x, add)
		}
	case []interface{}:
		for _, x := range t {
			collectSensitiveLeaves(x, add)
		}
	}
}

// scrubValues replaces every string leaf of v equal to / containing a
// sensitive value. Map keys are left alone (they are structure).
func (ps *PlanSensitivity) scrubValues(v interface{}) interface{} {
	switch t := v.(type) {
	case string:
		if ps.matches(t) {
			return SensitivePlaceholder
		}
		return t
	case map[string]interface{}:
		for k, x := range t {
			t[k] = ps.scrubValues(x)
		}
		return t
	case []interface{}:
		for i, x := range t {
			t[i] = ps.scrubValues(x)
		}
		return t
	default:
		return v
	}
}

func (ps *PlanSensitivity) matches(s string) bool {
	if s == SensitivePlaceholder {
		return false
	}
	for _, sv := range ps.Values {
		if s == sv {
			return true
		}
		if utf8.RuneCountInString(sv) >= minSubstringScrubLen && strings.Contains(s, sv) {
			return true
		}
	}
	return false
}

// RedactPlanJSON returns a deep copy of a `terraform show -json` plan with
// every sensitive value replaced by SensitivePlaceholder. This is the only
// form of a plan that may be stored (workspace_tasks.plan_json, resource
// changes, drift details, the agent upload, manifest_runs.plan_redacted) or
// handed to run tasks / the UI. Terraform marks sensitivity itself, so the
// rules are the plan format's own masks:
//   - resource_changes[] / resource_drift[]: change.before / change.after
//     masked by change.before_sensitive / change.after_sensitive;
//   - output_changes: same, per output;
//   - planned_values / prior_state: outputs with "sensitive": true, and every
//     resource's values masked by its sensitive_values (recursively through
//     child_modules);
//   - variables: the value of every variable declared sensitive in
//     configuration (root module) or sensitive on the platform (ps.Names);
//   - configuration: defaults of those variables, and every constant in
//     provider_config expressions (provider blocks carry credentials that
//     Terraform does not mark);
//   - everywhere: string leaves equal to / containing a platform-sensitive
//     value (ps.Values, see PlanSensitivity).
//
// ps is the platform-side sensitivity (nil = HCL markers only; callers with
// a task must pass it). The binary plan (plan_data, needed by apply) is not
// touched here. Redaction is idempotent.
func RedactPlanJSON(plan map[string]interface{}, ps *PlanSensitivity) map[string]interface{} {
	if plan == nil {
		return nil
	}
	out, _ := deepCopyJSON(plan).(map[string]interface{})
	if out == nil {
		return nil
	}

	for _, key := range []string{"resource_changes", "resource_drift"} {
		if rcs, ok := out[key].([]interface{}); ok {
			for _, rc := range rcs {
				if m, ok := rc.(map[string]interface{}); ok {
					redactChange(m["change"])
				}
			}
		}
	}
	if oc, ok := out["output_changes"].(map[string]interface{}); ok {
		for _, ch := range oc {
			redactChange(ch)
		}
	}
	for _, key := range []string{"planned_values", "prior_state"} {
		vals, _ := out[key].(map[string]interface{})
		if key == "prior_state" && vals != nil {
			vals, _ = vals["values"].(map[string]interface{})
		}
		redactStateValues(vals)
	}

	sensitiveVars := map[string]bool{}
	if cfg, ok := out["configuration"].(map[string]interface{}); ok {
		if root, ok := cfg["root_module"].(map[string]interface{}); ok {
			if vars, ok := root["variables"].(map[string]interface{}); ok {
				for name, v := range vars {
					if vm, ok := v.(map[string]interface{}); ok && (vm["sensitive"] == true || (ps != nil && ps.Names[name])) {
						sensitiveVars[name] = true
						if _, has := vm["default"]; has {
							vm["default"] = SensitivePlaceholder
						}
					}
				}
			}
		}
		if pcs, ok := cfg["provider_config"].(map[string]interface{}); ok {
			for _, pc := range pcs {
				if pm, ok := pc.(map[string]interface{}); ok {
					redactConstants(pm["expressions"])
				}
			}
		}
	}
	if vars, ok := out["variables"].(map[string]interface{}); ok {
		for name, v := range vars {
			if vm, ok := v.(map[string]interface{}); ok && (sensitiveVars[name] || (ps != nil && ps.Names[name])) {
				vm["value"] = SensitivePlaceholder
			}
		}
	}
	if ps != nil && len(ps.Values) > 0 {
		ps.scrubValues(out)
	}
	return out
}

// RedactedPlanHash the plan_hash of a redacted plan: SHA-256 (hex) over
// "terranova-plan-v1" 0x00 + its canonical JSON (encoding/json, which sorts
// map keys). Callers must hash the plan RedactPlanJSON returned — the value
// that is stored — never the raw plan.
func RedactedPlanHash(redacted map[string]interface{}) (string, error) {
	b, err := json.Marshal(redacted)
	if err != nil {
		return "", fmt.Errorf("encode redacted plan: %w", err)
	}
	h := sha256.New()
	h.Write([]byte("terranova-plan-v1"))
	h.Write([]byte{0})
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RedactPlanForStorage redacts plan and returns the redacted plan with its
// plan_hash (computed after redaction).
func RedactPlanForStorage(plan map[string]interface{}, ps *PlanSensitivity) (map[string]interface{}, string, error) {
	redacted := RedactPlanJSON(plan, ps)
	if redacted == nil {
		return nil, "", nil
	}
	h, err := RedactedPlanHash(redacted)
	return redacted, h, err
}

func redactChange(ch interface{}) {
	m, ok := ch.(map[string]interface{})
	if !ok {
		return
	}
	if v, ok := m["before"]; ok {
		m["before"] = applySensitiveMask(v, m["before_sensitive"])
	}
	if v, ok := m["after"]; ok {
		m["after"] = applySensitiveMask(v, m["after_sensitive"])
	}
}

// redactStateValues planned_values / prior_state.values: outputs and the
// resources of the root module and every child module.
func redactStateValues(vals map[string]interface{}) {
	if vals == nil {
		return
	}
	if outs, ok := vals["outputs"].(map[string]interface{}); ok {
		for _, o := range outs {
			if om, ok := o.(map[string]interface{}); ok && om["sensitive"] == true {
				if _, has := om["value"]; has {
					om["value"] = SensitivePlaceholder
				}
			}
		}
	}
	redactModule(vals["root_module"])
}

func redactModule(mod interface{}) {
	m, ok := mod.(map[string]interface{})
	if !ok {
		return
	}
	if rs, ok := m["resources"].([]interface{}); ok {
		for _, r := range rs {
			if rm, ok := r.(map[string]interface{}); ok {
				if v, has := rm["values"]; has {
					rm["values"] = applySensitiveMask(v, rm["sensitive_values"])
				}
			}
		}
	}
	if cms, ok := m["child_modules"].([]interface{}); ok {
		for _, cm := range cms {
			redactModule(cm)
		}
	}
}

// applySensitiveMask replaces the parts of v that mask marks sensitive. A
// mask is true (all of v), or an object / array mirroring v's shape.
func applySensitiveMask(v, mask interface{}) interface{} {
	switch mk := mask.(type) {
	case bool:
		if mk && v != nil {
			return SensitivePlaceholder
		}
		return v
	case map[string]interface{}:
		vm, ok := v.(map[string]interface{})
		if !ok {
			return v
		}
		for k, sub := range mk {
			if cur, has := vm[k]; has {
				vm[k] = applySensitiveMask(cur, sub)
			}
		}
		return vm
	case []interface{}:
		vl, ok := v.([]interface{})
		if !ok {
			return v
		}
		for i := range vl {
			if i < len(mk) {
				vl[i] = applySensitiveMask(vl[i], mk[i])
			}
		}
		return vl
	default:
		return v
	}
}

// redactConstants replaces every "constant_value" in a configuration
// expressions tree.
func redactConstants(expr interface{}) {
	switch e := expr.(type) {
	case map[string]interface{}:
		for k, v := range e {
			if k == "constant_value" {
				if v != nil {
					e[k] = SensitivePlaceholder
				}
				continue
			}
			redactConstants(v)
		}
	case []interface{}:
		for _, v := range e {
			redactConstants(v)
		}
	}
}

func deepCopyJSON(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, x := range t {
			out[k] = deepCopyJSON(x)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, x := range t {
			out[i] = deepCopyJSON(x)
		}
		return out
	default:
		return v
	}
}
