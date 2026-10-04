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

// allowAllChecker grants every evaluated permission at ADMIN and records calls.
type allowAllChecker struct {
	mu   sync.Mutex
	reqs []*service.CheckPermissionRequest
}

func (a *allowAllChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reqs = append(a.reqs, req)
	return &service.CheckPermissionResult{IsAllowed: true, EffectiveLevel: valueobject.PermissionLevelAdmin}, nil
}
func (a *allowAllChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return a.CheckPermission(ctx, req)
}
func (a *allowAllChecker) CheckBatchPermissions(context.Context, []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (a *allowAllChecker) GetUserTeams(context.Context, string) ([]string, error) { return nil, nil }
func (a *allowAllChecker) reset() []*service.CheckPermissionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.reqs
	a.reqs = nil
	return out
}

const manifestV2GroupPrefix = "/api/v1/organizations/:org_id/manifests/:id/"

// manifestV2GroupRoutes returns every route registered in the
// /organizations/:org_id/manifests/:id group, with path params filled in for
// the given org and manifest.
func manifestV2GroupRoutes(t *testing.T, r *gin.Engine, org, manifest string) []gin.RouteInfo {
	t.Helper()
	var out []gin.RouteInfo
	for _, ri := range r.Routes() {
		if !strings.HasPrefix(ri.Path, manifestV2GroupPrefix) {
			continue
		}
		p := strings.NewReplacer(
			":org_id", org, ":id", manifest,
			":version_id", "mfv-1", ":deployment_id", "mfd-1", "*path", "main.tf",
		).Replace(ri.Path)
		out = append(out, gin.RouteInfo{Method: ri.Method, Path: p})
	}
	// 23 routes of registerManifestV2Routes + GET /:id/export-zip of the CRUD
	// group (whose handler filters by organization_id itself).
	if len(out) != 24 {
		t.Fatalf("manifest /:id/* has %d routes, want 24; review org binding for new routes", len(out))
	}
	return out
}

func doManifestReq(r *gin.Engine, token, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(`{"version":"v9.9.9"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// Route-table assertion: every route in the group binds the path manifest to
// the org AFTER its permission middleware. With an allow-all checker a
// cross-org request must evaluate the permission first and then stop at 404.
func TestManifestV2GroupBindsManifestToOrgAfterPermission(t *testing.T) {
	checker := &allowAllChecker{}
	r, _, token := setupManifestRouterWithChecker(t, checker)

	// org B (2) caller addresses org A's (1) manifest mf-1 through its own org.
	for _, rt := range manifestV2GroupRoutes(t, r, "2", "mf-1") {
		t.Run(rt.Method+" "+rt.Path, func(t *testing.T) {
			checker.reset()
			w := doManifestReq(r, token, rt.Method, rt.Path)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
			}
			reqs := checker.reset()
			if len(reqs) == 0 {
				t.Fatal("org check ran before the route permission middleware")
			}
			if reqs[0].ScopeID != 2 {
				t.Fatalf("permission evaluated on org %d, want path org 2", reqs[0].ScopeID)
			}
		})
	}
	// nonexistent manifest in own org is also 404
	if w := doManifestReq(r, token, "GET", "/api/v1/organizations/1/manifests/mf-missing/files"); w.Code != http.StatusNotFound {
		t.Fatalf("missing manifest status = %d, want 404", w.Code)
	}
}

// (b) explicit: org-B user on org-A manifest -> 404 on files, versions, export, publish.
func TestManifestCrossOrgReturns404(t *testing.T) {
	r, _, token := setupManifestRouterWithChecker(t, &allowAllChecker{})
	const base = "/api/v1/organizations/2/manifests/mf-1"
	for _, tc := range []struct{ method, path string }{
		{"GET", base + "/files"},
		{"GET", base + "/files/main.tf"},
		{"GET", base + "/v2/versions"},
		{"GET", base + "/v2/versions/mfv-1"},
		{"POST", base + "/v2/versions/mfv-1/files/_export"},
		{"POST", base + "/draft/_export"},
		{"POST", base + "/v2/versions"}, // publish
	} {
		if w := doManifestReq(r, token, tc.method, tc.path); w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404; body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// (a) same-org caller with permission reaches the handler (guards against the
// org check running before auth_org_id is set, which would 400 everything).
func TestManifestSameOrgFilesReachHandler(t *testing.T) {
	r, _, token := setupManifestRouterWithChecker(t, &allowAllChecker{})
	w := doManifestReq(r, token, "GET", "/api/v1/organizations/1/manifests/mf-1/files?version=mfv-1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "main.tf") {
		t.Fatalf("expected file listing, got %s", w.Body.String())
	}
}

// (c) a caller without permission gets 403 (not 404) on every group route,
// whether the manifest is in another org or does not exist: no ID probing.
func TestManifestNoPermissionIs403NotProbe(t *testing.T) {
	r, _, token := setupManifestRouterWithChecker(t, &denyAllChecker{})
	for _, manifest := range []string{"mf-b", "mf-missing"} {
		for _, rt := range manifestV2GroupRoutes(t, r, "1", manifest) {
			if w := doManifestReq(r, token, rt.Method, rt.Path); w.Code != http.StatusForbidden {
				t.Errorf("%s %s = %d, want 403", rt.Method, rt.Path, w.Code)
			}
		}
	}
}
