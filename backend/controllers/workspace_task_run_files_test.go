package controllers

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iac-platform/internal/manifestbundle"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const runFilesVersionMain = `resource "null_resource" "a" {}` + "\n"

func setupRunFilesRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	h, _ := manifestbundle.Hash([]manifestbundle.File{{Path: "main.tf", Content: []byte(runFilesVersionMain), Mode: 0o644}})
	for _, stmt := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT, lock_id TEXT, provider_config TEXT)`,
		`CREATE TABLE workspace_tasks (id INTEGER PRIMARY KEY, workspace_id TEXT)`,
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, bundle_hash TEXT, bundle_invalid_reason TEXT)`,
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary BOOLEAN, mode INTEGER)`,
		`CREATE TABLE modules (id INTEGER PRIMARY KEY, module_source TEXT, status TEXT)`,
		`CREATE TABLE module_versions (id TEXT PRIMARY KEY, module_id INTEGER, module_source TEXT)`,
		`INSERT INTO workspaces (id, workspace_id, name) VALUES (1, 'ws-a', 'A')`,
		`INSERT INTO manifest_versions (id, manifest_id, bundle_hash) VALUES ('mfv-ok', 'mf-1', '` + h + `')`,
		`INSERT INTO manifest_versions (id, manifest_id, bundle_hash, bundle_invalid_reason) VALUES ('mfv-null', 'mf-1', NULL, 'denylisted_file @ prod.tfvars')`,
		`INSERT INTO manifest_files (manifest_id, version_id, path, content, mime, size, is_binary, mode) VALUES
		   ('mf-1', 'mfv-ok', 'main.tf', CAST('` + strings.TrimSuffix(runFilesVersionMain, "\n") + "\n" + `' AS BLOB), 'text/plain', 32, 0, 420)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	ctrl := &WorkspaceTaskController{db: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/workspaces/:id/tasks/plan", func(c *gin.Context) { c.Set("user_id", "u-1"); c.Next() }, ctrl.CreatePlanTask)
	return r, db
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func postPlan(r http.Handler, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/workspaces/ws-a/tasks/plan", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// Manifest [Run] files are validated before any task is created: a draft
// must pass the publish rules, files that claim to be a published version
// must be exactly that version (verified, non-NULL bundle_hash).
func TestCreatePlanTask_RunFilesValidated(t *testing.T) {
	r, db := setupRunFilesRouter(t)
	cases := []struct {
		name, body string
		code       int
		want       []string
	}{
		{"draft-provisioner", `{"run_type":"plan","external_files":[{"path":"main.tf","content_b64":"` +
			b64("resource \"null_resource\" \"a\" {\n  provisioner \"local-exec\" {\n    command = \"id\"\n  }\n}\n") + `"}]}`,
			422, []string{`"code":"bundle_rules_violated"`, `"rule":"hcl_provisioner"`, `"file":"main.tf"`}},
		{"draft-external-data", `{"run_type":"plan","external_files":[{"path":"d.tf","content_b64":"` +
			b64("data \"external\" \"x\" {\n  program = [\"sh\"]\n}\n") + `"}]}`,
			422, []string{`"rule":"hcl_external_data"`}},
		{"draft-denylisted", `{"run_type":"plan","external_files":[{"path":"prod.tfvars","content_b64":"` + b64("a = 1") + `"}]}`,
			422, []string{`"rule":"denylisted_file"`}},
		{"draft-path-traversal", `{"run_type":"plan","external_files":[{"path":"../x.tf","content_b64":"` + b64("") + `"}]}`,
			422, []string{`"rule":"path_invalid"`}},
		{"bad-base64", `{"run_type":"plan","external_files":[{"path":"main.tf","content_b64":"%%%"}]}`,
			400, []string{"not valid base64"}},
		{"version-mismatch", `{"run_type":"plan","manifest_version_id":"mfv-ok","external_files":[{"path":"main.tf","content_b64":"` +
			b64(`resource "null_resource" "evil" {}`+"\n") + `"}]}`,
			409, []string{`"code":"bundle_hash_mismatch"`}},
		{"version-null-hash", `{"run_type":"plan","manifest_version_id":"mfv-null","external_files":[{"path":"main.tf","content_b64":"` +
			b64(runFilesVersionMain) + `"}]}`,
			409, []string{`"code":"bundle_republish_required"`, `"reason":"denylisted_file @ prod.tfvars"`}},
		{"version-unknown", `{"run_type":"plan","manifest_version_id":"mfv-nope","external_files":[{"path":"main.tf","content_b64":"` +
			b64(runFilesVersionMain) + `"}]}`, 404, nil},
		{"version-without-files", `{"run_type":"plan","manifest_version_id":"mfv-ok"}`, 400, []string{"requires external_files"}},
		{"plan-and-apply", `{"run_type":"plan_and_apply","external_files":[{"path":"main.tf","content_b64":"` + b64(runFilesVersionMain) + `"}]}`,
			400, []string{"only supported for plan tasks"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postPlan(r, tc.body)
			if w.Code != tc.code {
				t.Fatalf("want %d, got %d %s", tc.code, w.Code, w.Body.String())
			}
			for _, s := range tc.want {
				if !strings.Contains(w.Body.String(), s) {
					t.Errorf("body lacks %s: %s", s, w.Body.String())
				}
			}
			if strings.Contains(w.Body.String(), "command") || strings.Contains(w.Body.String(), "evil") {
				t.Errorf("file content echoed: %s", w.Body.String())
			}
		})
	}
	var n int64
	db.Table("workspace_tasks").Count(&n)
	if n != 0 {
		t.Fatalf("%d tasks created from rejected Run files", n)
	}
}
