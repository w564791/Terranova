package manifestbundle

import (
	"errors"
	"strings"
	"testing"
)

func hclFiles(kv ...string) []File {
	var out []File
	for i := 0; i+1 < len(kv); i += 2 {
		out = append(out, File{Path: kv[i], Content: []byte(kv[i+1])})
	}
	return out
}

func mustCheck(t *testing.T, files []File, policy ModuleSourcePolicy) []Problem {
	t.Helper()
	probs, err := CheckHCL(files, policy)
	if err != nil {
		t.Fatal(err)
	}
	return probs
}

func hasProblem(probs []Problem, file, rule string, line int) bool {
	for _, p := range probs {
		if p.File == file && p.Rule == rule && p.Line == line {
			return true
		}
	}
	return false
}

func TestCheckHCL_Provisioners(t *testing.T) {
	probs := mustCheck(t, hclFiles(
		"main.tf", `resource "null_resource" "a" {
  provisioner "local-exec" {
    command = "id"
  }
}

resource "terraform_data" "b" {
  provisioner "remote-exec" {
    when   = destroy
    inline = ["id"]
  }
  provisioner "file" {
    source      = "a"
    destination = "/tmp/a"
  }
}

removed {
  from = null_resource.c
  provisioner "local-exec" {
    when    = destroy
    command = "id"
  }
}
`), nil)
	for _, line := range []int{2, 8, 12, 20} {
		if !hasProblem(probs, "main.tf", RuleHCLProvisioner, line) {
			t.Fatalf("missing provisioner at line %d: %+v", line, probs)
		}
	}
	if len(probs) != 4 {
		t.Fatalf("want 4 problems, got %+v", probs)
	}
	for _, p := range probs {
		if strings.Contains(p.Message, "id") && strings.Contains(p.Message, "command") {
			t.Fatalf("message leaks content: %q", p.Message)
		}
	}
}

func TestCheckHCL_JSONLocalExecWithLine(t *testing.T) {
	probs := mustCheck(t, hclFiles(
		"modules/x/exec_override.tf.json", `{
  "resource": {
    "null_resource": {
      "x": {
        "provisioner": {
          "local-exec": {"command": "id"}
        }
      }
    }
  }
}`), nil)
	if len(probs) != 1 || probs[0].Rule != RuleHCLProvisioner || probs[0].File != "modules/x/exec_override.tf.json" || probs[0].Line < 5 {
		t.Fatalf("json local-exec not caught with line: %+v", probs)
	}
	// array form
	probs = mustCheck(t, hclFiles("a.tf.json", `{"resource":{"null_resource":{"x":{"provisioner":[{"local-exec":{"command":"id"}}]}}}}`), nil)
	if len(probs) != 1 || probs[0].Rule != RuleHCLProvisioner || probs[0].Line != 1 {
		t.Fatalf("json array local-exec: %+v", probs)
	}
}

func TestCheckHCL_DataSources(t *testing.T) {
	probs := mustCheck(t, hclFiles(
		"d.tf", `data "external" "x" {
  program = ["sh"]
}
check "c" {
  data "http" "h" {
    url = "https://example.com"
  }
}
data "aws_caller_identity" "me" {}
terraform {
  required_providers {
    shell = {
      source = "registry.terraform.io/hashicorp/external"
    }
    web = {
      source = "hashicorp/http"
    }
    aws = {
      source = "hashicorp/aws"
    }
  }
}
`, "j.tf.json", `{"data":{"http":{"x":{"url":"https://example.com"}}}}`), nil)
	want := []struct {
		file, rule string
		line       int
	}{
		{"d.tf", RuleHCLExternalData, 1},
		{"d.tf", RuleHCLHTTPData, 5},
		{"d.tf", RuleHCLExternalData, 12},
		{"d.tf", RuleHCLHTTPData, 15},
		{"j.tf.json", RuleHCLHTTPData, 1},
	}
	for _, w := range want {
		if !hasProblem(probs, w.file, w.rule, w.line) {
			t.Fatalf("missing %+v in %+v", w, probs)
		}
	}
	if len(probs) != len(want) {
		t.Fatalf("unexpected problems: %+v", probs)
	}
}

func TestCheckHCL_ModuleSources(t *testing.T) {
	files := hclFiles(
		"envs/prod/main.tf", `module "net" {
  source = "../../modules/net"
}
module "self" {
  source = "./sub"
}
module "escape" {
  source = "../../../outside"
}
module "registry" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
}
module "registry_sub" {
  source = "terraform-aws-modules/vpc/aws//modules/vpc-endpoints"
}
module "git" {
  source = "git::https://github.com/acme/mods.git//net?ref=main"
}
module "dynamic" {
  source = "${var.x}/mod"
}
module "nosrc" {
}
`)
	probs := mustCheck(t, files, nil) // local only
	for _, line := range []int{8, 11, 15, 21, 23} {
		if !hasProblem(probs, "envs/prod/main.tf", RuleHCLModuleSource, line) {
			t.Fatalf("local-only: missing module_source at %d: %+v", line, probs)
		}
	}
	// a git source on a branch is unpinned whatever the policy
	if !hasProblem(probs, "envs/prod/main.tf", RuleHCLModuleUnpinned, 18) {
		t.Fatalf("local-only: missing module_unpinned at 18: %+v", probs)
	}
	if len(probs) != 6 {
		t.Fatalf("local-only: %+v", probs)
	}
	probs = mustCheck(t, files, ModuleSourceAllowlist([]string{"terraform-aws-modules/vpc/aws"}))
	if hasProblem(probs, "envs/prod/main.tf", RuleHCLModuleSource, 11) || hasProblem(probs, "envs/prod/main.tf", RuleHCLModuleSource, 15) {
		t.Fatalf("allowlisted registry module flagged: %+v", probs)
	}
	if len(probs) != 4 {
		t.Fatalf("allowlist: %+v", probs)
	}
	probs = mustCheck(t, files, ModuleSourceAllowlist([]string{"git::https://github.com/acme/mods.git"}))
	if hasProblem(probs, "envs/prod/main.tf", RuleHCLModuleSource, 18) || !hasProblem(probs, "envs/prod/main.tf", RuleHCLModuleUnpinned, 18) {
		t.Fatalf("allowlisted unpinned git subdir: %+v", probs)
	}
	boom := errors.New("db down")
	if _, err := CheckHCL(files, func(string) (bool, error) { return false, boom }); !errors.Is(err, boom) {
		t.Fatalf("policy error must abort: %v", err)
	}
}

