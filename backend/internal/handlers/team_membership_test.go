package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"
	"iac-platform/internal/infrastructure/persistence"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTeamMembershipHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Organization{}, &entity.UserOrganization{}, &entity.Team{}, &entity.TeamMember{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX user_organizations_user_org_key ON user_organizations (user_id, org_id)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Organization{ID: 1, Name: "org-1", DisplayName: "Organization 1", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Team{ID: "team-1", OrgID: 1, Name: "team-1", DisplayName: "Team 1"}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestAddTeamMemberMakesOrganizationAccessible(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTeamMembershipHandlerDB(t)
	teamRepo := persistence.NewTeamRepository(db)
	orgRepo := persistence.NewOrganizationRepository(db)
	teamService := service.NewTeamService(teamRepo, orgRepo, nil)
	teamHandler := NewTeamHandler(teamService)
	organizationHandler := NewOrganizationHandler(service.NewOrganizationService(orgRepo, teamRepo, nil), nil)

	r := gin.New()
	r.POST("/iam/teams/:id/members", func(c *gin.Context) {
		c.Set("user_id", "admin-1")
		c.Set("auth_org_id", uint(1))
		c.Next()
	}, teamHandler.AddTeamMember)
	r.GET("/iam/organizations/accessible", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Next()
	}, organizationHandler.ListAccessibleOrganizations)

	add := httptest.NewRecorder()
	addReq := httptest.NewRequest(http.MethodPost, "/iam/teams/team-1/members", bytes.NewBufferString(`{"user_id":"user-1","role":"MEMBER"}`))
	addReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(add, addReq)
	if add.Code != http.StatusOK {
		t.Fatalf("add team member: %d %s", add.Code, add.Body.String())
	}

	accessible := httptest.NewRecorder()
	r.ServeHTTP(accessible, httptest.NewRequest(http.MethodGet, "/iam/organizations/accessible", nil))
	if accessible.Code != http.StatusOK {
		t.Fatalf("list accessible organizations: %d %s", accessible.Code, accessible.Body.String())
	}
	if !strings.Contains(accessible.Body.String(), "org-1") {
		t.Fatalf("team member must be able to bootstrap its organization, got %s", accessible.Body.String())
	}
}

func TestAssignTeamRoleRepairsExistingMemberOrganizationMemberships(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRoleHandlerDB(t)
	if err := db.Exec(`
CREATE TABLE iam_team_roles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  team_id TEXT NOT NULL,
  role_id INTEGER NOT NULL,
  scope_type TEXT NOT NULL,
  scope_id INTEGER NOT NULL,
  assigned_by TEXT,
  assigned_at DATETIME,
  expires_at DATETIME,
  reason TEXT
);
CREATE TABLE IF NOT EXISTS team_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  team_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  role TEXT,
  joined_at DATETIME,
  joined_by TEXT
);
CREATE UNIQUE INDEX user_organizations_user_org_key ON user_organizations (user_id, org_id);
INSERT INTO team_members (team_id, user_id, role) VALUES
  ('team-1', 'existing-member', 'MEMBER'),
  ('team-1', 'already-member', 'MEMBER');
INSERT INTO user_organizations (user_id, org_id, joined_at) VALUES
  ('already-member', 1, CURRENT_TIMESTAMP);`).Error; err != nil {
		t.Fatal(err)
	}

	role := &entity.Role{Name: "team-membership-role", DisplayName: "Team Membership Role", IsActive: true, OrgID: 1}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouterFull(h, true)
	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/teams/team-1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("assign team role: %d %s", w.Code, w.Body.String())
	}

	for _, userID := range []string{"existing-member", "already-member"} {
		var count int64
		if err := db.Model(&entity.UserOrganization{}).
			Where("user_id = ? AND org_id = ?", userID, 1).
			Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("want one membership for %s, got %d", userID, count)
		}
	}
}

func TestAssignTeamRoleRollsBackWhenMembershipRepairFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRoleHandlerDB(t)
	if err := db.Exec(`
CREATE TABLE iam_team_roles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  team_id TEXT NOT NULL,
  role_id INTEGER NOT NULL,
  scope_type TEXT NOT NULL,
  scope_id INTEGER NOT NULL,
  assigned_by TEXT,
  assigned_at DATETIME,
  expires_at DATETIME,
  reason TEXT
);
INSERT INTO team_members (team_id, user_id, role) VALUES
  ('team-1', 'member-needing-repair', 'MEMBER');
CREATE TRIGGER reject_team_membership_repair
BEFORE INSERT ON user_organizations
BEGIN
  SELECT RAISE(ABORT, 'membership write rejected');
END;`).Error; err != nil {
		t.Fatal(err)
	}

	role := &entity.Role{Name: "rollback-team-role", DisplayName: "Rollback Team Role", IsActive: true, OrgID: 1}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}
	h := NewRoleHandler(db, allowAllPermChecker{})
	r := roleRouterFull(h, true)
	body := `{"role_id":` + itoa(role.ID) + `,"scope_type":"ORGANIZATION","scope_id":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/teams/team-1/roles", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("membership repair failure must fail assignment, got %d %s", w.Code, w.Body.String())
	}

	var roleCount int64
	if err := db.Table("iam_team_roles").
		Where("team_id = ? AND role_id = ?", "team-1", role.ID).
		Count(&roleCount).Error; err != nil {
		t.Fatal(err)
	}
	if roleCount != 0 {
		t.Fatalf("team-role insert must roll back with membership failure, got %d", roleCount)
	}
}
