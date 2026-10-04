package manifestbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func f(path, content string) File { return File{Path: path, Content: []byte(content)} }

func pr(rule, file string) Problem {
	return Problem{File: file, Rule: rule, Message: ruleMessage(rule)}
}

func prl(rule, file string, line int) Problem {
	return Problem{File: file, Line: line, Rule: rule, Message: ruleMessage(rule)}
}

func TestValidate_Rules(t *testing.T) {
	big := strings.Repeat("a", MaxFileSize+1)
	cases := []struct {
		name  string
		files []File
		want  []Problem
	}{
		{"valid", []File{f("main.tf", "x"), f(".terraform.lock.hcl", "x"), f("modules/net/main.tf", "x"), f("docs/café.md", "x")}, nil},
		{"absolute", []File{f("/etc/passwd", "")}, []Problem{pr(RulePathInvalid, "/etc/passwd")}},
		{"dotdot", []File{f("a/../b.tf", "")}, []Problem{pr(RulePathInvalid, "a/../b.tf")}},
		{"backslash", []File{f(`a\b.tf`, "")}, []Problem{pr(RulePathInvalid, `a\b.tf`)}},
		{"control char", []File{f("a\nb.tf", "")}, []Problem{pr(RulePathInvalid, "a\nb.tf")}},
		{"too long", []File{f(strings.Repeat("a", MaxPathLen+1), "")}, []Problem{pr(RulePathTooLong, strings.Repeat("a", MaxPathLen+1))}},
		{"not nfc", []File{f("cafe\u0301.tf", "")}, []Problem{pr(RulePathNotNFC, "cafe\u0301.tf")}},
		{"duplicate", []File{f("a.tf", ""), f("a.tf", "")}, []Problem{pr(RulePathDuplicate, "a.tf")}},
		{"case duplicate", []File{f("Main.tf", ""), f("main.tf", "")}, []Problem{pr(RulePathCaseDuplicate, "Main.tf"), pr(RulePathCaseDuplicate, "main.tf")}},
		{"denylist", []File{f("prod.tfvars", ""), f("x.tfvars.json", ""), f("terraform.tfstate", ""), f("terraform.tfstate.backup", ""),
			f(".terraform/providers/p", ""), f("sub/.terraformrc", ""), f("terraform.rc", "")},
			[]Problem{pr(RuleDenylistedFile, ".terraform/providers/p"), pr(RuleDenylistedFile, "prod.tfvars"), pr(RuleDenylistedFile, "sub/.terraformrc"),
				pr(RuleDenylistedFile, "terraform.rc"), pr(RuleDenylistedFile, "terraform.tfstate"), pr(RuleDenylistedFile, "terraform.tfstate.backup"), pr(RuleDenylistedFile, "x.tfvars.json")}},
		{"git metadata", []File{f(".git/config", ""), f("modules/vendored/.git/HEAD", ""), f("sub/.git", ""), f("x/.GIT/config", ""), f(".gitignore", ""), f(".github/workflows/ci.yml", "")},
			[]Problem{pr(RuleDenylistedFile, ".git/config"), pr(RuleDenylistedFile, "modules/vendored/.git/HEAD"), pr(RuleDenylistedFile, "sub/.git"), pr(RuleDenylistedFile, "x/.GIT/config")}},
		{"dotenv", []File{f(".env", ""), f("app/.env.production", ""), f(".env.local", ""), f("y/.ENV", ""), f(".envrc", ""), f("env.tf", ""), f("x.env", "")},
			[]Problem{pr(RuleDenylistedFile, ".env"), pr(RuleDenylistedFile, ".env.local"), pr(RuleDenylistedFile, "app/.env.production"), pr(RuleDenylistedFile, "y/.ENV")}},
		{"private key files", []File{f("certs/server.pem", ""), f("tls.key", ""), f("id_rsa", ""), f("keys/id_dsa", ""), f("id_ecdsa", ""), f("id_ed25519", ""),
			f("store.p12", ""), f("store.pfx", ""), f("old/ID_RSA", ""), f("id_rsa.pub", ""), f("keys.tf", ""), f("monkey.tf", "")},
			[]Problem{pr(RuleDenylistedFile, "certs/server.pem"), pr(RuleDenylistedFile, "id_ecdsa"), pr(RuleDenylistedFile, "id_ed25519"),
				pr(RuleDenylistedFile, "id_rsa"), pr(RuleDenylistedFile, "keys/id_dsa"), pr(RuleDenylistedFile, "old/ID_RSA"), pr(RuleDenylistedFile, "store.p12"), pr(RuleDenylistedFile, "store.pfx"), pr(RuleDenylistedFile, "tls.key")}},
		{"file too large", []File{f("big.bin", big)}, []Problem{pr(RuleFileTooLarge, "big.bin")}},
		{"secrets", []File{
			f("aws.tf", "# creds\nprovider \"aws\" {\n  access_key = \"AKIAQWERTYUIOPASDFGH\"\n}\n"),
			f("key.txt", "-----BEGIN RSA PRIVATE KEY-----\nMIIB\n"),
			f("gh.tf", "token = \"ghp_"+strings.Repeat("A", 36)+"\""),
			f("slack.tf", "hook = \"xoxb-1234567890-abc\""),
		}, []Problem{prl(RuleSecretScanPrefix+"aws_access_key", "aws.tf", 3), prl(RuleSecretScanPrefix+"github_token", "gh.tf", 1),
			prl(RuleSecretScanPrefix+"private_key", "key.txt", 1), prl(RuleSecretScanPrefix+"slack_token", "slack.tf", 1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Validate(c.files); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Validate = %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidate_BundleTooLarge(t *testing.T) {
	var files []File
	chunk := bytes.Repeat([]byte("a"), MaxFileSize)
	for i := 0; i*MaxFileSize <= MaxBundleSize; i++ {
		files = append(files, File{Path: fmt.Sprintf("f%03d.bin", i), Content: chunk})
	}
	got := Validate(files)
	if len(got) != 1 || got[0] != pr(RuleBundleTooLarge, "") {
		t.Fatalf("Validate = %v", got)
	}
}

func TestReason_NeverContainsContent(t *testing.T) {
	secret := "AKIAQWERTYUIOPASDFGH"
	ps := Validate([]File{f("main.tf", `k = "`+secret+`"`), f("a.tfvars", `pw = "hunter2"`), f("x\ny.tf", "")})
	r := Reason(ps)
	if strings.Contains(r, secret) || strings.Contains(r, "hunter2") || strings.Contains(r, "\n") {
		t.Fatalf("reason leaks content or raw control chars: %q", r)
	}
	if !strings.Contains(r, "secret_scan:aws_access_key @ main.tf") || !strings.Contains(r, "denylisted_file @ a.tfvars") || !strings.Contains(r, `path_invalid @ "x\ny.tf"`) {
		t.Fatalf("reason = %q", r)
	}
	for _, p := range ps {
		if strings.Contains(p.String(), secret) {
			t.Fatal("Problem.String leaks content")
		}
	}
}

func TestReason_Capped(t *testing.T) {
	var ps []Problem
	for i := 0; i < 25; i++ {
		ps = append(ps, pr(RuleDenylistedFile, fmt.Sprintf("f%02d.tfvars", i)))
	}
	r := Reason(ps)
	if !strings.HasSuffix(r, "(+5 more)") || strings.Count(r, "; ") != 20 {
		t.Fatalf("reason = %q", r)
	}
	if Reason(nil) != "" {
		t.Fatal("empty reason expected")
	}
}

func TestProblem_JSONShapeAndMessages(t *testing.T) {
	secret := "AKIAQWERTYUIOPASDFGH"
	ps := Validate([]File{f("prod.tfvars", `pw = "hunter2"`), f("main.tf", "a\nb\nkey = \""+secret+"\"\n")})
	raw, _ := json.Marshal(ps)
	want := `[{"file":"main.tf","line":3,"rule":"secret_scan:aws_access_key","message":"possible AWS access key found; remove it and use a variable or secret store instead"},` +
		`{"file":"prod.tfvars","rule":"denylisted_file","message":"file type is not allowed in a bundle (variable values, state, git metadata, env or private key files)"}]`
	if string(raw) != want {
		t.Fatalf("problems JSON =\n%s\nwant\n%s", raw, want)
	}
	// every rule has a fixed message that never echoes content
	for _, rule := range []string{RulePathInvalid, RulePathTooLong, RulePathNotNFC, RulePathDuplicate, RulePathCaseDuplicate,
		RuleDenylistedFile, RuleFileTooLarge, RuleBundleTooLarge, RuleSecretScanPrefix + "aws_access_key", RuleSecretScanPrefix + "private_key",
		RuleSecretScanPrefix + "github_token", RuleSecretScanPrefix + "slack_token"} {
		if m := ruleMessage(rule); m == "" || m == rule || strings.Contains(m, secret) {
			t.Fatalf("message of %s = %q", rule, m)
		}
	}
	if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("hunter2")) {
		t.Fatal("problems leak content")
	}
}