func TestCheckHCL_ParseErrorsAreHits(t *testing.T) {
	probs := mustCheck(t, hclFiles(
		"ok.tf", `resource "null_resource" "a" {}`,
		"bad.tf", "resource \"null_resource\" \"a\" {\n  x = \n}\n",
		"bad.tf.json", `{"resource": `,
		"override.tf", `resource "null_resource" {}`, // wrong label count
		"README.md", `not { hcl`,
		"x.tofu", `}`,
	), nil)
	for _, f := range []string{"bad.tf", "bad.tf.json", "override.tf", "x.tofu"} {
		found := false
		for _, p := range probs {
			if p.File == f && p.Rule == RuleHCLParseError && p.Line >= 1 {
				found = true
			}
		}
		if !found {
			t.Fatalf("parse error in %s not reported: %+v", f, probs)
		}
	}
	if len(probs) != 4 {
		t.Fatalf("only terraform files are parsed: %+v", probs)
	}
	if !hasProblem(probs, "bad.tf", RuleHCLParseError, 3) && !hasProblem(probs, "bad.tf", RuleHCLParseError, 2) {
		t.Fatalf("parse error line: %+v", probs)
	}
}

func TestIsTerraformConfigFile(t *testing.T) {
	for p, want := range map[string]bool{
		"main.tf": true, "a/b/x.tf.json": true, "override.tf": true, "x_override.tf.json": true,
		"MAIN.TF": true, "m.tofu": true, "m.tofu.json": true,
		".terraform.lock.hcl": false, "x.tfvars": false, "x.tf.bak": false, "README.md": false,
	} {
		if got := IsTerraformConfigFile(p); got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
}

func TestValidateForPublish_MergesRules(t *testing.T) {
	probs, err := ValidateForPublish(hclFiles(
		"main.tf", "resource \"null_resource\" \"a\" {\n  provisioner \"local-exec\" { command = \"id\" }\n}\n",
		"prod.tfvars", "x = 1",
	), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(probs) != 2 || probs[0].File != "main.tf" || probs[0].Rule != RuleHCLProvisioner || probs[0].Line != 2 ||
		probs[1].Rule != RuleDenylistedFile {
		t.Fatalf("%+v", probs)
	}
	// Validate (migrations / stored versions) is unchanged: no HCL rules
	if p := Validate(hclFiles("main.tf", "resource \"null_resource\" \"a\" {\n  provisioner \"local-exec\" { command = \"id\" }\n}\n")); p != nil {
		t.Fatalf("Validate must not run HCL checks: %+v", p)
	}
	b, probs, err := PackFilesForPublish(hclFiles("main.tf", `module "m" { source = "./m" }`, "m/main.tf", `resource "null_resource" "a" {}`), nil)
	if err != nil || len(probs) != 0 || b == nil || b.Hash == "" {
		t.Fatalf("clean publish: %v %+v %v", b, probs, err)
	}
}

func TestCheckHCL_GitModuleSourcesMustPinSHA(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	src := func(s string) []File {
		return hclFiles("main.tf", "module \"m\" {\n  source = \""+s+"\"\n}\n")
	}
	allow := ModuleSourceAllowlist([]string{"git::https://github.com/acme/mods.git"})
	for _, tc := range []struct {
		source string
		rule   string // "" = allowed
	}{
		{"git::https://github.com/acme/mods.git//net?ref=" + sha, ""},
		{"git::https://github.com/acme/mods.git//net?ref=" + sha + "&depth=1", ""},
		{"git::https://github.com/acme/mods.git//net", RuleHCLModuleUnpinned},
		{"git::https://github.com/acme/mods.git//net?ref=v1.2.0", RuleHCLModuleUnpinned},
		{"git::https://github.com/acme/mods.git//net?ref=main", RuleHCLModuleUnpinned},
		{"git::https://github.com/acme/mods.git//net?ref=" + sha[:12], RuleHCLModuleUnpinned},
		{"git::https://github.com/acme/mods.git//net?ref=" + strings.ToUpper(sha), RuleHCLModuleUnpinned},
		{"git::https://github.com/acme/mods.git//net?ref=" + sha + "&sshkey=abc", RuleHCLModuleUnpinned},
		{"github.com/acme/mods?ref=main", RuleHCLModuleUnpinned},
		{"git@github.com:acme/mods.git", RuleHCLModuleUnpinned},
		{"bitbucket.org/acme/mods", RuleHCLModuleUnpinned},
		{"git::https://github.com/other/mods.git//net?ref=" + sha, RuleHCLModuleSource}, // pinned, not allowlisted
	} {
		probs := mustCheck(t, src(tc.source), allow)
		if tc.rule == "" && len(probs) != 0 {
			t.Fatalf("%s: %+v", tc.source, probs)
		}
		if tc.rule != "" && !hasProblem(probs, "main.tf", tc.rule, 2) {
			t.Fatalf("%s: want %s, got %+v", tc.source, tc.rule, probs)
		}
	}
}
