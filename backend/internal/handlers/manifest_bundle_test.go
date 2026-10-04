package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// bundleEnv: setupOverrideDB (mfd-a on ws-a at mfv-1) + an empty workspace
// ws-new and routes for publish / versions / export / deploy.
type bundleEnv struct {
	db *gorm.DB
	r  *gin.Engine
}

func newBundleEnv(t *testing.T) *bundleEnv {
	t.Helper()
	db := setupOverrideDB(t)
	for _, stmt := range []string{
		`INSERT INTO workspaces (id, workspace_id) VALUES (2, 'ws-new')`,
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`INSERT INTO projects (id, org_id) VALUES (10, 1)`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES ('ws-new', 10)`,
		`ALTER TABLE manifest_versions ADD COLUMN source_ref TEXT`,
		`ALTER TABLE workspaces ADD COLUMN tf_state TEXT`,
		`ALTER TABLE workspace_resources ADD COLUMN resource_type TEXT`,
		`ALTER TABLE workspace_resources ADD COLUMN resource_name TEXT`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	perm := middleware.NewIAMPermissionMiddlewareWithChecker(&resourceChecker{levels: writeVars})
	vh := NewManifestVersionsHandler(db)
	dh := NewManifestDeploymentsV2Handler(db, perm)
	mh := NewManifestHandler(db, perm)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	g := r.Group("/organizations/:org_id/manifests/:id", withCaller(valueobject.PermissionLevelWrite))
	g.POST("/v2/versions", vh.PublishVersion)
	g.GET("/v2/versions", vh.ListVersions)
	g.GET("/v2/versions/:version_id", vh.GetVersion)
	g.GET("/v2/versions/:version_id/workdirs", vh.ListWorkdirs)
	g.POST("/v2/versions/:version_id/files/_export", vh.ExportVersion)
	g.GET("/v2/draft/diff", vh.DiffDraft)
	g.GET("/export-zip", mh.ExportManifestZip)
	g.POST("/v2/deployments/install", dh.Install)
	g.POST("/v2/deployments/variable-preview", dh.FirstInstallVariablePreview)
	g.POST("/v2/deployments/:deployment_id/upgrade", dh.Upgrade)
	g.POST("/v2/deployments/:deployment_id/uninstall", dh.Uninstall)
	g.POST("/v2/deployments/:deployment_id/variable-preview", dh.VariablePreview)
	return &bundleEnv{db: db, r: r}
}

const base = "/organizations/1/manifests/mf-1"

func (e *bundleEnv) putDraft(t *testing.T, owner, path, content string) {
	t.Helper()
	if err := e.db.Exec(`DELETE FROM manifest_files WHERE manifest_id = 'mf-1' AND version_id IS NULL AND owner_user_id = ? AND path = ?`, owner, path).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, owner_user_id, path, content, mime, size, is_binary, mode)
		VALUES ('mf-1', NULL, ?, ?, ?, 'text/x-terraform', ?, 0, 420)`, owner, path, []byte(content), len(content)).Error; err != nil {
		t.Fatal(err)
	}
}

func (e *bundleEnv) publish(t *testing.T, version string) (id, hash string) {
	t.Helper()
	w := doJSON(e.r, "POST", base+"/v2/versions", `{"version":"`+version+`"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("publish %s: %d %s", version, w.Code, w.Body.String())
	}
	var resp struct {
		ID         string `json:"id"`
		BundleHash string `json:"bundle_hash"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.ID, resp.BundleHash
}

const pubMain = `resource "null_resource" "published" {}
variable "region" {
  default = "eu-west-1"
}
`

func TestPublish_DeterministicBundleHash(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "modules/net/main.tf", `resource "null_resource" "net" {}`)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id1, h1 := e.publish(t, "v2.0.0")

	want, _ := manifestbundle.Hash([]manifestbundle.File{
		{Path: "main.tf", Content: []byte(pubMain)},
		{Path: "modules/net/main.tf", Content: []byte(`resource "null_resource" "net" {}`)},
	})
	if h1 != want {
		t.Fatalf("publish hash %s, want manifestbundle.Hash %s", h1, want)
	}
	// same content published again (rows written in another order) -> same hash
	e.db.Exec(`DELETE FROM manifest_files WHERE version_id IS NULL`)
	e.putDraft(t, "u1", "main.tf", pubMain)
	e.putDraft(t, "u1", "modules/net/main.tf", `resource "null_resource" "net" {}`)
	id2, h2 := e.publish(t, "v2.0.1")
	if id1 == id2 || h1 != h2 {
		t.Fatalf("same files must give the same hash: %s vs %s", h1, h2)
	}
	stored, _ := manifestbundle.VersionHash(context.Background(), e.db, id2)
	if stored != h2 {
		t.Fatalf("stored snapshot hashes to %s, version says %s", stored, h2)
	}
	// content-addressed read
	b, err := manifestbundle.OpenBundle(context.Background(), e.db, h1)
	if err != nil || b.Hash != h1 || len(b.Files) != 2 {
		t.Fatalf("OpenBundle: %+v %v", b, err)
	}
	// list and detail expose bundle_hash / bundle_invalid_reason
	lw := doJSON(e.r, "GET", base+"/v2/versions", "")
	gw := doJSON(e.r, "GET", base+"/v2/versions/"+id1, "")
	for name, body := range map[string]string{"list": lw.Body.String(), "detail": gw.Body.String()} {
		if !strings.Contains(body, `"bundle_hash":"`+h1+`"`) || !strings.Contains(body, `"bundle_invalid_reason":null`) {
			t.Fatalf("%s lacks bundle fields: %s", name, body)
		}
	}
}

