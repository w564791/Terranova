package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// --- fakes -----------------------------------------------------------------

type fakeWorkspaceListResolver struct {
	access *service.WorkspaceListAccess
	last   service.WorkspaceListAccessRequest
}

func (f *fakeWorkspaceListResolver) ResolveWorkspaceListAccess(_ context.Context, req service.WorkspaceListAccessRequest) (*service.WorkspaceListAccess, error) {
	f.last = req
	return f.access, nil
}

// resourceChecker allows exactly the (resource -> max level) map.
type resourceChecker struct {
	levels map[valueobject.ResourceType]valueobject.PermissionLevel
	calls  []*service.CheckPermissionRequest
}

func (r *resourceChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	r.calls = append(r.calls, req)
	lvl := r.levels[req.ResourceType]
	return &service.CheckPermissionResult{IsAllowed: lvl >= req.RequiredLevel && lvl != valueobject.PermissionLevelNone, EffectiveLevel: lvl}, nil
}
func (r *resourceChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return r.CheckPermission(ctx, req)
}
func (r *resourceChecker) CheckBatchPermissions(context.Context, []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (r *resourceChecker) GetUserTeams(context.Context, string) ([]string, error) { return nil, nil }

func setupManifestIAMDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE manifests (id TEXT PRIMARY KEY, organization_id INTEGER, name TEXT, description TEXT, status TEXT, source_type TEXT NOT NULL DEFAULT 'native', git_repo_url TEXT, git_subpath TEXT, github_installation_id INTEGER, created_by TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_deployments (id TEXT PRIMARY KEY, manifest_id TEXT, version_id TEXT, workspace_id TEXT, variable_overrides TEXT, status TEXT, last_task_id INTEGER, deployed_by TEXT, deployed_at DATETIME, approved_bundle_hash TEXT, approved_plan_hash TEXT, sensitive_keys TEXT, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_deployment_varsets (deployment_id TEXT, varset_id TEXT, priority INTEGER)`,
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY AUTOINCREMENT, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary INTEGER, mode INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE variable_sets (id INTEGER PRIMARY KEY, varset_id TEXT, name TEXT, description TEXT, scope TEXT, is_deleted INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME, created_by TEXT)`,
		`CREATE TABLE varset_variables (id INTEGER PRIMARY KEY, variable_id TEXT, varset_id TEXT, key TEXT, value TEXT, variable_type TEXT, value_format TEXT, sensitive INTEGER, description TEXT, is_deleted INTEGER DEFAULT 0, version INTEGER, created_at DATETIME, updated_at DATETIME, created_by TEXT)`,
		`CREATE TABLE varset_assignments (id INTEGER PRIMARY KEY, varset_id TEXT, scope_type TEXT, project_id INTEGER, workspace_id TEXT, attached_at DATETIME, attached_by TEXT)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT, project_id INTEGER)`,
		`CREATE TABLE workspace_variables (id INTEGER PRIMARY KEY, variable_id TEXT, workspace_id TEXT, key TEXT, version INTEGER, value TEXT, variable_type TEXT, value_format TEXT, sensitive INTEGER, description TEXT, is_deleted INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME, created_by TEXT)`,
		`INSERT INTO manifests (id, organization_id, name, status, created_by) VALUES ('mf-1', 1, 'm1', 'draft', 'u1'), ('mf-other', 2, 'm2', 'draft', 'u1')`,
		`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id, status, deployed_by) VALUES
		   ('mfd-a', 'mf-1', 'mfv-1', 'ws-a', 'active', 'u1'),
		   ('mfd-b', 'mf-1', 'mfv-1', 'ws-b', 'active', 'u1')`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES ('ws-a', 10)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	return db
}

// withCaller simulates JWT + route-level RequirePermission(MANIFESTS ...).
func withCaller(level valueobject.PermissionLevel) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Set("auth_org_id", uint(1))
		c.Set("permission_check_result", &service.CheckPermissionResult{IsAllowed: true, EffectiveLevel: level})
		c.Next()
	}
}

