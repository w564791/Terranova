package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
)

func TestRequireWorkspaceListAccess_CapabilityQuery(t *testing.T) {
	resolver := &staticWorkspaceListAccessResolver{
		access: &service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{}},
	}
	m := &IAMPermissionMiddleware{workspaceListAccess: resolver}
	r := setupGin()
	r.GET("/workspaces", func(c *gin.Context) {
		c.Set("user_id", "u1")
		c.Next()
	}, m.RequireWorkspaceListAccess(), func(c *gin.Context) { c.Status(http.StatusOK) })

	// valid capability is parsed and forwarded to the shared resolver
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces?org_id=1&capability=WORKSPACE_RESOURCES:WRITE", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	if c := resolver.last.Capability; c == nil || c.ResourceType != valueobject.ResourceTypeWorkspaceResources || c.Level != valueobject.PermissionLevelWrite {
		t.Fatalf("capability not forwarded: %+v", resolver.last.Capability)
	}

	// no capability => plain list
	resolver.last = service.WorkspaceListAccessRequest{}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces?org_id=1", nil))
	if w.Code != http.StatusOK || resolver.last.Capability != nil {
		t.Fatalf("plain list must not carry a capability: %d %+v", w.Code, resolver.last.Capability)
	}

	// unknown values => 400 and the resolver is never called
	for _, bad := range []string{"NOPE:WRITE", "WORKSPACE_RESOURCES:SUPER", "MANIFESTS:WRITE", "WORKSPACE_RESOURCES"} {
		resolver.last = service.WorkspaceListAccessRequest{}
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/workspaces?org_id=1&capability="+bad, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("capability=%s: got %d, want 400", bad, w.Code)
		}
		if resolver.last.UserID != "" {
			t.Fatalf("capability=%s: resolver must not run", bad)
		}
	}
}

func TestHasWorkspaceCapability(t *testing.T) {
	for name, tc := range map[string]struct {
		access *service.WorkspaceListAccess
		want   bool
	}{
		"full":     {&service.WorkspaceListAccess{FullOrganization: true, HasAccess: true}, true},
		"one":      {&service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{"ws-a"}}, true},
		"empty":    {&service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{}}, false},
		"unauthzd": {&service.WorkspaceListAccess{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			resolver := &staticWorkspaceListAccessResolver{access: tc.access}
			m := &IAMPermissionMiddleware{workspaceListAccess: resolver}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/x?org_id=7", nil)
			c.Set("user_id", "u1")
			got, err := m.HasWorkspaceCapability(c, valueobject.ResourceTypeWorkspaceResources, valueobject.PermissionLevelWrite)
			if err != nil || got != tc.want {
				t.Fatalf("got %v, %v want %v", got, err, tc.want)
			}
			if resolver.last.Capability == nil || resolver.last.Capability.ResourceType != valueobject.ResourceTypeWorkspaceResources || resolver.last.OrgID != 7 {
				t.Fatalf("resolver request: %+v", resolver.last)
			}
			if w.Body.Len() != 0 {
				t.Fatal("HasWorkspaceCapability must not write a response")
			}
		})
	}
}

func TestRequireWorkspaceResourcePermission_ChecksGivenResource(t *testing.T) {
	mock := &mockPermChecker{allowed: false, level: valueobject.PermissionLevelRead, reason: "insufficient"}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/x", nil)
	c.Set("user_id", "u1")
	if m.RequireWorkspaceResourcePermission(c, "ws-1", "WORKSPACE_RESOURCES", "WRITE") {
		t.Fatal("must deny")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d", w.Code)
	}
	if mock.last.ResourceType != valueobject.ResourceTypeWorkspaceResources || mock.last.ScopeType != valueobject.ScopeTypeWorkspace ||
		mock.last.ScopeIDStr != "ws-1" || mock.last.RequiredLevel != valueobject.PermissionLevelWrite {
		t.Fatalf("unexpected check: %+v", mock.last)
	}
}

// RequireWorkspacePermission is relied upon by task read / run trigger and must
// keep checking WORKSPACE_MANAGEMENT.
func TestRequireWorkspacePermission_StillChecksWorkspaceManagement(t *testing.T) {
	mock := &mockPermChecker{allowed: true, level: valueobject.PermissionLevelWrite}
	m := &IAMPermissionMiddleware{permissionChecker: mock}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/x", nil)
	c.Set("user_id", "u1")
	if !m.RequireWorkspacePermission(c, "ws-1", "WRITE") {
		t.Fatal("must allow")
	}
	if mock.last.ResourceType != valueobject.ResourceTypeWorkspaceManagement {
		t.Fatalf("RequireWorkspacePermission must check WORKSPACE_MANAGEMENT, got %s", mock.last.ResourceType)
	}
}

func TestEffectiveLevelFromContext(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if EffectiveLevelFromContext(c) != valueobject.PermissionLevelNone {
		t.Fatal("missing result must be NONE")
	}
	c.Set("permission_check_result", &service.CheckPermissionResult{IsAllowed: true, EffectiveLevel: valueobject.PermissionLevelWrite})
	if EffectiveLevelFromContext(c) != valueobject.PermissionLevelWrite {
		t.Fatal("must return stored effective level")
	}
}
