package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
)

// denyChecker grants everything at ADMIN except the listed resource types and
// records every evaluated request.
type denyChecker struct {
	mu   sync.Mutex
	deny map[valueobject.ResourceType]bool
	reqs []*service.CheckPermissionRequest
}

func (d *denyChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reqs = append(d.reqs, req)
	if d.deny[req.ResourceType] {
		return &service.CheckPermissionResult{IsAllowed: false, EffectiveLevel: valueobject.PermissionLevelRead}, nil
	}
	return &service.CheckPermissionResult{IsAllowed: true, EffectiveLevel: valueobject.PermissionLevelAdmin}, nil
}
func (d *denyChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return d.CheckPermission(ctx, req)
}
func (d *denyChecker) CheckBatchPermissions(context.Context, []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (d *denyChecker) GetUserTeams(context.Context, string) ([]string, error) { return nil, nil }
func (d *denyChecker) reset() []*service.CheckPermissionRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.reqs
	d.reqs = nil
	return out
}

// Fixtures on top of setupVarsetRouter (org 1: project 10, ws-a/ws-b;
// org 2: project 30, ws-other). Caller u1 works in org 1.
var varsetOrgFixtures = []string{
	`INSERT INTO variable_sets (varset_id, name, scope, created_by) VALUES
	   ('vs-foreign', 'f', 'specific', 'u2'), ('vs-mixed', 'x', 'specific', 'u2'),
	   ('vs-mine', 'm', 'specific', 'u1'), ('vs-theirs', 't', 'specific', 'u2')`,
	`INSERT INTO varset_assignments (id, varset_id, scope_type, project_id, workspace_id) VALUES
	   (101, 'vs-foreign', 'workspace', NULL, 'ws-other'),
	   (102, 'vs-mixed', 'workspace', NULL, 'ws-a'), (103, 'vs-mixed', 'project', 30, NULL)`,
}

func setupVarsetOrgRouter(t *testing.T, checker *denyChecker, systemAdmin bool) *gin.Engine {
	t.Helper()
	return setupVarsetRouterWith(t, checker, func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Set("is_system_admin", systemAdmin)
	}, varsetOrgFixtures...)
}

func doVarsetReq(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	if body == "" {
		body = `{}`
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path+sep+"org_id=1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

const varsetByIDPrefix = "/api/v1/variable-sets/:varset_id"

// Route-table assertion: every /variable-sets/:varset_id route evaluates its
// permission first and then binds the varset to the caller org (404 for a
// varset of another org, even with an allow-all checker).
func TestVarsetByIDRoutesBindToOrgAfterPermission(t *testing.T) {
	checker := &denyChecker{}
	r := setupVarsetOrgRouter(t, checker, false)
	n := 0
	for _, ri := range r.Routes() {
		if !strings.HasPrefix(ri.Path, varsetByIDPrefix) {
			continue
		}
		n++
		path := strings.NewReplacer(":varset_id", "vs-foreign", ":var_id", "var-1", ":assignment_id", "101").Replace(ri.Path)
		t.Run(ri.Method+" "+ri.Path, func(t *testing.T) {
			checker.reset()
			w := doVarsetReq(r, ri.Method, path, `{"scope":"specific","scope_type":"workspace","workspace_id":"ws-a","name":"z","key":"k","value":"v"}`)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
			}
			reqs := checker.reset()
			if len(reqs) != 1 || reqs[0].ResourceType != valueobject.ResourceTypeVariableSets || reqs[0].ScopeID != 1 {
				t.Fatalf("want exactly the route's VARIABLE_SETS check on org 1 before the org guard, got %+v", reqs)
			}
		})
	}
	if n != 12 {
		t.Fatalf("/variable-sets/:varset_id has %d routes, want 12; review org binding for new routes", n)
	}
}

func TestVarsetCrossOrgReturns404(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	for _, id := range []string{"vs-foreign", "vs-theirs", "vs-missing", "vs-deleted"} {
		for _, tc := range []struct{ method, path, body string }{
			{"GET", "/api/v1/variable-sets/" + id, ""},
			{"GET", "/api/v1/variable-sets/" + id + "/variables", ""},
			{"PUT", "/api/v1/variable-sets/" + id, `{"name":"renamed"}`},
			{"DELETE", "/api/v1/variable-sets/" + id, ""},
			{"POST", "/api/v1/variable-sets/" + id + "/assignments", `{"scope_type":"workspace","workspace_id":"ws-a"}`},
		} {
			if w := doVarsetReq(r, tc.method, tc.path, tc.body); w.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404; body=%s", tc.method, tc.path, w.Code, w.Body.String())
			}
		}
	}
}

