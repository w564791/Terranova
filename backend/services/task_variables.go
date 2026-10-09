package services

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"iac-platform/internal/models"
)

// VariableOverrides is the manifest deployment override snapshot of one task
// (workspace_tasks.variable_overrides + sensitive_keys, frozen at task
// creation). Local mode reads it from the task row; agent / K8s mode receives
// the same two columns through the agent task-data channel (GetTaskData
// task.variable_overrides / task.override_sensitive_keys) — never through
// environment variables or logs.
type VariableOverrides struct {
	Values map[string]string
	// SensitiveKeys raw sensitive_keys (jsonb string array). NULL / unparsable
	// means "not computed yet": every override key is treated as sensitive,
	// the same rule as RedactOverrides.
	SensitiveKeys json.RawMessage
}

// TaskVariableOverrides the override snapshot of task (zero value when none).
func TaskVariableOverrides(task *models.WorkspaceTask) VariableOverrides {
	if task == nil || len(task.VariableOverrides) == 0 {
		return VariableOverrides{}
	}
	return VariableOverrides{
		Values:        FlattenOverrides(task.VariableOverrides),
		SensitiveKeys: task.SensitiveKeys,
	}
}

// IsSensitive reports whether override key k must be declared sensitive.
func (o VariableOverrides) IsSensitive(k string) bool {
	sens, known := ParseSensitiveKeys(o.SensitiveKeys)
	return !known || sens[k]
}

// ApplyVariableOverrides overlays o on resolved variables of varType (the
// shared rule of LocalDataAccessor and RemoteDataAccessor). Only Terraform
// variables are overridden: an existing key gets the override value, a
// missing key is appended. A variable stays sensitive when it was sensitive
// before or when the override key is sensitive per sensitive_keys (an
// override never downgrades a sensitive variable). vars is not modified.
func ApplyVariableOverrides(vars []models.WorkspaceVariable, varType models.VariableType, o VariableOverrides) []models.WorkspaceVariable {
	if len(o.Values) == 0 || varType != models.VariableTypeTerraform {
		return vars
	}
	out := make([]models.WorkspaceVariable, len(vars))
	copy(out, vars)
	idx := make(map[string]int, len(out))
	for i, v := range out {
		idx[v.Key] = i
	}
	keys := make([]string, 0, len(o.Values))
	for k := range o.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := o.Values[k]
		if i, ok := idx[k]; ok {
			out[i].Value = val // ValueFormat kept: an override of an HCL variable is HCL
			out[i].Sensitive = out[i].Sensitive || o.IsSensitive(k)
			out[i].VariableID = "override-" + k
			continue
		}
		out = append(out, models.WorkspaceVariable{
			VariableID:   "override-" + k,
			Key:          k,
			Value:        val,
			VariableType: models.VariableTypeTerraform,
			Sensitive:    o.IsSensitive(k),
		})
	}
	return out
}

// terraformVariables the Terraform-type variables of vars sorted by key
// (stable), the single ordering of every generated variable file.
func terraformVariables(vars []models.WorkspaceVariable) []models.WorkspaceVariable {
	out := make([]models.WorkspaceVariable, 0, len(vars))
	for _, v := range vars {
		if v.VariableType != "" && v.VariableType != models.VariableTypeTerraform {
			continue
		}
		if v.Key == "" {
			continue
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// RenderTFVars is the one variables.tfvars generator of every runner (local
// plan, agent / K8s plan, apply-from-snapshot, and the step-6 sandbox): same
// input variables => byte-identical output. Only Terraform variables are
// written, sorted by key. String escaping is unchanged from the previous
// per-path generators (only '"' and newlines are escaped).
func RenderTFVars(vars []models.WorkspaceVariable) string {
	var b strings.Builder
	for _, v := range terraformVariables(vars) {
		if v.ValueFormat == models.ValueFormatHCL {
			trimmed := strings.TrimSpace(v.Value)
			needsQuotes := !strings.HasPrefix(trimmed, "{") &&
				!strings.HasPrefix(trimmed, "[") &&
				trimmed != "true" &&
				trimmed != "false" &&
				!isNumeric(trimmed)
			if !needsQuotes {
				fmt.Fprintf(&b, "%s = %s\n", v.Key, v.Value)
				continue
			}
		}
		fmt.Fprintf(&b, "%s = \"%s\"\n", v.Key, escapeTFVarString(v.Value))
	}
	return b.String()
}

func escapeTFVarString(s string) string {
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return strings.ReplaceAll(s, "\n", "\\n")
}

// VariablesTFJSON the variables.tf.json document declaring vars (Terraform
// variables only; nil when there are none).
func VariablesTFJSON(vars []models.WorkspaceVariable) map[string]interface{} {
	tv := terraformVariables(vars)
	if len(tv) == 0 {
		return nil
	}
	decl := make(map[string]interface{}, len(tv))
	for _, v := range tv {
		d := map[string]interface{}{"type": "string"}
		if v.Description != "" {
			d["description"] = v.Description
		}
		if v.Sensitive {
			d["sensitive"] = true
		}
		decl[v.Key] = d
	}
	return map[string]interface{}{"variable": decl}
}
