package router

import (
	"context"
	"encoding/json"
	"fmt"
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
	`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES
	   ('vs-foreign', 'f', 'specific', 2, 'u2'),
	   ('vs-mixed', 'x', 'specific', NULL, 'u2'),
	   ('vs-mine', 'm', 'specific', NULL, 'u1'),
	   ('vs-theirs', 't', 'specific', NULL, 'u2'),
	   ('vs-org1', 'o1', 'specific', 1, 'u1')`,
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

// A legacy orphan (org_id NULL) mounted here is readable (compat), but only
// its creator may write it; anyone else gets 404 on writes.
func TestVarsetOrphanNonCreatorWrite404(t *testing.T) {
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
		if w := doVarsetReq(r, tc.method, tc.path, tc.body); w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404; %s", tc.method, tc.path, w.Code, w.Body.String())
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
		`CREATE TABLE variable_sets (id INTEGER PRIMARY KEY, varset_id TEXT, name TEXT, scope TEXT, org_id INTEGER, is_deleted INTEGER DEFAULT 0, created_by TEXT)`,
		`CREATE TABLE varset_assignments (id INTEGER PRIMARY KEY, varset_id TEXT, scope_type TEXT, project_id INTEGER, workspace_id TEXT)`,
		`INSERT INTO projects VALUES (10, 1), (30, 2)`,
		`INSERT INTO variable_sets (varset_id, name, scope, org_id, created_by) VALUES ('vs-foreign', 'f', 'specific', 2, 'u2')`,
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

func TestVarsetCreateSetsCallerOrgID(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	w := doVarsetReq(r, "POST", "/api/v1/variable-sets", `{"name":"brand-new","description":"","scope":"specific"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201; %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"org_id":1`) {
		t.Fatalf("create body missing org_id=1: %s", w.Body.String())
	}
}

func TestVarsetOrgBCannotReadOrgA(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	// vs-org1 belongs to org 1; request as org 2 via query override
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/variable-sets/vs-org1?org_id=2", nil)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("org B read org A varset = %d, want 404; %s", w.Code, w.Body.String())
	}
}

func TestVarsetMountOnOtherOrgWorkspace404(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false)
	// Auth org 1, try to mount org-1 varset onto ws-other (org 2) → target 404
	w := doVarsetReq(r, "POST", "/api/v1/variable-sets/vs-org1/assignments",
		`{"scope_type":"workspace","workspace_id":"ws-other"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("mount on other-org workspace = %d, want 404; %s", w.Code, w.Body.String())
	}
}

func varsetOrgOf(t *testing.T, r *gin.Engine, id string) string {
	t.Helper()
	w := doVarsetReq(r, "GET", "/api/v1/variable-sets/"+id, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d; %s", id, w.Code, w.Body.String())
	}
	var body struct {
		OrgID *uint `json:"org_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.OrgID == nil {
		return "null"
	}
	return fmt.Sprint(*body.OrgID)
}