func TestVarsetGlobalWritableOnlyBySystemAdmin(t *testing.T) {
	orgAdmin := setupVarsetOrgRouter(t, &denyChecker{}, false)
	if w := doVarsetReq(orgAdmin, "GET", "/api/v1/variable-sets/vs-global", ""); w.Code != http.StatusOK {
		t.Fatalf("org admin read global = %d, want 200", w.Code)
	}
	for _, tc := range []struct{ method, path, body string }{
		{"PUT", "/api/v1/variable-sets/vs-global", `{"name":"renamed"}`},
		{"DELETE", "/api/v1/variable-sets/vs-global", ""},
		{"POST", "/api/v1/variable-sets/vs-global/variables", `{"key":"k","value":"v"}`},
	} {
		if w := doVarsetReq(orgAdmin, tc.method, tc.path, tc.body); w.Code != http.StatusForbidden {
			t.Errorf("org admin %s %s = %d, want 403", tc.method, tc.path, w.Code)
		}
	}
	super := setupVarsetOrgRouter(t, &denyChecker{}, true)
	if w := doVarsetReq(super, "PUT", "/api/v1/variable-sets/vs-global", `{"name":"renamed"}`); w.Code != http.StatusOK {
		t.Fatalf("system admin update global = %d, want 200; %s", w.Code, w.Body.String())
	}
}

// A varset assigned to this org and another one is readable here but read-only.
func TestVarsetAssignedToOtherOrgIsReadOnly(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	for _, p := range []string{"/api/v1/variable-sets/vs-mixed", "/api/v1/variable-sets/vs-mixed/assignments"} {
		if w := doVarsetReq(r, "GET", p, ""); w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", p, w.Code)
		}
	}
	for _, tc := range []struct{ method, path, body string }{
		{"PUT", "/api/v1/variable-sets/vs-mixed", `{"name":"renamed"}`},
		{"DELETE", "/api/v1/variable-sets/vs-mixed", ""},
		{"POST", "/api/v1/variable-sets/vs-mixed/variables", `{"key":"k","value":"v"}`},
		{"POST", "/api/v1/variable-sets/vs-mixed/assignments", `{"scope_type":"workspace","workspace_id":"ws-b"}`},
		{"DELETE", "/api/v1/variable-sets/vs-mixed/assignments/102", ""},
	} {
		if w := doVarsetReq(r, tc.method, tc.path, tc.body); w.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403; %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestVarsetOwnUnassignedIsWritable(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	if w := doVarsetReq(r, "PUT", "/api/v1/variable-sets/vs-mine", `{"name":"mine-renamed"}`); w.Code != http.StatusOK {
		t.Fatalf("update own unassigned = %d, want 200; %s", w.Code, w.Body.String())
	}
	if w := doVarsetReq(r, "POST", "/api/v1/variable-sets/vs-mine/assignments", `{"scope_type":"workspace","workspace_id":"ws-a"}`); w.Code != http.StatusCreated {
		t.Fatalf("assign own varset to same-org workspace = %d, want 201; %s", w.Code, w.Body.String())
	}
	// now assigned inside org 1 only: still writable (by anyone in the org)
	if w := doVarsetReq(r, "PUT", "/api/v1/variable-sets/vs-proj", `{"name":"p2"}`); w.Code != http.StatusOK {
		t.Fatalf("update org-assigned varset = %d, want 200; %s", w.Code, w.Body.String())
	}
}

func TestVarsetAssignmentTargetValidation(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	for name, body := range map[string]string{
		"workspace of another org": `{"scope_type":"workspace","workspace_id":"ws-other"}`,
		"unknown workspace":        `{"scope_type":"workspace","workspace_id":"ws-nope"}`,
		"project of another org":   `{"scope_type":"project","project_id":30}`,
		"unknown project":          `{"scope_type":"project","project_id":999}`,
	} {
		if w := doVarsetReq(r, "POST", "/api/v1/variable-sets/vs-mine/assignments", body); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404; %s", name, w.Code, w.Body.String())
		}
	}

	noVarsWrite := &denyChecker{deny: map[valueobject.ResourceType]bool{valueobject.ResourceTypeWorkspaceVars: true}}
	r = setupVarsetOrgRouter(t, noVarsWrite, false)
	for name, body := range map[string]string{
		"same-org workspace": `{"scope_type":"workspace","workspace_id":"ws-a"}`,
		"same-org project":   `{"scope_type":"project","project_id":10}`,
	} {
		noVarsWrite.reset()
		if w := doVarsetReq(r, "POST", "/api/v1/variable-sets/vs-mine/assignments", body); w.Code != http.StatusForbidden {
			t.Errorf("%s without WORKSPACE_VARIABLES WRITE: %d, want 403; %s", name, w.Code, w.Body.String())
		}
		reqs := noVarsWrite.reset()
		last := reqs[len(reqs)-1]
		if last.ResourceType != valueobject.ResourceTypeWorkspaceVars || last.RequiredLevel != valueobject.PermissionLevelWrite {
			t.Errorf("%s: last check %+v, want WORKSPACE_VARIABLES WRITE on the target", name, last)
		}
		if name == "same-org project" && (last.ScopeType != valueobject.ScopeTypeProject || last.ScopeID != 10) {
			t.Errorf("project target checked at %s/%d, want PROJECT/10", last.ScopeType, last.ScopeID)
		}
		if name == "same-org workspace" && (last.ScopeType != valueobject.ScopeTypeWorkspace || last.ScopeIDStr != "ws-a") {
			t.Errorf("workspace target checked at %s/%s, want WORKSPACE/ws-a", last.ScopeType, last.ScopeIDStr)
		}
	}
	// unassigning also needs WORKSPACE_VARIABLES WRITE on the assignment's target
	if w := doVarsetReq(r, "DELETE", "/api/v1/variable-sets/vs-proj/assignments/1", ""); w.Code != http.StatusForbidden {
		t.Errorf("delete assignment without target WRITE: %d, want 403; %s", w.Code, w.Body.String())
	}
}

