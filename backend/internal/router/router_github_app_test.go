package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Installation registry = org ADMIN (ORGANIZATION resource); the webhook is
// public and authenticated only by its signature.
func TestGitHubAppRoutesPermissions(t *testing.T) {
	checker := &denyAllChecker{}
	_, db, token := setupManifestRouterWithChecker(t, checker)
	r := newEngineForGitHubApp(db, checker)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/organizations/1/github-app/installations"},
		{"POST", "/api/v1/organizations/1/github-app/installations"},
		{"DELETE", "/api/v1/organizations/1/github-app/installations/42"},
	} {
		checker.reset()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{"installation_id":42}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		reqs := checker.reset()
		if len(reqs) == 0 || reqs[0].ResourceType != "ORGANIZATION" || reqs[0].ScopeType != "ORGANIZATION" || reqs[0].RequiredLevel.String() != "ADMIN" {
			t.Fatalf("%s %s: checks %+v, want ORGANIZATION/ORGANIZATION/ADMIN", tc.method, tc.path, reqs)
		}
		// without a login token: 401 from JWTAuth
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without login: %d", tc.method, tc.path, w.Code)
		}
	}

	webhook := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/v1/webhooks/github", strings.NewReader(`{}`))
		req.Header.Set("X-GitHub-Event", "push")
		r.ServeHTTP(w, req)
		return w.Code
	}
	t.Setenv("GITHUB_WEBHOOK_SECRET", "")
	if code := webhook(); code != http.StatusServiceUnavailable {
		t.Fatalf("webhook without secret: %d", code)
	}
	t.Setenv("GITHUB_WEBHOOK_SECRET", "whsec")
	if code := webhook(); code != http.StatusUnauthorized {
		t.Fatalf("unsigned webhook: %d", code)
	}
	if len(checker.reset()) != 0 {
		t.Fatal("webhook must not evaluate IAM")
	}
}

func newEngineForGitHubApp(db *gorm.DB, checker *denyAllChecker) http.Handler {
	r := gin.New()
	api := r.Group("/api/v1")
	protected := api.Group("")
	protected.Use(middleware.JWTAuth())
	RegisterGitHubAppRoutes(api, protected, db, middleware.NewIAMPermissionMiddlewareWithChecker(checker))
	return r
}
