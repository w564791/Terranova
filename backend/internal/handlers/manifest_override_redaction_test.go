package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupOverrideDB: mfd-a on ws-a (mfv-1) with stored overrides. Version mfv-1
// declares variable "tf_secret" sensitive; varset vs-proj (attached) holds
// sensitive db_password; workspace ws-a has sensitive api_token.
func setupOverrideDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "test-jwt-secret-at-least-32-bytes-long!!")
	db := setupManifestIAMDB(t)
	for _, stmt := range []string{
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, variables TEXT, changelog TEXT, created_by TEXT, created_at DATETIME)`,
		`INSERT INTO manifest_versions (id, manifest_id, version, created_by) VALUES ('mfv-1', 'mf-1', 'v1.0.0', 'u1'), ('mfv-2', 'mf-1', 'v1.1.0', 'u1')`,
		`INSERT INTO manifest_files (manifest_id, version_id, path, content, mime, size, is_binary, mode) VALUES
		   ('mf-1', 'mfv-1', 'variables.tf', CAST('variable "tf_secret" {
  sensitive = true
}
variable "region" {
  default = "eu-west-1"
}
' AS BLOB), 'text/plain', 1, 0, 420),
		   ('mf-1', NULL, 'draft.tf', CAST('variable "draft_only" {
  sensitive = true
}
' AS BLOB), 'text/plain', 1, 0, 420)`,
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, manifest_subpath TEXT, manifest_active_tag TEXT, manifest_deployment_id TEXT, updated_at DATETIME)`,
		`INSERT INTO workspaces (id, workspace_id, manifest_deployment_id, manifest_active_tag) VALUES (1, 'ws-a', 'mfd-a', 'v1.0.0')`,
		`CREATE TABLE workspace_resources (id INTEGER PRIMARY KEY, workspace_id TEXT, resource_id TEXT, manifest_deployment_id TEXT)`,
		`ALTER TABLE manifest_deployment_varsets ADD COLUMN id INTEGER`,
		`ALTER TABLE manifest_deployment_varsets ADD COLUMN created_at DATETIME`,
		`INSERT INTO variable_sets (varset_id, name, scope) VALUES ('vs-proj', 'p', 'specific')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-proj', 'project', 10)`,
		`INSERT INTO manifest_deployment_varsets (deployment_id, varset_id, priority) VALUES ('mfd-a', 'vs-proj', 1)`,
		`INSERT INTO varset_variables (variable_id, varset_id, key, value, variable_type, value_format, sensitive, version) VALUES
		   ('v1', 'vs-proj', 'db_password', 'from-varset', 'terraform', 'string', 1, 1),
		   ('v2', 'vs-proj', 'plain_from_varset', 'p', 'terraform', 'string', 0, 1)`,
		`INSERT INTO workspace_variables (variable_id, workspace_id, key, version, value, variable_type, value_format, sensitive) VALUES
		   ('wv1', 'ws-a', 'api_token', 1, 'tok', 'terraform', 'string', 1)`,
		`UPDATE manifest_deployments SET variable_overrides = CAST('{"db_password":"stored-pw-SECRET","region":"eu-central-1"}' AS BLOB),
		   sensitive_keys = CAST('["db_password"]' AS BLOB) WHERE id = 'mfd-a'`,
		`UPDATE manifest_deployments SET variable_overrides = CAST('{"region":"us-east-1-VISIBLE"}' AS BLOB) WHERE id = 'mfd-b'`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	sealFixtureVersions(t, db)
	return db
}