func doJSON(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// --- can_write / can_deploy ------------------------------------------------

func TestListManifests_CanWriteAndCanDeploy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name          string
		manifestLevel valueobject.PermissionLevel
		access        *service.WorkspaceListAccess
		wantWrite     bool
		wantDeploy    bool
	}{
		{"read only, no writable workspace", valueobject.PermissionLevelRead, &service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{}}, false, false},
		// MANIFESTS READ must never imply can_deploy; can_deploy comes only from WORKSPACE_RESOURCES WRITE
		{"read + writable workspace", valueobject.PermissionLevelRead, &service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{"ws-a"}}, false, true},
		{"admin but no writable workspace", valueobject.PermissionLevelAdmin, &service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{}}, true, false},
		{"write + org-wide capability", valueobject.PermissionLevelWrite, &service.WorkspaceListAccess{FullOrganization: true, HasAccess: true}, true, true},
		{"no workspace access at all", valueobject.PermissionLevelWrite, &service.WorkspaceListAccess{}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupManifestIAMDB(t)
			resolver := &fakeWorkspaceListResolver{access: tc.access}
			perm := middleware.NewIAMPermissionMiddlewareWithChecker(&resourceChecker{}).WithWorkspaceListAccess(resolver)
			h := NewManifestHandler(db, perm)
			r := gin.New()
			r.Use(middleware.ErrorHandler()) // production 500 path (router.go)
			r.GET("/organizations/:org_id/manifests", withCaller(tc.manifestLevel), h.ListManifests)
			r.GET("/organizations/:org_id/manifests/:id", withCaller(tc.manifestLevel), h.GetManifest)

			w := doJSON(r, "GET", "/organizations/1/manifests", "")
			if w.Code != http.StatusOK {
				t.Fatalf("list: %d %s", w.Code, w.Body.String())
			}
			var body struct {
				Items []map[string]interface{} `json:"items"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if len(body.Items) != 1 {
				t.Fatalf("want 1 org-scoped manifest, got %d", len(body.Items))
			}
			if body.Items[0]["can_write"] != tc.wantWrite || body.Items[0]["can_deploy"] != tc.wantDeploy {
				t.Fatalf("can_write=%v can_deploy=%v, want %v/%v", body.Items[0]["can_write"], body.Items[0]["can_deploy"], tc.wantWrite, tc.wantDeploy)
			}
			if c := resolver.last.Capability; c == nil || c.ResourceType != valueobject.ResourceTypeWorkspaceResources || c.Level != valueobject.PermissionLevelWrite {
				t.Fatalf("can_deploy must use the WORKSPACE_RESOURCES:WRITE capability, got %+v", c)
			}

			w = doJSON(r, "GET", "/organizations/1/manifests/mf-1", "")
			var one map[string]interface{}
			_ = json.Unmarshal(w.Body.Bytes(), &one)
			if one["can_write"] != tc.wantWrite || one["can_deploy"] != tc.wantDeploy {
				t.Fatalf("get: can_write=%v can_deploy=%v", one["can_write"], one["can_deploy"])
			}
		})
	}
}

func TestUpdateManifest_ArchiveRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		level valueobject.PermissionLevel
		body  string
		want  int
	}{
		{valueobject.PermissionLevelWrite, `{"status":"archived"}`, http.StatusForbidden},
		{valueobject.PermissionLevelWrite, `{"description":"x"}`, http.StatusOK},
		{valueobject.PermissionLevelAdmin, `{"status":"archived"}`, http.StatusOK},
	} {
		db := setupManifestIAMDB(t)
		h := NewManifestHandler(db, nil)
		r := gin.New()
		r.Use(middleware.ErrorHandler()) // production 500 path (router.go)
		r.PUT("/organizations/:org_id/manifests/:id", withCaller(tc.level), h.UpdateManifest)
		if w := doJSON(r, "PUT", "/organizations/1/manifests/mf-1", tc.body); w.Code != tc.want {
			t.Fatalf("level %s body %s: got %d want %d (%s)", tc.level, tc.body, w.Code, tc.want, w.Body.String())
		}
	}
}

// --- deployments ------------------------------------------------------------

func TestListDeployments_FilteredToReadableWorkspaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupManifestIAMDB(t)
	h := NewManifestDeploymentsV2Handler(db, nil)
	for name, tc := range map[string]struct {
		access *service.WorkspaceListAccess
		want   []string
	}{
		"scoped": {&service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{"ws-a"}}, []string{"mfd-a"}},
		"empty":  {&service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{}}, nil},
		"full":   {&service.WorkspaceListAccess{FullOrganization: true, HasAccess: true}, []string{"mfd-a", "mfd-b"}},
	} {
		t.Run(name, func(t *testing.T) {
			r := gin.New()
			r.Use(middleware.ErrorHandler()) // production 500 path (router.go)
			r.GET("/organizations/:org_id/manifests/:id/v2/deployments", withCaller(valueobject.PermissionLevelRead), func(c *gin.Context) {
				c.Set(service.WorkspaceListAccessContextKey, tc.access)
			}, h.ListDeployments)
			w := doJSON(r, "GET", "/organizations/1/manifests/mf-1/v2/deployments", "")
			if w.Code != http.StatusOK {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			var body struct {
				Deployments []struct {
					ID string `json:"id"`
				} `json:"deployments"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			got := map[string]bool{}
			for _, d := range body.Deployments {
				got[d.ID] = true
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for _, id := range tc.want {
				if !got[id] {
					t.Fatalf("missing %s in %v", id, got)
				}
			}
		})
	}

	// missing allow-list context fails closed; another org's manifest is 404
	r := gin.New()
	r.Use(middleware.ErrorHandler()) // production 500 path (router.go)
	r.GET("/organizations/:org_id/manifests/:id/v2/deployments", withCaller(valueobject.PermissionLevelRead), h.ListDeployments)
	if w := doJSON(r, "GET", "/organizations/1/manifests/mf-1/v2/deployments", ""); w.Code != http.StatusInternalServerError {
		t.Fatalf("missing access context must fail closed, got %d", w.Code)
	}
	if w := doJSON(r, "GET", "/organizations/1/manifests/mf-other/v2/deployments", ""); w.Code != http.StatusNotFound {
		t.Fatalf("cross-org manifest must 404, got %d", w.Code)
	}
}

