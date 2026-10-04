package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
)

func setupFirstInstallPreview(t *testing.T, levels map[valueobject.ResourceType]valueobject.PermissionLevel) (*gin.Engine, *resourceChecker) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "test-jwt-secret-at-least-32-bytes-long!!") // variable decryption key
	db := setupManifestIAMDB(t)
	for _, stmt := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT)`,
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`INSERT INTO workspaces (id, workspace_id) VALUES (1, 'ws-a'), (2, 'ws-foreign')`,
		`INSERT INTO projects (id, org_id) VALUES (10, 1), (20, 2)`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES ('ws-foreign', 20)`,
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, variables TEXT, changelog TEXT, created_by TEXT, created_at DATETIME)`,
		`INSERT INTO manifest_versions (id, manifest_id, version, created_by) VALUES
		   ('mfv-1', 'mf-1', 'v1.0.0', 'u1'), ('mfv-draft', 'mf-1', 'draft', 'u1'), ('mfv-other', 'mf-other', 'v1.0.0', 'u1')`,
		`INSERT INTO variable_sets (varset_id, name, scope) VALUES ('vs-proj', 'p', 'specific'), ('vs-foreign', 'f', 'specific')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-proj', 'project', 10)`,
		`INSERT INTO varset_variables (variable_id, varset_id, key, value, variable_type, value_format, sensitive, version) VALUES
		   ('v1', 'vs-proj', 'region', 'us-east-1', 'terraform', 'string', 0, 1),
		   ('v2', 'vs-proj', 'db_password', 'hunter2', 'terraform', 'string', 1, 1)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	checker := &resourceChecker{levels: levels}
	h := NewManifestDeploymentsV2Handler(db, middleware.NewIAMPermissionMiddlewareWithChecker(checker))
	r := gin.New()
	r.POST("/organizations/:org_id/manifests/:id/v2/deployments/variable-preview", withCaller(valueobject.PermissionLevelRead), h.FirstInstallVariablePreview)
	return r, checker
}

const firstInstallPreviewPath = "/organizations/1/manifests/mf-1/v2/deployments/variable-preview"

var canReadWorkspaceVars = map[valueobject.ResourceType]valueobject.PermissionLevel{
	valueobject.ResourceTypeWorkspaceVars: valueobject.PermissionLevelRead,
}

func TestFirstInstallPreview_ReturnsMaskedVariables(t *testing.T) {
	r, checker := setupFirstInstallPreview(t, canReadWorkspaceVars)
	w := doJSON(r, "POST", firstInstallPreviewPath,
		`{"workspace_id":"ws-a","version_id":"mfv-1","varsets":[{"varset_id":"vs-proj","priority":1}],"variable_overrides":{"db_password":"typed","extra":"x"}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var body struct {
		Variables []struct {
			Key       string `json:"key"`
			Value     string `json:"value"`
			Sensitive bool   `json:"sensitive"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, v := range body.Variables {
		got[v.Key] = v.Value
		if v.Key == "db_password" && (!v.Sensitive || v.Value != "") {
			t.Fatalf("sensitive value must be blank: %+v", v)
		}
	}
	if got["region"] != "us-east-1" || got["extra"] != "x" {
		t.Fatalf("unexpected variables %v", got)
	}
	last := checker.calls[len(checker.calls)-1]
	if last.ResourceType != valueobject.ResourceTypeWorkspaceVars || last.ScopeType != valueobject.ScopeTypeWorkspace || last.ScopeIDStr != "ws-a" {
		t.Fatalf("must check WORKSPACE_VARIABLES READ on the target workspace, got %+v", last)
	}
}

// MANIFESTS READ alone (route-level, simulated by withCaller) is not enough.
func TestFirstInstallPreview_RequiresWorkspaceVariablesRead(t *testing.T) {
	r, _ := setupFirstInstallPreview(t, map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeManifests:          valueobject.PermissionLevelAdmin,
		valueobject.ResourceTypeWorkspaceResources: valueobject.PermissionLevelWrite,
	})
	if w := doJSON(r, "POST", firstInstallPreviewPath, `{"workspace_id":"ws-a","version_id":"mfv-1"}`); w.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403 (%s)", w.Code, w.Body.String())
	}
}

func TestFirstInstallPreview_TargetValidationIs404(t *testing.T) {
	r, checker := setupFirstInstallPreview(t, canReadWorkspaceVars)
	for name, body := range map[string]string{
		"workspace from another org":  `{"workspace_id":"ws-foreign","version_id":"mfv-1"}`,
		"unknown workspace":           `{"workspace_id":"ws-nope","version_id":"mfv-1"}`,
		"draft version":               `{"workspace_id":"ws-a","version_id":"mfv-draft"}`,
		"version of another manifest": `{"workspace_id":"ws-a","version_id":"mfv-other"}`,
		"unknown version":             `{"workspace_id":"ws-a","version_id":"mfv-nope"}`,
	} {
		checker.calls = nil
		if w := doJSON(r, "POST", firstInstallPreviewPath, body); w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d want 404 (%s)", name, w.Code, w.Body.String())
		}
		if len(checker.calls) != 0 {
			t.Errorf("%s: target must be validated before any workspace permission evaluation", name)
		}
	}
}

func TestFirstInstallPreview_RejectsUnmountableVarset(t *testing.T) {
	r, _ := setupFirstInstallPreview(t, canReadWorkspaceVars)
	w := doJSON(r, "POST", firstInstallPreviewPath, `{"workspace_id":"ws-a","version_id":"mfv-1","varsets":[{"varset_id":"vs-foreign"}]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d want 400 (%s)", w.Code, w.Body.String())
	}
}

func TestFirstInstallPreview_RequiresBody(t *testing.T) {
	r, _ := setupFirstInstallPreview(t, canReadWorkspaceVars)
	if w := doJSON(r, "POST", firstInstallPreviewPath, `{"workspace_id":"ws-a"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("missing version_id: got %d want 400", w.Code)
	}
}

// Install shares resolveInstallTarget: a workspace from another org or a
// draft/foreign version is 404 there as well.
func TestInstall_SharesTargetValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupManifestIAMDB(t)
	for _, stmt := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT)`,
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`INSERT INTO workspaces (id, workspace_id) VALUES (1, 'ws-a'), (2, 'ws-foreign')`,
		`INSERT INTO projects (id, org_id) VALUES (10, 1), (20, 2)`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES ('ws-foreign', 20)`,
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, variables TEXT, changelog TEXT, created_by TEXT, created_at DATETIME)`,
		`INSERT INTO manifest_versions (id, manifest_id, version, created_by) VALUES ('mfv-1', 'mf-1', 'v1.0.0', 'u1'), ('mfv-draft', 'mf-1', 'draft', 'u1')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	checker := &resourceChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeWorkspaceResources: valueobject.PermissionLevelWrite,
	}}
	h := NewManifestDeploymentsV2Handler(db, middleware.NewIAMPermissionMiddlewareWithChecker(checker))
	ir := gin.New()
	ir.POST("/organizations/:org_id/manifests/:id/v2/deployments/install", withCaller(valueobject.PermissionLevelRead), h.Install)
	for name, body := range map[string]string{
		"workspace from another org": `{"workspace_id":"ws-foreign","version_id":"mfv-1"}`,
		"draft version":              `{"workspace_id":"ws-a","version_id":"mfv-draft"}`,
	} {
		if w := doJSON(ir, "POST", "/organizations/1/manifests/mf-1/v2/deployments/install", body); w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d want 404 (%s)", name, w.Code, w.Body.String())
		}
	}
}