func overrideRouter(db *gorm.DB, levels map[valueobject.ResourceType]valueobject.PermissionLevel) *gin.Engine {
	h := NewManifestDeploymentsV2Handler(db, middleware.NewIAMPermissionMiddlewareWithChecker(&resourceChecker{levels: levels}))
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	full := func(c *gin.Context) {
		c.Set(service.WorkspaceListAccessContextKey, &service.WorkspaceListAccess{FullOrganization: true})
		c.Next()
	}
	r.GET("/organizations/:org_id/manifests/:id/v2/deployments", withCaller(valueobject.PermissionLevelRead), full, h.ListDeployments)
	r.GET("/organizations/:org_id/manifests/:id/v2/deployments/:deployment_id", withCaller(valueobject.PermissionLevelRead), h.GetDeployment)
	r.POST("/organizations/:org_id/manifests/:id/v2/deployments/:deployment_id/upgrade", withCaller(valueobject.PermissionLevelRead), h.Upgrade)
	r.POST("/organizations/:org_id/manifests/:id/v2/deployments/:deployment_id/variable-preview", withCaller(valueobject.PermissionLevelRead), h.VariablePreview)
	return r
}

var readVars = map[valueobject.ResourceType]valueobject.PermissionLevel{
	valueobject.ResourceTypeAllWorkspaces:      valueobject.PermissionLevelRead,
	valueobject.ResourceTypeWorkspaceVars:      valueobject.PermissionLevelRead,
	valueobject.ResourceTypeWorkspaceResources: valueobject.PermissionLevelWrite,
}

// writeVars: upgrade with overrides also needs WORKSPACE_VARIABLES WRITE
var writeVars = map[valueobject.ResourceType]valueobject.PermissionLevel{
	valueobject.ResourceTypeAllWorkspaces:      valueobject.PermissionLevelRead,
	valueobject.ResourceTypeWorkspaceVars:      valueobject.PermissionLevelWrite,
	valueobject.ResourceTypeWorkspaceResources: valueobject.PermissionLevelWrite,
}

func overridesOf(t *testing.T, raw json.RawMessage) map[string]services.OverrideView {
	t.Helper()
	var d struct {
		Overrides []services.OverrideView `json:"overrides"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	out := map[string]services.OverrideView{}
	for _, o := range d.Overrides {
		out[o.Key] = o
	}
	return out
}

func TestDeploymentGetAndList_RedactSensitiveOverrides(t *testing.T) {
	r := overrideRouter(setupOverrideDB(t), readVars)
	get := doJSON(r, "GET", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a", "")
	list := doJSON(r, "GET", "/organizations/1/manifests/mf-1/v2/deployments", "")
	for name, w := range map[string]string{"get": get.Body.String(), "list": list.Body.String()} {
		if strings.Contains(w, "stored-pw-SECRET") || strings.Contains(w, "variable_overrides") || strings.Contains(w, "sensitive_keys") {
			t.Fatalf("%s leaks overrides: %s", name, w)
		}
	}
	if get.Code != http.StatusOK || list.Code != http.StatusOK {
		t.Fatalf("get %d list %d", get.Code, list.Code)
	}
	var gb struct {
		Deployment json.RawMessage `json:"deployment"`
	}
	_ = json.Unmarshal(get.Body.Bytes(), &gb)
	ov := overridesOf(t, gb.Deployment)
	if o := ov["db_password"]; !o.Sensitive || !o.HasValue || o.Value != nil {
		t.Fatalf("db_password = %+v, want sensitive, has_value, no value", o)
	}
	if o := ov["region"]; o.Sensitive || o.Value == nil || *o.Value != "eu-central-1" {
		t.Fatalf("region = %+v, want visible value", o)
	}
	// list: mfd-b has NULL sensitive_keys -> every key sensitive, no value
	var lb struct {
		Deployments []json.RawMessage `json:"deployments"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &lb)
	if len(lb.Deployments) != 2 {
		t.Fatalf("deployments: %s", list.Body.String())
	}
	if strings.Contains(list.Body.String(), "us-east-1-VISIBLE") {
		t.Fatalf("NULL sensitive_keys row must not return values: %s", list.Body.String())
	}
}