func TestDeploymentWrites_RequireWorkspaceResourcesWriteOnTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// MANIFESTS ADMIN + WORKSPACE_MANAGEMENT READ, but no WORKSPACE_RESOURCES WRITE
	checker := &resourceChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeManifests:           valueobject.PermissionLevelAdmin,
		valueobject.ResourceTypeWorkspaceResources:  valueobject.PermissionLevelRead,
		valueobject.ResourceTypeWorkspaceManagement: valueobject.PermissionLevelRead,
	}}
	db := setupManifestIAMDB(t)
	h := NewManifestDeploymentsV2Handler(db, middleware.NewIAMPermissionMiddlewareWithChecker(checker))
	r := gin.New()
	r.Use(middleware.ErrorHandler()) // production 500 path (router.go)
	g := r.Group("/organizations/:org_id/manifests/:id/v2/deployments", withCaller(valueobject.PermissionLevelAdmin))
	g.POST("/install", h.Install)
	g.POST("/:deployment_id/upgrade", h.Upgrade)
	g.POST("/:deployment_id/uninstall", h.Uninstall)

	for _, tc := range []struct{ path, body, wantWS string }{
		{"/organizations/1/manifests/mf-1/v2/deployments/install", `{"version_id":"mfv-1","workspace_id":"ws-body"}`, "ws-body"},
		{"/organizations/1/manifests/mf-1/v2/deployments/mfd-a/upgrade", `{"target_version_id":"mfv-1"}`, "ws-a"},
		{"/organizations/1/manifests/mf-1/v2/deployments/mfd-a/uninstall", `{}`, "ws-a"},
	} {
		checker.calls = nil
		w := doJSON(r, "POST", tc.path, tc.body)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d want 403 (%s)", tc.path, w.Code, w.Body.String())
		}
		last := checker.calls[len(checker.calls)-1]
		if last.ResourceType != valueobject.ResourceTypeWorkspaceResources || last.RequiredLevel != valueobject.PermissionLevelWrite ||
			last.ScopeType != valueobject.ScopeTypeWorkspace || last.ScopeIDStr != tc.wantWS {
			t.Fatalf("%s: unexpected check %+v", tc.path, last)
		}
	}
}

