package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// SensitivePlaceholder replaces every sensitive value in a stored plan.
const SensitivePlaceholder = "(sensitive value)"

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
//     configuration (root module);
//   - configuration: defaults of sensitive variables, and every constant in
//     provider_config expressions (provider blocks carry credentials that
//     Terraform does not mark).
//
// The binary plan (plan_data, needed by apply) is not touched here.
// Redaction is idempotent.
func RedactPlanJSON(plan map[string]interface{}) map[string]interface{} {
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
					if vm, ok := v.(map[string]interface{}); ok && vm["sensitive"] == true {
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
			if vm, ok := v.(map[string]interface{}); ok && sensitiveVars[name] {
				vm["value"] = SensitivePlaceholder
			}
		}
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
func RedactPlanForStorage(plan map[string]interface{}) (map[string]interface{}, string, error) {
	redacted := RedactPlanJSON(plan)
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
