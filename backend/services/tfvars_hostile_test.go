package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"golang.org/x/text/unicode/norm"

	"iac-platform/internal/models"
)

// hostileStringValues values that broke out of / were interpreted by the old
// HCL tfvars writer. Each must reach Terraform as exactly this string.
var hostileStringValues = map[string]string{
	"backslash_quote": `x\" ` + "\n" + `other = "y`,
	"trailing_bslash": `ends with backslash \`,
	"template_interp": `${file("/etc/passwd")}`,
	"template_direct": `%{ if true }yes%{ endif }`,
	"escaped_interp":  `$${literal} and %%{literal}`,
	"newlines":        "line1\nline2\r\nline3\n",
	"control_chars":   "nul\x00 bell\x07 tab\t esc\x1b del\x7f ls\u2028 ps\u2029",
	"unicode":         "héllo 世界 🚀 \u200b zero-width, rtl \u202e",
	"json_breakout":   `"}, "injected": "x`,
	"heredoc_marker":  "<<EOT\nhi\nEOT",
	"comment_like":    "# not a comment // nor /* this */",
	"empty":           "",
}

func hostileVars() []models.WorkspaceVariable {
	var vars []models.WorkspaceVariable
	for k, v := range hostileStringValues {
		vars = append(vars, models.WorkspaceVariable{Key: k, Value: v, VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatString})
	}
	// HCL-format values that are strings by the historical heuristic
	vars = append(vars,
		models.WorkspaceVariable{Key: "hcl_func_string", Value: `file("/etc/passwd")`, VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatHCL},
		models.WorkspaceVariable{Key: "hcl_interp_string", Value: `${file("/etc/passwd")}`, VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatHCL},
	)
	return vars
}

// decodeLikeTerraform loads a JSON variables file the way Terraform does
// (hcl JSON body, attributes evaluated with a nil context).
func decodeLikeTerraform(t *testing.T, src []byte) map[string]cty.Value {
	t.Helper()
	f, diags := hcljson.Parse(src, TFVarsFileName)
	if diags.HasErrors() {
		t.Fatalf("parse: %s", diags)
	}
	attrs, diags := f.Body.JustAttributes()
	if diags.HasErrors() {
		t.Fatalf("attributes: %s", diags)
	}
	out := map[string]cty.Value{}
	for name, a := range attrs {
		v, diags := a.Expr.Value(nil)
		if diags.HasErrors() {
			t.Fatalf("%s: %s", name, diags)
		}
		out[name] = v
	}
	return out
}

func TestRenderTFVars_HostileStringsRoundTrip(t *testing.T) {
	vars := hostileVars()
	src, err := RenderTFVars(vars)
	if err != nil {
		t.Fatal(err)
	}
	got := decodeLikeTerraform(t, src)
	if len(got) != len(vars) {
		t.Fatalf("got %d attributes, want %d (injection?): %s", len(got), len(vars), src)
	}
	for _, v := range vars {
		g := got[v.Key]
		if !g.Type().Equals(cty.String) || g.AsString() != norm.NFC.String(v.Value) {
			t.Errorf("%s: got %#v want %q", v.Key, g, v.Value)
		}
	}
	// deterministic
	again, _ := RenderTFVars(hostileVars())
	if string(again) != string(src) {
		t.Fatal("render not deterministic")
	}
}

func TestRenderTFVars_HCLValuesBecomeJSONValues(t *testing.T) {
	vars := []models.WorkspaceVariable{
		{Key: "obj", Value: `{ team = "infra", n = 3, ok = true, list = [1, "a"], nested = { x = null } }`, ValueFormat: models.ValueFormatHCL},
		{Key: "list", Value: `["a", "${"b"}", "c\"d"]`, ValueFormat: models.ValueFormatHCL},
		{Key: "num", Value: "12345678901234567890.5", ValueFormat: models.ValueFormatHCL},
		{Key: "flag", Value: "true", ValueFormat: models.ValueFormatHCL},
		{Key: "hostile_in_obj", Value: `{ s = "$${file(\"/etc/passwd\")}" }`, ValueFormat: models.ValueFormatHCL},
	}
	src, err := RenderTFVars(vars)
	if err != nil {
		t.Fatal(err)
	}
	got := decodeLikeTerraform(t, src)
	for _, v := range vars {
		expr, _ := hclParseValue(v.Value)
		want, _ := expr.Value(nil)
		g := got[v.Key]
		// the JSON form loses only the HCL object-vs-map / tuple distinction
		// that Terraform's type conversion ignores; compare as JSON
		wj, _ := ctyjson.Marshal(want, want.Type())
		gj, _ := ctyjson.Marshal(g, g.Type())
		if string(wj) != string(gj) {
			t.Errorf("%s: got %s want %s", v.Key, gj, wj)
		}
	}
	if !strings.Contains(string(src), `"s": "${file(\"/etc/passwd\")}"`) {
		t.Fatalf("escaped template must stay a literal string: %s", src)
	}
	decl := VariablesTFJSON(append(vars, models.WorkspaceVariable{Key: "str", Value: "x"}))
	types := map[string]string{}
	for k, d := range decl["variable"].(map[string]interface{}) {
		types[k] = d.(map[string]interface{})["type"].(string)
	}
	if types["obj"] != "any" || types["list"] != "any" || types["num"] != "string" || types["flag"] != "string" || types["str"] != "string" {
		t.Fatalf("declared types: %v", types)
	}
}

func hclParseValue(s string) (interface {
	Value(*hcl.EvalContext) (cty.Value, hcl.Diagnostics)
}, error) {
	expr, diags := hclsyntaxParse(s)
	if diags.HasErrors() {
		return nil, diags
	}
	return expr, nil
}

func TestRenderTFVars_RejectsUnrepresentable(t *testing.T) {
	cases := map[string]models.WorkspaceVariable{
		"second assignment": {Key: "tags", Value: "{ a = 1 }\nother = \"y\"", ValueFormat: models.ValueFormatHCL},
		"function call":     {Key: "tags", Value: `{ a = file("/etc/passwd") }`, ValueFormat: models.ValueFormatHCL},
		"template function": {Key: "tags", Value: `["${upper("x")}"]`, ValueFormat: models.ValueFormatHCL},
		"variable ref":      {Key: "tags", Value: `[var.secret]`, ValueFormat: models.ValueFormatHCL},
		"bad number":        {Key: "n", Value: "NaN", ValueFormat: models.ValueFormatHCL},
		"bad key":           {Key: "a\" = 1\nb", Value: "x"},
		"invalid utf8":      {Key: "s", Value: "bad \xff byte"},
	}
	for name, v := range cases {
		v.VariableType = models.VariableTypeTerraform
		v.Sensitive = true
		if name != "bad number" {
			v.Value += "SECRETMARK"
		}
		_, err := RenderTFVars([]models.WorkspaceVariable{v})
		if !errors.Is(err, ErrInvalidTFVar) {
			t.Errorf("%s: err = %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), "SECRETMARK") {
			t.Errorf("%s: error leaks the value: %v", name, err)
		}
	}
	dup := []models.WorkspaceVariable{{Key: "a", Value: "1"}, {Key: "a", Value: "2"}}
	if _, err := RenderTFVars(dup); !errors.Is(err, ErrInvalidTFVar) {
		t.Fatalf("duplicate: %v", err)
	}
}

// Real Terraform reads every hostile value back exactly (TERRANOVA_TEST_TERRAFORM).
func TestRenderTFVars_RealTerraformRoundTrip(t *testing.T) {
	tf := os.Getenv("TERRANOVA_TEST_TERRAFORM")
	if tf == "" {
		t.Skip("TERRANOVA_TEST_TERRAFORM not set")
	}
	vars := hostileVars()
	vars = append(vars,
		models.WorkspaceVariable{Key: "obj", Value: `{ team = "infra", s = "$${x}", n = [1, 2] }`, VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatHCL},
		models.WorkspaceVariable{Key: "num", Value: "3", VariableType: models.VariableTypeTerraform, ValueFormat: models.ValueFormatHCL},
		models.WorkspaceVariable{Key: "secret", Value: `s3"cr\et`, VariableType: models.VariableTypeTerraform, Sensitive: true},
	)
	dir := t.TempDir()
	tfvars, err := RenderTFVars(vars)
	if err != nil {
		t.Fatal(err)
	}
	decl, _ := json.Marshal(VariablesTFJSON(vars))
	outputs := map[string]interface{}{}
	for _, v := range vars {
		outputs[v.Key] = map[string]interface{}{"value": "${var." + v.Key + "}", "sensitive": v.Sensitive}
	}
	main, _ := json.Marshal(map[string]interface{}{"output": outputs})
	for name, b := range map[string][]byte{TFVarsFileName: tfvars, "variables.tf.json": decl, "main.tf.json": main} {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) []byte {
		cmd := exec.Command(tf, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1", "CHECKPOINT_DISABLE=1")
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
	run("init", "-input=false", "-no-color")
	run("apply", "-auto-approve", "-input=false", "-no-color")
	var got map[string]struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(run("output", "-json"), &got); err != nil {
		t.Fatal(err)
	}
	for _, v := range vars {
		o, ok := got[v.Key]
		if !ok {
			t.Errorf("%s: missing output", v.Key)
			continue
		}
		switch v.Key {
		case "obj":
			var buf bytes.Buffer
			_ = json.Compact(&buf, o.Value)
			if buf.String() != `{"n":[1,2],"s":"${x}","team":"infra"}` {
				t.Errorf("obj: %s", o.Value)
			}
		case "num":
			if string(o.Value) != `"3"` {
				t.Errorf("num: %s", o.Value)
			}
		default:
			var s string
			if err := json.Unmarshal(o.Value, &s); err != nil || s != norm.NFC.String(v.Value) {
				t.Errorf("%s: terraform read %s, want %q", v.Key, o.Value, v.Value)
			}
		}
	}
	if len(got) != len(vars) {
		t.Fatalf("outputs %d != vars %d", len(got), len(vars))
	}
}

func hclsyntaxParse(s string) (hclsyntax.Expression, hcl.Diagnostics) {
	return hclsyntax.ParseExpression([]byte(s), "test", hcl.InitialPos)
}