// Making a varset global (it then reaches every org) is system-admin only.
func TestVarsetGlobalScopeRequiresSystemAdmin(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	if w := doVarsetReq(r, "PUT", "/api/v1/variable-sets/vs-mine/scope", `{"scope":"global"}`); w.Code != http.StatusForbidden {
		t.Fatalf("org admin set scope=global: %d, want 403; %s", w.Code, w.Body.String())
	}
	super := setupVarsetOrgRouter(t, &denyChecker{}, true)
	if w := doVarsetReq(super, "PUT", "/api/v1/variable-sets/vs-mine/scope", `{"scope":"global"}`); w.Code != http.StatusOK {
		t.Fatalf("system admin set scope=global: %d, want 200; %s", w.Code, w.Body.String())
	}
}

// The manifest reverse-lookup route /variable-sets/:varset_id/manifest-deployments
// (registered by RegisterManifestRoutes) carries the same read guard after its
// permission check.
func TestVarsetReverseLookupBindsToOrg(t *testing.T) {
	checker := &allowAllChecker{}
	r, db, token := setupManifestRouterWithChecker(t, checker)
	for _, stmt := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`,
		`CREATE TABLE variable_sets (id INTEGER PRIMARY KEY, varset_id TEXT, name TEXT, scope TEXT, is_deleted INTEGER DEFAULT 0, created_by TEXT)`,
		`CREATE TABLE varset_assignments (id INTEGER PRIMARY KEY, varset_id TEXT, scope_type TEXT, project_id INTEGER, workspace_id TEXT)`,
		`INSERT INTO projects VALUES (10, 1), (30, 2)`,
		`INSERT INTO variable_sets (varset_id, name, scope, created_by) VALUES ('vs-foreign', 'f', 'specific', 'u2')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-foreign', 'project', 30)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	for _, path := range []string{
		"/api/v1/variable-sets/vs-foreign/manifest-deployments?org_id=1",
		"/api/v1/variable-sets/vs-missing/manifest-deployments?org_id=1",
	} {
		checker.reset()
		w := doManifestReq(r, token, "GET", path)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s = %d, want 404; %s", path, w.Code, w.Body.String())
		}
		if reqs := checker.reset(); len(reqs) != 1 || reqs[0].ResourceType != valueobject.ResourceTypeVariableSets {
			t.Fatalf("%s: want the VARIABLE_SETS check before the org guard, got %+v", path, reqs)
		}
	}
}
