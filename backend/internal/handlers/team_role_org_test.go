package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAssignTeamRole_RejectsForeignTeam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:team_role_org?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE teams (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_id TEXT UNIQUE,
		org_id INTEGER,
		name TEXT
	)`)
	_ = db.Exec(`CREATE TABLE iam_roles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		org_id INTEGER DEFAULT 0,
		name TEXT,
		display_name TEXT,
		description TEXT,
		is_system INTEGER DEFAULT 0,
		is_active INTEGER DEFAULT 1,
		created_by TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	_ = db.Exec(`CREATE TABLE iam_team_roles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		team_id TEXT,
		role_id INTEGER,
		scope_type TEXT,
		scope_id INTEGER,
		assigned_by TEXT,
		assigned_at DATETIME,
		expires_at DATETIME,
		reason TEXT
	)`)
	now := time.Now()
	_ = db.Exec(`INSERT INTO teams (team_id, org_id, name) VALUES ('team-b', 2, 'Team B')`)
	_ = db.Exec(`INSERT INTO iam_roles (id, org_id, name, display_name, is_system, is_active, created_at, updated_at)
		VALUES (1, 0, 'viewer', 'Viewer', 1, 1, ?, ?)`, now, now)

	// Unit: org helpers
	teamOrg, err := loadTeamOrgID(t.Context(), db, "team-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureTeamBelongsToAuthOrg(teamOrg, 1); err == nil {
		t.Fatal("team of org2 must not belong to auth org1")
	}
	if err := ensureTeamBelongsToAuthOrg(teamOrg, 2); err != nil {
		t.Fatal(err)
	}

	// HTTP: foreign team → 404 before guard
	h := NewRoleHandler(db, nil)
	r := gin.New()
	r.POST("/iam/teams/:id/roles", func(c *gin.Context) {
		c.Set("user_id", "user-1")
		c.Set("auth_org_id", uint(1))
		h.AssignTeamRole(c)
	})
	body, _ := json.Marshal(map[string]interface{}{
		"role_id":    1,
		"scope_type": "ORGANIZATION",
		"scope_id":   1,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/iam/teams/team-b/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404 for foreign team, got %d body=%s", w.Code, w.Body.String())
	}
}