// Every write path heals a creator-owned orphan: org_id = caller auth org.
func TestVarsetOrphanHealedOnWriteByCreator(t *testing.T) {
	for _, tc := range []struct{ name, method, path, body string }{
		{"rename", "PUT", "/api/v1/variable-sets/vs-mine", `{"name":"healed"}`},
		{"add variable", "POST", "/api/v1/variable-sets/vs-mine/variables", `{"key":"k","value":"v","variable_type":"terraform"}`},
		{"mount", "POST", "/api/v1/variable-sets/vs-mine/assignments", `{"scope_type":"workspace","workspace_id":"ws-a"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := setupVarsetOrgRouter(t, &denyChecker{}, false)
			if got := varsetOrgOf(t, r, "vs-mine"); got != "null" {
				t.Fatalf("precondition org_id = %s", got)
			}
			if w := doVarsetReq(r, tc.method, tc.path, tc.body); w.Code >= 300 {
				t.Fatalf("%s = %d; %s", tc.name, w.Code, w.Body.String())
			}
			if got := varsetOrgOf(t, r, "vs-mine"); got != "1" {
				t.Fatalf("after %s org_id = %s, want 1", tc.name, got)
			}
		})
	}
}

// Another user cannot write (or thereby claim) someone else's orphan: 404.
func TestVarsetOrphanNonCreatorCannotHeal(t *testing.T) {
	r := setupVarsetOrgRouter(t, &denyChecker{}, false) // caller u1
	for _, tc := range []struct{ method, path, body string }{
		{"PUT", "/api/v1/variable-sets/vs-theirs", `{"name":"stolen"}`},
		{"POST", "/api/v1/variable-sets/vs-theirs/variables", `{"key":"k","value":"v","variable_type":"terraform"}`},
		{"POST", "/api/v1/variable-sets/vs-theirs/assignments", `{"scope_type":"workspace","workspace_id":"ws-a"}`},
		{"DELETE", "/api/v1/variable-sets/vs-theirs", ""},
	} {
		if w := doVarsetReq(r, tc.method, tc.path, tc.body); w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404; %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// healGateChecker: the route-level VARIABLE_SETS check passes, but the fresh
// heal-time check (the second VARIABLE_SETS evaluation in the request) sees
// only READ - e.g. the grant changed, or a route whose gate is weaker.
type healGateChecker struct {
	denyChecker
	healLevel valueobject.PermissionLevel
	vsChecks  int
}

func (h *healGateChecker) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	if req.ResourceType == valueobject.ResourceTypeVariableSets {
		h.mu.Lock()
		h.vsChecks++
		n := h.vsChecks
		h.mu.Unlock()
		if n > 1 { // heal-time check
			ok := h.healLevel >= req.RequiredLevel
			return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: h.healLevel}, nil
		}
	}
	return h.denyChecker.CheckPermission(ctx, req)
}
func (h *healGateChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return h.CheckPermission(ctx, req)
}

// Creator with only READ in the auth org at heal time: 404, org_id stays NULL,
// on every heal path. With WRITE: healed.
func TestVarsetOrphanHealRequiresVarsetWrite(t *testing.T) {
	paths := []struct{ name, method, path, body string }{
		{"rename", "PUT", "/api/v1/variable-sets/vs-mine", `{"name":"healed"}`},
		{"scope", "PUT", "/api/v1/variable-sets/vs-mine/scope", `{"scope":"specific"}`},
		{"delete", "DELETE", "/api/v1/variable-sets/vs-mine", ""},
		{"add variable", "POST", "/api/v1/variable-sets/vs-mine/variables", `{"key":"k","value":"v","variable_type":"terraform"}`},
		{"mount", "POST", "/api/v1/variable-sets/vs-mine/assignments", `{"scope_type":"workspace","workspace_id":"ws-a"}`},
	}
	for _, tc := range paths {
		t.Run("read-only/"+tc.name, func(t *testing.T) {
			hc := &healGateChecker{healLevel: valueobject.PermissionLevelRead}
			r := setupVarsetRouterWith(t, hc, func(c *gin.Context) { c.Set("user_id", "u1") }, varsetOrgFixtures...)
			if w := doVarsetReq(r, tc.method, tc.path, tc.body); w.Code != http.StatusNotFound {
				t.Fatalf("%s = %d, want 404; %s", tc.name, w.Code, w.Body.String())
			}
			hc.healLevel = valueobject.PermissionLevelAdmin // let GET through for the assertion
			if got := varsetOrgOf(t, r, "vs-mine"); got != "null" {
				t.Fatalf("org_id = %s after denied %s, want null", got, tc.name)
			}
		})
		t.Run("write/"+tc.name, func(t *testing.T) {
			hc := &healGateChecker{healLevel: valueobject.PermissionLevelAdmin}
			r := setupVarsetRouterWith(t, hc, func(c *gin.Context) { c.Set("user_id", "u1") }, varsetOrgFixtures...)
			if w := doVarsetReq(r, tc.method, tc.path, tc.body); w.Code >= 300 {
				t.Fatalf("%s = %d; %s", tc.name, w.Code, w.Body.String())
			}
			if tc.name == "delete" {
				return // row is soft-deleted (org_id bound in the same UPDATE; checked on PG)
			}
			if got := varsetOrgOf(t, r, "vs-mine"); got != "1" {
				t.Fatalf("org_id = %s after %s, want 1", got, tc.name)
			}
		})
	}
	// Creator with only READ at the route gate: refused before any heal.
	ro := &denyChecker{}
	r := setupVarsetRouterWith(t, readOnlyVarsets{ro}, func(c *gin.Context) { c.Set("user_id", "u1") }, varsetOrgFixtures...)
	if w := doVarsetReq(r, "PUT", "/api/v1/variable-sets/vs-mine", `{"name":"x"}`); w.Code < 400 {
		t.Fatalf("read-only rename = %d", w.Code)
	}
	if got := varsetOrgOf(t, r, "vs-mine"); got != "null" {
		t.Fatalf("org_id = %s, want null", got)
	}
}

type readOnlyVarsets struct{ *denyChecker }

func (r readOnlyVarsets) CheckPermission(ctx context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	if req.ResourceType == valueobject.ResourceTypeVariableSets {
		ok := req.RequiredLevel <= valueobject.PermissionLevelRead
		return &service.CheckPermissionResult{IsAllowed: ok, EffectiveLevel: valueobject.PermissionLevelRead}, nil
	}
	return r.denyChecker.CheckPermission(ctx, req)
}
func (r readOnlyVarsets) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return r.CheckPermission(ctx, req)
}
