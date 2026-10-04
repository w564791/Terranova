package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// denyAllChecker records every evaluation and denies it, so a request stops at
// the first route-level IAM requirement. That first requirement is the route's
// permission contract asserted below.
type denyAllChecker struct {
	mu   sync.Mutex
	reqs []*service.CheckPermissionRequest
}

func (d *denyAllChecker) CheckPermission(_ context.Context, req *service.CheckPermissionRequest) (*service.CheckPermissionResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reqs = append(d.reqs, req)
	return &service.CheckPermissionResult{IsAllowed: false, DenyReason: "test deny"}, nil
}
func (d *denyAllChecker) CheckPermissionWithTemporary(ctx context.Context, req *service.CheckPermissionRequest, _ *uint) (*service.CheckPermissionResult, error) {
	return d.CheckPermission(ctx, req)
}
func (d *denyAllChecker) CheckBatchPermissions(context.Context, []*service.CheckPermissionRequest) ([]*service.CheckPermissionResult, error) {
	return nil, nil
}
func (d *denyAllChecker) GetUserTeams(context.Context, string) ([]string, error) { return nil, nil }

func (d *denyAllChecker) reset() []*service.CheckPermissionRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.reqs
	d.reqs = nil
	return out
}

func setupManifestRouterForPermissionTest(t *testing.T) (*gin.Engine, *denyAllChecker, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	t.Setenv("JWT_SECRET", secret)
	_ = os.Unsetenv("IAM_SINGLE_TENANT")

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE users (user_id TEXT PRIMARY KEY, is_active INTEGER, is_system_admin INTEGER)`,
		`CREATE TABLE login_sessions (session_id TEXT PRIMARY KEY, user_id TEXT, is_active INTEGER, expires_at DATETIME, last_used_at DATETIME)`,
		`INSERT INTO users (user_id, is_active, is_system_admin) VALUES ('user-1', 1, 0)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO login_sessions (session_id, user_id, is_active, expires_at) VALUES ('sess-1', 'user-1', 1, ?)`,
		time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	middleware.SetGlobalDB(db)
	t.Cleanup(func() { middleware.SetGlobalDB(nil) })

	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"type": "login_token", "user_id": "user-1", "session_id": "sess-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}

	checker := &denyAllChecker{}
	r := gin.New()
	RegisterManifestRoutes(r.Group("/api/v1"), db, nil, middleware.NewIAMPermissionMiddlewareWithChecker(checker))
	return r, checker, tok
}

func TestManifestRoutesPermissionTable(t *testing.T) {
	r, checker, token := setupManifestRouterForPermissionTest(t)
	const base = "/api/v1/organizations/1/manifests"

	cases := []struct {
		method, path, resource, scope, level string
	}{
		// catalog
		{"GET", base, "MANIFESTS", "ORGANIZATION", "READ"},
		{"POST", base, "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"GET", base + "/mf-1", "MANIFESTS", "ORGANIZATION", "READ"},
		{"PUT", base + "/mf-1", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"DELETE", base + "/mf-1", "MANIFESTS", "ORGANIZATION", "ADMIN"},
		{"GET", base + "/mf-1/export-zip", "MANIFESTS", "ORGANIZATION", "READ"},
		{"GET", base + "/mf-1/provider-schemas", "MANIFESTS", "ORGANIZATION", "READ"},
		// files / draft
		{"GET", base + "/mf-1/files", "MANIFESTS", "ORGANIZATION", "READ"},
		{"GET", base + "/mf-1/files/main.tf", "MANIFESTS", "ORGANIZATION", "READ"},
		{"PUT", base + "/mf-1/files/main.tf", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"DELETE", base + "/mf-1/files/main.tf", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"POST", base + "/mf-1/files/_move", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"POST", base + "/mf-1/files/_move_dir", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"POST", base + "/mf-1/files/_delete_dir", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"POST", base + "/mf-1/draft/_reset_from", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"POST", base + "/mf-1/draft/_export", "MANIFESTS", "ORGANIZATION", "READ"},
		// versions
		{"GET", base + "/mf-1/v2/versions", "MANIFESTS", "ORGANIZATION", "READ"},
		{"GET", base + "/mf-1/v2/versions/mfv-1", "MANIFESTS", "ORGANIZATION", "READ"},
		{"POST", base + "/mf-1/v2/versions", "MANIFESTS", "ORGANIZATION", "WRITE"},
		{"GET", base + "/mf-1/v2/versions/mfv-1/diff", "MANIFESTS", "ORGANIZATION", "READ"},
		{"GET", base + "/mf-1/v2/versions/mfv-1/workdirs", "MANIFESTS", "ORGANIZATION", "READ"},
		{"GET", base + "/mf-1/v2/draft/diff", "MANIFESTS", "ORGANIZATION", "READ"},
		{"POST", base + "/mf-1/v2/versions/mfv-1/files/_export", "MANIFESTS", "ORGANIZATION", "READ"},
		// deployments: MANIFESTS READ at the route; workspace checks are in-handler
		{"GET", base + "/mf-1/v2/deployments", "MANIFESTS", "ORGANIZATION", "READ"},
		{"GET", base + "/mf-1/v2/deployments/mfd-1", "MANIFESTS", "ORGANIZATION", "READ"},
		{"POST", base + "/mf-1/v2/deployments/install", "MANIFESTS", "ORGANIZATION", "READ"},
		{"POST", base + "/mf-1/v2/deployments/mfd-1/upgrade", "MANIFESTS", "ORGANIZATION", "READ"},
		{"POST", base + "/mf-1/v2/deployments/mfd-1/uninstall", "MANIFESTS", "ORGANIZATION", "READ"},
		{"POST", base + "/mf-1/v2/deployments/mfd-1/variable-preview", "MANIFESTS", "ORGANIZATION", "READ"},
		// editor / varset reverse lookup
		{"GET", "/api/v1/manifest-editor/modules?org_id=1", "MODULES", "ORGANIZATION", "READ"},
		{"GET", "/api/v1/manifest-editor/modules/1/demos?org_id=1", "MODULES", "ORGANIZATION", "READ"},
		{"GET", "/api/v1/manifest-editor/modules/1/inputs?org_id=1", "MODULES", "ORGANIZATION", "READ"},
		{"GET", "/api/v1/variable-sets/vs-1/manifest-deployments?org_id=1", "VARIABLE_SETS", "ORGANIZATION", "READ"},
		// unchanged
		{"GET", "/api/v1/workspaces/ws-1/manifest-summary?org_id=1", "WORKSPACES", "ORGANIZATION", "READ"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			checker.reset()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (deny-all checker); body=%s", w.Code, w.Body.String())
			}
			reqs := checker.reset()
			if len(reqs) == 0 {
				t.Fatal("no permission check was evaluated")
			}
			first := reqs[0]
			if string(first.ResourceType) != tc.resource || string(first.ScopeType) != tc.scope || first.RequiredLevel.String() != tc.level {
				t.Fatalf("first check = %s/%s/%s, want %s/%s/%s",
					first.ResourceType, first.ScopeType, first.RequiredLevel, tc.resource, tc.scope, tc.level)
			}
			for _, r := range reqs {
				if r.ResourceType == "SYSTEM_SETTINGS" || r.ResourceType == "ORGANIZATION" {
					t.Fatalf("stand-in permission %s must not be used", r.ResourceType)
				}
			}
		})
	}
}

// Every route registered by RegisterManifestRoutes must be in the table above,
// so a new route cannot silently ship without a reviewed permission.
func TestManifestRoutesPermissionTableIsComplete(t *testing.T) {
	r, _, _ := setupManifestRouterForPermissionTest(t)
	if got := len(r.Routes()); got != 34 {
		t.Fatalf("RegisterManifestRoutes registers %d routes; update TestManifestRoutesPermissionTable (34 covered)", got)
	}
}