// --- variable preview: structured + sensitive masking ----------------------

func TestVariablePreview_SensitiveValuesAreNeverPrefilled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupManifestIAMDB(t)
	for _, stmt := range []string{
		`INSERT INTO variable_sets (varset_id, name, scope) VALUES ('vs-global', 'g', 'global'), ('vs-proj', 'p', 'specific'), ('vs-foreign', 'f', 'specific')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-proj', 'project', 10)`,
		`INSERT INTO varset_variables (variable_id, varset_id, key, value, variable_type, value_format, sensitive, version) VALUES
		   ('v1', 'vs-global', 'region', 'us-east-1', 'terraform', 'string', 0, 1),
		   ('v2', 'vs-proj', 'db_password', 'hunter2', 'terraform', 'string', 1, 1),
		   ('v3', 'vs-foreign', 'leak', 'secret-from-elsewhere', 'terraform', 'string', 0, 1)`,
		`INSERT INTO workspace_variables (variable_id, workspace_id, key, version, value, variable_type, value_format, sensitive) VALUES
		   ('wv1', 'ws-a', 'api_token', 1, 'tok-123', 'terraform', 'string', 1)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	sealFixtureVersions(t, db)
	checker := &resourceChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeWorkspaceVars: valueobject.PermissionLevelRead,
	}}
	h := NewManifestDeploymentsV2Handler(db, middleware.NewIAMPermissionMiddlewareWithChecker(checker))
	r := gin.New()
	r.Use(middleware.ErrorHandler()) // production 500 path (router.go)
	r.POST("/organizations/:org_id/manifests/:id/v2/deployments/:deployment_id/variable-preview", withCaller(valueobject.PermissionLevelRead), h.VariablePreview)
	path := "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/variable-preview"

	// an override on a sensitive key must not turn it into a prefilled value
	w := doJSON(r, "POST", path, `{"varsets":[{"varset_id":"vs-proj","priority":1}],"variable_overrides":{"db_password":"typed","extra":"x"}}`)
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
		t.Fatalf("variables must be a structured list: %v (%s)", err, w.Body.String())
	}
	got := map[string]struct {
		v string
		s bool
	}{}
	for _, v := range body.Variables {
		got[v.Key] = struct {
			v string
			s bool
		}{v.Value, v.Sensitive}
	}
	want := map[string]struct {
		v string
		s bool
	}{
		"region":      {"us-east-1", false},
		"db_password": {"", true},
		"api_token":   {"", true},
		"extra":       {"x", false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, wv := range want {
		if got[k] != wv {
			t.Fatalf("%s: got %+v want %+v", k, got[k], wv)
		}
	}
	if strings.Contains(w.Body.String(), "hunter2") || strings.Contains(w.Body.String(), "tok-123") {
		t.Fatal("sensitive value leaked in preview response")
	}

	// varsets the target workspace cannot mount are rejected
	w = doJSON(r, "POST", path, `{"varsets":[{"varset_id":"vs-foreign","priority":1}]}`)
	if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "secret-from-elsewhere") {
		t.Fatalf("unmountable varset must be rejected: %d %s", w.Code, w.Body.String())
	}

	// preview requires WORKSPACE_VARIABLES READ on the target workspace
	checker.levels = map[valueobject.ResourceType]valueobject.PermissionLevel{}
	if w := doJSON(r, "POST", path, `{}`); w.Code != http.StatusForbidden {
		t.Fatalf("missing WORKSPACE_VARIABLES READ must 403, got %d", w.Code)
	}
}