func TestDeploymentGet_NoValuesWithoutWorkspaceVariablesRead(t *testing.T) {
	r := overrideRouter(setupOverrideDB(t), map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeAllWorkspaces: valueobject.PermissionLevelRead,
	})
	for _, p := range []string{"/organizations/1/manifests/mf-1/v2/deployments/mfd-a", "/organizations/1/manifests/mf-1/v2/deployments"} {
		w := doJSON(r, "GET", p, "")
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", p, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), `"value"`) || strings.Contains(w.Body.String(), "eu-central-1") {
			t.Fatalf("%s: caller without WORKSPACE_VARIABLES READ must see no values: %s", p, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"has_value":true`) {
			t.Fatalf("%s: keys and has_value are still listed: %s", p, w.Body.String())
		}
	}
}

func TestRedactOverrides_NullSensitiveKeysMeansAllSensitive(t *testing.T) {
	views := services.RedactOverrides(map[string]string{"a": "1", "b": ""}, nil, true)
	for _, v := range views {
		if !v.Sensitive || v.Value != nil {
			t.Fatalf("NULL sensitive_keys must hide every value: %+v", views)
		}
	}
	if views[0].Key != "a" || !views[0].HasValue || views[1].HasValue {
		t.Fatalf("has_value: %+v", views)
	}
}

func sensitiveKeysOf(t *testing.T, db *gorm.DB, id string) (map[string]bool, bool) {
	t.Helper()
	var dep models.ManifestDeployment
	if err := db.First(&dep, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	return services.ParseSensitiveKeys(dep.SensitiveKeys)
}

// Once sensitive, always sensitive; request flags count on a computed row.
func TestUpgrade_SensitivityIsSticky(t *testing.T) {
	db := setupOverrideDB(t)
	db.Exec(`UPDATE manifest_deployments SET sensitive_keys = CAST('["db_password","manual"]' AS BLOB) WHERE id = 'mfd-a'`)
	r := overrideRouter(db, writeVars)
	w := doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/upgrade",
		`{"target_version_id":"mfv-2","variable_overrides":{"manual":"now-plain","flagged":{"value":"f","sensitive":true}}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("upgrade: %d %s", w.Code, w.Body.String())
	}
	keys, known := sensitiveKeysOf(t, db, "mfd-a")
	if !known || !keys["manual"] || !keys["db_password"] || !keys["flagged"] {
		t.Fatalf("sticky / flagged keys lost: %v", keys)
	}
	// computed from versions (tf_secret of the current version) + varsets/workspace
	if !keys["tf_secret"] || !keys["api_token"] {
		t.Fatalf("computed keys missing: %v", keys)
	}
	if keys["region"] || keys["plain_from_varset"] || keys["draft_only"] {
		t.Fatalf("over-marked: %v", keys)
	}
	// a later upgrade sending the key as plain does not un-mark it
	doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/upgrade",
		`{"target_version_id":"mfv-2","variable_overrides":{"flagged":{"value":"f2","sensitive":false}}}`)
	if keys, _ := sensitiveKeysOf(t, db, "mfd-a"); !keys["flagged"] || !keys["manual"] {
		t.Fatalf("sensitivity must be sticky: %v", keys)
	}
}

