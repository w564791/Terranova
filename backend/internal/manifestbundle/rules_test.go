package manifestbundle

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func f(path, content string) File { return File{Path: path, Content: []byte(content)} }

func TestValidate_Rules(t *testing.T) {
	big := strings.Repeat("a", MaxFileSize+1)
	cases := []struct {
		name  string
		files []File
		want  []Problem
	}{
		{"valid", []File{f("main.tf", "x"), f(".terraform.lock.hcl", "x"), f("modules/net/main.tf", "x"), f("docs/café.md", "x")}, nil},
		{"absolute", []File{f("/etc/passwd", "")}, []Problem{{RulePathInvalid, "/etc/passwd"}}},
		{"dotdot", []File{f("a/../b.tf", "")}, []Problem{{RulePathInvalid, "a/../b.tf"}}},
		{"backslash", []File{f(`a\b.tf`, "")}, []Problem{{RulePathInvalid, `a\b.tf`}}},
		{"control char", []File{f("a\nb.tf", "")}, []Problem{{RulePathInvalid, "a\nb.tf"}}},
		{"too long", []File{f(strings.Repeat("a", MaxPathLen+1), "")}, []Problem{{RulePathTooLong, strings.Repeat("a", MaxPathLen+1)}}},
		{"not nfc", []File{f("cafe\u0301.tf", "")}, []Problem{{RulePathNotNFC, "cafe\u0301.tf"}}},
		{"duplicate", []File{f("a.tf", ""), f("a.tf", "")}, []Problem{{RulePathDuplicate, "a.tf"}}},
		{"case duplicate", []File{f("Main.tf", ""), f("main.tf", "")}, []Problem{{RulePathCaseDuplicate, "Main.tf"}, {RulePathCaseDuplicate, "main.tf"}}},
		{"denylist", []File{f("prod.tfvars", ""), f("x.tfvars.json", ""), f("terraform.tfstate", ""), f("terraform.tfstate.backup", ""),
			f(".terraform/providers/p", ""), f("sub/.terraformrc", ""), f("terraform.rc", "")},
			[]Problem{{RuleDenylistedFile, ".terraform/providers/p"}, {RuleDenylistedFile, "prod.tfvars"}, {RuleDenylistedFile, "sub/.terraformrc"},
				{RuleDenylistedFile, "terraform.rc"}, {RuleDenylistedFile, "terraform.tfstate"}, {RuleDenylistedFile, "terraform.tfstate.backup"}, {RuleDenylistedFile, "x.tfvars.json"}}},
		{"git metadata", []File{f(".git/config", ""), f("modules/vendored/.git/HEAD", ""), f("sub/.git", ""), f("x/.GIT/config", ""), f(".gitignore", ""), f(".github/workflows/ci.yml", "")},
			[]Problem{{RuleDenylistedFile, ".git/config"}, {RuleDenylistedFile, "modules/vendored/.git/HEAD"}, {RuleDenylistedFile, "sub/.git"}, {RuleDenylistedFile, "x/.GIT/config"}}},
		{"dotenv", []File{f(".env", ""), f("app/.env.production", ""), f(".env.local", ""), f("y/.ENV", ""), f(".envrc", ""), f("env.tf", ""), f("x.env", "")},
			[]Problem{{RuleDenylistedFile, ".env"}, {RuleDenylistedFile, ".env.local"}, {RuleDenylistedFile, "app/.env.production"}, {RuleDenylistedFile, "y/.ENV"}}},
		{"private key files", []File{f("certs/server.pem", ""), f("tls.key", ""), f("id_rsa", ""), f("keys/id_dsa", ""), f("id_ecdsa", ""), f("id_ed25519", ""),
			f("store.p12", ""), f("store.pfx", ""), f("old/ID_RSA", ""), f("id_rsa.pub", ""), f("keys.tf", ""), f("monkey.tf", "")},
			[]Problem{{RuleDenylistedFile, "certs/server.pem"}, {RuleDenylistedFile, "id_ecdsa"}, {RuleDenylistedFile, "id_ed25519"},
				{RuleDenylistedFile, "id_rsa"}, {RuleDenylistedFile, "keys/id_dsa"}, {RuleDenylistedFile, "old/ID_RSA"}, {RuleDenylistedFile, "store.p12"}, {RuleDenylistedFile, "store.pfx"}, {RuleDenylistedFile, "tls.key"}}},
		{"file too large", []File{f("big.bin", big)}, []Problem{{RuleFileTooLarge, "big.bin"}}},
		{"secrets", []File{
			f("aws.tf", `access_key = "AKIAQWERTYUIOPASDFGH"`),
			f("key.txt", "-----BEGIN RSA PRIVATE KEY-----\nMIIB\n"),
			f("gh.tf", "token = \"ghp_"+strings.Repeat("A", 36)+"\""),
			f("slack.tf", "hook = \"xoxb-1234567890-abc\""),
		}, []Problem{{RuleSecretScanPrefix + "aws_access_key", "aws.tf"}, {RuleSecretScanPrefix + "github_token", "gh.tf"},
			{RuleSecretScanPrefix + "private_key", "key.txt"}, {RuleSecretScanPrefix + "slack_token", "slack.tf"}}},
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
	if len(got) != 1 || got[0] != (Problem{Rule: RuleBundleTooLarge}) {
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
		ps = append(ps, Problem{Rule: RuleDenylistedFile, Path: fmt.Sprintf("f%02d.tfvars", i)})
	}
	r := Reason(ps)
	if !strings.HasSuffix(r, "(+5 more)") || strings.Count(r, "; ") != 20 {
		t.Fatalf("reason = %q", r)
	}
	if Reason(nil) != "" {
		t.Fatal("empty reason expected")
	}
}
