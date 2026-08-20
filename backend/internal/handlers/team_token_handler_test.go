package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/internal/application/service"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTeamTokenHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE teams (team_id TEXT PRIMARY KEY, name TEXT, org_id INTEGER);`)
	_ = db.Exec(`INSERT INTO teams (team_id, name, org_id) VALUES ('team-1', 't1', 1);`)
	_ = db.Exec(`
CREATE TABLE team_tokens (
  token_id_hash TEXT PRIMARY KEY,
  team_id TEXT,
  token_name TEXT,
  token_hash TEXT,
  is_active INTEGER,
  created_at DATETIME,
  created_by TEXT,
  expires_at DATETIME,
  revoked_at DATETIME,
  revoked_by TEXT,
  last_used_at DATETIME
);`)
	return db
}

func teamTokenRouter(h *TeamTokenHandler, withUser bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if withUser {
			c.Set("user_id", "user-1")
			c.Set("auth_org_id", uint(1))
		}
		c.Next()
	})
	r.POST("/teams/:id/tokens", h.CreateTeamToken)
	r.GET("/teams/:id/tokens", h.ListTeamTokens)
	r.DELETE("/teams/:id/tokens/:token_id", h.RevokeTeamToken)
	return r
}

func TestTeamTokenHandler_CreateListRevoke(t *testing.T) {
	db := setupTeamTokenHandlerDB(t)
	svc := service.NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	h := NewTeamTokenHandlerWithDB(svc, db)
	r := teamTokenRouter(h, true)

	// create
	body := `{"token_name":"ci-token","expires_in_days":1}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/teams/team-1/tokens", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	tok, _ := created["token"].(map[string]interface{})
	if tok == nil || tok["token"] == "" {
		// response embeds TeamTokenCreateResponse under "token"
		if created["token"] == nil {
			t.Fatalf("missing token in response: %s", w.Body.String())
		}
	}

	// list
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/teams/team-1/tokens", nil))
	if w2.Code != 200 {
		t.Fatalf("list: %d %s", w2.Code, w2.Body.String())
	}

	// revoke by name
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest("DELETE", "/teams/team-1/tokens/ci-token", nil))
	if w3.Code != 200 {
		t.Fatalf("revoke: %d %s", w3.Code, w3.Body.String())
	}

	// second revoke fails
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, httptest.NewRequest("DELETE", "/teams/team-1/tokens/ci-token", nil))
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("second revoke want 400 got %d", w4.Code)
	}
}

func TestTeamTokenHandler_CreateUnauthenticated(t *testing.T) {
	db := setupTeamTokenHandlerDB(t)
	h := NewTeamTokenHandlerWithDB(service.NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!"), db)
	r := teamTokenRouter(h, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/teams/team-1/tokens", bytes.NewBufferString(`{"token_name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestTeamTokenHandler_CreateDuplicateName(t *testing.T) {
	db := setupTeamTokenHandlerDB(t)
	h := NewTeamTokenHandlerWithDB(service.NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!"), db)
	r := teamTokenRouter(h, true)
	body := `{"token_name":"same"}`
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/teams/team-1/tokens", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if i == 0 && w.Code != http.StatusCreated {
			t.Fatalf("first: %d %s", w.Code, w.Body.String())
		}
		if i == 1 && w.Code != http.StatusBadRequest {
			t.Fatalf("dup want 400 got %d %s", w.Code, w.Body.String())
		}
	}
}

func TestTeamTokenHandler_RevokeUnauthenticated(t *testing.T) {
	db := setupTeamTokenHandlerDB(t)
	h := NewTeamTokenHandlerWithDB(service.NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!"), db)
	r := teamTokenRouter(h, false)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("DELETE", "/teams/team-1/tokens/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}