// On a row whose sensitive_keys is still NULL, request flags are ignored: the
// written set is computed from versions and varsets only.
func TestUpgrade_NullRowIgnoresRequestFlags(t *testing.T) {
	db := setupOverrideDB(t)
	db.Exec(`UPDATE manifest_deployments SET sensitive_keys = NULL WHERE id = 'mfd-a'`)
	r := overrideRouter(db, writeVars)
	w := doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/upgrade",
		`{"target_version_id":"mfv-2","variable_overrides":{"flagged":{"value":"f","sensitive":true}}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("upgrade: %d %s", w.Code, w.Body.String())
	}
	keys, known := sensitiveKeysOf(t, db, "mfd-a")
	if !known {
		t.Fatal("upgrade must write the computed set")
	}
	if keys["flagged"] {
		t.Fatalf("request flag must be ignored on a NULL row: %v", keys)
	}
	if !keys["db_password"] || !keys["tf_secret"] || keys["region"] {
		t.Fatalf("computed set wrong: %v", keys)
	}
}

// The per-deployment preview applies the stored overrides through the same
// merge as upgrade; sensitive ones stay blank.
func TestVariablePreview_MergesStoredOverrides(t *testing.T) {
	r := overrideRouter(setupOverrideDB(t), readVars)
	w := doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/variable-preview", `{"varsets":[{"varset_id":"vs-proj","priority":1}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Variables []struct {
			Key        string `json:"key"`
			Value      string `json:"value"`
			Sensitive  bool   `json:"sensitive"`
			SourceType string `json:"source_type"`
		} `json:"variables"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	got := map[string]int{}
	for i, v := range body.Variables {
		got[v.Key] = i
	}
	region, ok := got["region"]
	if !ok || body.Variables[region].Value != "eu-central-1" || body.Variables[region].SourceType != "override" {
		t.Fatalf("stored region override not applied: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "stored-pw-SECRET") {
		t.Fatalf("sensitive stored override leaked in preview: %s", w.Body.String())
	}
	// unset_keys removes a stored override from the preview, as upgrade would
	w = doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/variable-preview", `{"unset_keys":["region"]}`)
	if strings.Contains(w.Body.String(), "eu-central-1") {
		t.Fatalf("unset_keys not applied in preview: %s", w.Body.String())
	}
}

func TestBackfillDeploymentSensitiveKeys_IdempotentNoOverMarking(t *testing.T) {
	db := setupOverrideDB(t)
	db.Exec(`UPDATE manifest_deployments SET sensitive_keys = NULL WHERE id = 'mfd-a'`)
	db.Exec(`UPDATE manifest_deployments SET sensitive_keys = CAST('["kept"]' AS BLOB) WHERE id = 'mfd-b'`)
	n, err := services.BackfillDeploymentSensitiveKeys(context.Background(), db)
	if err != nil || n != 1 {
		t.Fatalf("first run: n=%d err=%v", n, err)
	}
	keys, known := sensitiveKeysOf(t, db, "mfd-a")
	if !known || !keys["db_password"] || !keys["tf_secret"] || !keys["api_token"] {
		t.Fatalf("backfill missed sensitive keys: %v", keys)
	}
	if keys["region"] || keys["plain_from_varset"] || keys["draft_only"] || len(keys) != 3 {
		t.Fatalf("backfill over-marked: %v", keys)
	}
	if b, _ := sensitiveKeysOf(t, db, "mfd-b"); len(b) != 1 || !b["kept"] {
		t.Fatalf("non-NULL row must be left alone: %v", b)
	}
	if n, err := services.BackfillDeploymentSensitiveKeys(context.Background(), db); err != nil || n != 0 {
		t.Fatalf("second run must be a no-op: n=%d err=%v", n, err)
	}
}

func TestOverrideInputs_AcceptStringOrObject(t *testing.T) {
	var in models.OverrideInputs
	if err := json.Unmarshal([]byte(`{"a":"1","b":{"value":"2","sensitive":true}}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.Values()["a"] != "1" || in.Values()["b"] != "2" || !in.SensitiveFlags()["b"] || in.SensitiveFlags()["a"] {
		t.Fatalf("parsed %+v", in)
	}
	if err := json.Unmarshal([]byte(`{"a":1}`), &in); err == nil {
		t.Fatal("non-string scalar must be rejected")
	}
}

// A DB failure inside a manifest handler surfaces as the generic 500 from
// ErrorHandler, without driver text.
func TestManifestHandler500_NoDBDetailsInBody(t *testing.T) {
	db := setupOverrideDB(t)
	if err := db.Exec(`DROP TABLE manifest_deployments`).Error; err != nil {
		t.Fatal(err)
	}
	r := overrideRouter(db, readVars)
	w := doJSON(r, "GET", "/organizations/1/manifests/mf-1/v2/deployments", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "no such table") || strings.Contains(body, "manifest_deployments") || strings.Contains(body, "SQL") {
		t.Fatalf("500 body leaks DB details: %s", body)
	}
	if !strings.Contains(body, `"error":"internal error"`) || !strings.Contains(body, `"request_id":"`) {
		t.Fatalf("500 body: %s", body)
	}
}
