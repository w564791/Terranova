package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	ctyjson "github.com/zclconf/go-cty/cty/json"

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

// TFVarsFileName the variable values file every runner writes into the run
// directory. Terraform loads *.auto.tfvars.json automatically; the bundle
// rules reject user-supplied *.tfvars / *.tfvars.json, so it is the only
// variable file in the root module.
const TFVarsFileName = "terranova.auto.tfvars.json"

// TFVarsSensitiveMask the value written for sensitive variables by
// RenderTFVarsMasked (logs only).
const TFVarsSensitiveMask = "***SENSITIVE***"

// ErrInvalidTFVar a variable that cannot be handed to Terraform: the key is
// not an HCL identifier or is duplicated, the value is not valid UTF-8, or an
// HCL-format value is not one literal HCL expression. Messages name the key
// only, never the value (it may be sensitive).
var ErrInvalidTFVar = errors.New("invalid terraform variable")

// RenderTFVars is the one variable-values generator of every runner (local
// plan, agent / K8s plan, apply-from-snapshot, and the step-6 sandbox): same
// input variables => byte-identical output. It renders TFVarsFileName, a JSON
// object of the Terraform variables (keys sorted, encoding/json).
//
// Terraform evaluates a JSON variables file without an evaluation context, so
// a JSON string is always the literal string: "${...}" / "%{...}" are not
// templates, and JSON escaping makes '\', '"', newlines and control
// characters unable to end the value. Values by format:
//   - string format, and HCL format values that are not an object / list /
//     bool / number (the historical heuristic): a JSON string;
//   - HCL format objects / lists / bools / numbers: parsed as one standalone
//     HCL expression (hclsyntax) and evaluated with a nil context — exactly
//     how Terraform evaluates a .tfvars attribute, so no variables and no
//     functions — then converted to the equivalent JSON value. Anything else
//     (a second assignment after the value, references, function calls) is
//     rejected with ErrInvalidTFVar instead of reaching Terraform.
func RenderTFVars(vars []models.WorkspaceVariable) ([]byte, error) {
	return renderTFVars(vars, false)
}

// RenderTFVarsMasked RenderTFVars with every sensitive value replaced by
// TFVarsSensitiveMask, for logs (masks values, not rendered text).
func RenderTFVarsMasked(vars []models.WorkspaceVariable) ([]byte, error) {
	return renderTFVars(vars, true)
}

func renderTFVars(vars []models.WorkspaceVariable, mask bool) ([]byte, error) {
	doc := make(map[string]json.RawMessage)
	for _, v := range terraformVariables(vars) {
		if !hclsyntax.ValidIdentifier(v.Key) {
			return nil, fmt.Errorf("%w: variable name %q is not a valid identifier", ErrInvalidTFVar, v.Key)
		}
		if _, dup := doc[v.Key]; dup {
			return nil, fmt.Errorf("%w: variable %s is defined more than once", ErrInvalidTFVar, v.Key)
		}
		raw, err := tfvarJSONValue(v)
		if err != nil {
			return nil, err
		}
		if mask && v.Sensitive {
			raw, _ = json.Marshal(TFVarsSensitiveMask)
		}
		doc[v.Key] = raw
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// tfvarJSONValue the JSON value Terraform must see for v.
func tfvarJSONValue(v models.WorkspaceVariable) (json.RawMessage, error) {
	if !utf8.ValidString(v.Value) {
		return nil, fmt.Errorf("%w: variable %s: value is not valid UTF-8", ErrInvalidTFVar, v.Key)
	}
	if !isHCLTypedValue(v) {
		return json.Marshal(v.Value)
	}
	expr, diags := hclsyntax.ParseExpression([]byte(v.Value), "value of "+v.Key, hcl.InitialPos)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w: variable %s: value is not a single HCL expression (%s)", ErrInvalidTFVar, v.Key, firstDiag(diags))
	}
	val, diags := expr.Value(nil) // no variables, no functions: same as a .tfvars file
	if diags.HasErrors() {
		return nil, fmt.Errorf("%w: variable %s: value must be a literal (%s)", ErrInvalidTFVar, v.Key, firstDiag(diags))
	}
	if !val.IsWhollyKnown() {
		return nil, fmt.Errorf("%w: variable %s: value must be a literal", ErrInvalidTFVar, v.Key)
	}
	raw, err := ctyjson.Marshal(val, val.Type())
	if err != nil {
		return nil, fmt.Errorf("%w: variable %s: value cannot be represented as JSON", ErrInvalidTFVar, v.Key)
	}
	return raw, nil
}

// isHCLTypedValue the historical rule for HCL-format values: objects, lists,
// bools and numbers are HCL; any other HCL-format value is a plain string.
func isHCLTypedValue(v models.WorkspaceVariable) bool {
	if v.ValueFormat != models.ValueFormatHCL {
		return false
	}
	trimmed := strings.TrimSpace(v.Value)
	return strings.HasPrefix(trimmed, "{") ||
		strings.HasPrefix(trimmed, "[") ||
		trimmed == "true" ||
		trimmed == "false" ||
		isNumeric(trimmed)
}

// isComplexHCLValue an HCL-format object / list value: declared `any` (a
// string declaration cannot hold it).
func isComplexHCLValue(v models.WorkspaceVariable) bool {
	if v.ValueFormat != models.ValueFormatHCL {
		return false
	}
	trimmed := strings.TrimSpace(v.Value)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// firstDiag summary + position of the first error (details / snippets may
// quote the value, so they are left out).
func firstDiag(diags hcl.Diagnostics) string {
	for _, d := range diags {
		if d.Severity != hcl.DiagError {
			continue
		}
		if d.Subject != nil {
			return fmt.Sprintf("%s at line %d, column %d", d.Summary, d.Subject.Start.Line, d.Subject.Start.Column)
		}
		return d.Summary
	}
	return "invalid"
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
		// string unless the value is an HCL object / list (a string
		// declaration cannot hold it; bools / numbers keep converting to
		// string as before)
		typ := "string"
		if isComplexHCLValue(v) {
			typ = "any"
		}
		d := map[string]interface{}{"type": typ}
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
