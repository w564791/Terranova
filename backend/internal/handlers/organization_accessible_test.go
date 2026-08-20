package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAccessibleOrganizationsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE organizations (
  id INTEGER PRIMARY KEY,
  name TEXT,
  display_name TEXT,
  description TEXT,
  is_active BOOLEAN,
  settings TEXT,
  created_by TEXT,
  created_at DATETIME,
  updated_at DATETIME
);
CREATE TABLE user_organizations (
  id INTEGER PRIMARY KEY,
  user_id TEXT,
  org_id INTEGER,
  joined_at DATETIME
);
INSERT INTO organizations (id, name, is_active, created_at, updated_at) VALUES
  (1, 'member-org', 1, '2026-01-01', '2026-01-01'),
  (2, 'inactive-org', 0, '2026-01-02', '2026-01-02'),
  (3, 'unrelated-org', 1, '2026-01-03', '2026-01-03');
INSERT INTO user_organizations (id, user_id, org_id, joined_at) VALUES
  (1, 'user-a', 1, '2026-01-01'),
  (2, 'user-a', 2, '2026-01-02');`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestListAccessibleOrganizations_IsMembershipBound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccessibleOrganizationsDB(t)
	orgService := service.NewOrganizationService(persistence.NewOrganizationRepository(db), nil, nil)
	h := NewOrganizationHandler(orgService, nil)

	r := gin.New()
	r.GET("/iam/organizations/accessible", func(c *gin.Context) {
		c.Set("user_id", "user-a")
		c.Next()
	}, h.ListAccessibleOrganizations)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/iam/organizations/accessible", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "member-org") || strings.Contains(body, "inactive-org") || strings.Contains(body, "unrelated-org") {
		t.Fatalf("accessible orgs must be active memberships only, got %s", body)
	}
	if !strings.Contains(body, `"default_org_id":1`) {
		t.Fatalf("expected deterministic default, got %s", body)
	}
}

func TestListAccessibleOrganizations_SystemAdminGetsPlatformList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccessibleOrganizationsDB(t)
	orgService := service.NewOrganizationService(persistence.NewOrganizationRepository(db), nil, nil)
	h := NewOrganizationHandler(orgService, nil)

	r := gin.New()
	r.GET("/iam/organizations/accessible", func(c *gin.Context) {
		c.Set("user_id", "system-admin")
		c.Set("is_system_admin", true)
		c.Next()
	}, h.ListAccessibleOrganizations)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/iam/organizations/accessible", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "member-org") || !strings.Contains(body, "inactive-org") || !strings.Contains(body, "unrelated-org") {
		t.Fatalf("system admin should receive full organization list, got %s", body)
	}
}