func TestPublish_RejectsRuleViolationsWithProblemsOnly(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", "provider \"aws\" {\n  access_key = \"AKIAQWERTYUIOPASDFGH\"\n}\n")
	e.putDraft(t, "u1", "prod.tfvars", `password = "hunter2"`)
	var before int64
	e.db.Table("manifest_versions").Count(&before)
	w := doJSON(e.r, "POST", base+"/v2/versions", `{"version":"v9.0.0"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`{"rule":"denylisted_file","path":"prod.tfvars"}`, `{"rule":"secret_scan:aws_access_key","path":"main.tf"}`, `"code":"bundle_rules_violated"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("422 body lacks %s: %s", want, body)
		}
	}
	for _, leak := range []string{"AKIA", "hunter2", "access_key ="} {
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

func TestBundle_ImmutableAfterPublish(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, hash := e.publish(t, "v2.0.0")

	// editing the draft afterwards does not touch the version
	e.putDraft(t, "u1", "main.tf", `resource "null_resource" "draft_only" {}`)
	b, err := manifestbundle.OpenVersion(context.Background(), e.db, "mf-1", id)
	if err != nil || b.Hash != hash || string(b.Scope()["main.tf"]) != pubMain {
		t.Fatalf("version changed with the draft: %+v %v", b, err)
	}

	// tampering with the stored snapshot is detected by every reader
	e.db.Exec(`UPDATE manifest_files SET content = CAST('resource "null_resource" "evil" {}' AS BLOB) WHERE version_id = ?`, id)
	if _, err := manifestbundle.OpenVersion(context.Background(), e.db, "mf-1", id); !errors.Is(err, manifestbundle.ErrIntegrity) {
		t.Fatalf("tampered bundle must fail integrity, got %v", err)
	}
	w := doJSON(e.r, "POST", base+"/v2/versions/"+id+"/files/_export", "")
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "evil") {
		t.Fatalf("export of tampered bundle: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(e.r, "POST", base+"/v2/deployments/install", `{"version_id":"`+id+`","workspace_id":"ws-new"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("install of tampered bundle: %d %s", w.Code, w.Body.String())
	}
}

func readZip(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestDownstreamReadsBundleNotDraft(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, _ := e.publish(t, "v2.0.0")

	// the draft moves on: new content, a draft-only workdir and a sensitive variable
	e.putDraft(t, "u1", "main.tf", `resource "null_resource" "draft_only" {}
variable "region" {
  sensitive = true
}
`)
	e.putDraft(t, "u1", "draftdir/main.tf", `resource "null_resource" "x" {}`)

	// export (latest version, no version_id) and version export
	for _, req := range []struct{ method, path string }{{"GET", base + "/export-zip"}, {"POST", base + "/v2/versions/" + id + "/files/_export"}} {
		w := doJSON(e.r, req.method, req.path, "")
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", req.path, w.Code, w.Body.String())
		}
		files := readZip(t, w.Body.Bytes())
		if files["main.tf"] != pubMain || len(files) != 1 {
			t.Fatalf("%s exported the draft: %v", req.path, files)
		}
	}
	// workdirs
	w := doJSON(e.r, "GET", base+"/v2/versions/"+id+"/workdirs", "")
	if strings.Contains(w.Body.String(), "draftdir") {
		t.Fatalf("workdirs read the draft: %s", w.Body.String())
	}
	// install: workdir / resource parsing use the version bundle
	w = doJSON(e.r, "POST", base+"/v2/deployments/install", `{"version_id":"`+id+`","workspace_id":"ws-new","workdir":"draftdir"}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "not found in version") {
		t.Fatalf("install must not see the draft-only workdir: %d %s", w.Code, w.Body.String())
	}
	b, err := manifestbundle.OpenVersion(context.Background(), e.db, "mf-1", id)
	if err != nil {
		t.Fatal(err)
	}
	refs := shallowParseBundleResources(b, nil)
	if len(refs) != 1 || refs[0].Name != "published" {
		t.Fatalf("install resources: %+v", refs)
	}
	// sensitivity is computed from the version (region is not sensitive there)
	keys, err := services.ComputeDeploymentSensitiveKeys(e.db, []string{id}, "ws-new", nil)
	if err != nil || keys["region"] {
		t.Fatalf("sensitivity read the draft: %v %v", keys, err)
	}
	// executor file source (local data accessor)
	e.db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id, status) VALUES ('mfd-pub', 'mf-1', ?, 'ws-new', 'active')`, id)
	files, err := services.NewLocalDataAccessor(e.db).GetManifestFilesByTag("mfd-pub", "v2.0.0")
	if err != nil || len(files) != 1 || string(files[0].Content) != pubMain {
		t.Fatalf("executor files: %v %v", files, err)
	}
	// draft diff compares the draft against the version bundle
	w = doJSON(e.r, "GET", base+"/v2/draft/diff?against="+id, "")
	if !strings.Contains(w.Body.String(), `{"path":"main.tf","state":"changed"}`) || !strings.Contains(w.Body.String(), `{"path":"draftdir/main.tf","state":"added"}`) {
		t.Fatalf("draft diff: %s", w.Body.String())
	}
}

func markInvalid(t *testing.T, db *gorm.DB, versionID string) {
	t.Helper()
	if err := db.Exec(`UPDATE manifest_versions SET bundle_hash = NULL, bundle_invalid_reason = 'denylisted_file @ prod.tfvars' WHERE id = ?`, versionID).Error; err != nil {
		t.Fatal(err)
	}
}

func TestNullBundleHash_DeployPathsRequireRepublish(t *testing.T) {
	e := newBundleEnv(t)
	markInvalid(t, e.db, "mfv-1") // mfd-a's current version

	check409 := func(name string, w interface {
		Result() *http.Response
	}, body string, code int) {
		t.Helper()
		if code != http.StatusConflict || !strings.Contains(body, `"code":"bundle_republish_required"`) ||
			!strings.Contains(body, "please republish") || !strings.Contains(body, `"reason":"denylisted_file @ prod.tfvars"`) {
			t.Fatalf("%s: want 409 republish, got %d %s", name, code, body)
		}
	}
	w := doJSON(e.r, "POST", base+"/v2/deployments/install", `{"version_id":"mfv-1","workspace_id":"ws-new"}`)
	check409("install", w, w.Body.String(), w.Code)
	w = doJSON(e.r, "POST", base+"/v2/deployments/variable-preview", `{"version_id":"mfv-1","workspace_id":"ws-new"}`)
	check409("first-install preview", w, w.Body.String(), w.Code)
	w = doJSON(e.r, "POST", base+"/v2/deployments/mfd-a/variable-preview", `{}`)
	check409("deployment preview (current version)", w, w.Body.String(), w.Code)

	// preview / upgrade towards a valid target from the invalid current version are allowed
	w = doJSON(e.r, "POST", base+"/v2/deployments/mfd-a/variable-preview", `{"target_version_id":"mfv-2"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("preview of a valid target: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(e.r, "POST", base+"/v2/deployments/mfd-a/upgrade", `{"target_version_id":"mfv-2","varsets":[{"varset_id":"vs-proj","priority":1}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("upgrade from a NULL-hash version to a valid one: %d %s", w.Code, w.Body.String())
	}
	// mfd-a is now on the valid mfv-2; upgrading (back) to the invalid mfv-1 is rejected
	w = doJSON(e.r, "POST", base+"/v2/deployments/mfd-a/upgrade", `{"target_version_id":"mfv-1","varsets":[{"varset_id":"vs-proj","priority":1}]}`)
	check409("upgrade to NULL-hash target", w, w.Body.String(), w.Code)
	w = doJSON(e.r, "POST", base+"/v2/deployments/mfd-a/variable-preview", `{"target_version_id":"mfv-1"}`)
	check409("preview of NULL-hash target", w, w.Body.String(), w.Code)
}

func TestNullBundleHash_UninstallNotBlocked(t *testing.T) {
	e := newBundleEnv(t)
	markInvalid(t, e.db, "mfv-1")
	w := doJSON(e.r, "POST", base+"/v2/deployments/mfd-a/uninstall", "")
	if w.Code != http.StatusOK {
		t.Fatalf("uninstall of a deployment on a NULL-hash version must work: %d %s", w.Code, w.Body.String())
	}
}
