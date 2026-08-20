package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func hashAppSecretForTest(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func setupAgentAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`
CREATE TABLE applications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id INTEGER,
  name TEXT,
  app_key TEXT,
  app_secret TEXT,
  is_active INTEGER,
  expires_at DATETIME,
  last_used_at DATETIME,
  workspace_tag_filter TEXT
);
CREATE TABLE agents (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  agent_id TEXT,
  application_id INTEGER,
  name TEXT,
  status TEXT
);`)
	plain := "secret-plain"
	_ = db.Exec(`INSERT INTO applications (id, org_id, name, app_key, app_secret, is_active)
		VALUES (1, 1, 'app', 'app_key_1', ?, 1)`, hashAppSecretForTest(plain))
	_ = db.Exec(`INSERT INTO agents (agent_id, application_id, name, status) VALUES ('agent-1', 1, 'a', 'idle')`)
	_ = db.Exec(`INSERT INTO agents (agent_id, application_id, name, status) VALUES ('agent-other', 99, 'b', 'idle')`)
	return db
}

func TestAgentAuthMiddleware_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAgentAuthDB(t)
	r := gin.New()
	r.GET("/x", AgentAuthMiddleware(db), func(c *gin.Context) {
		if c.GetUint("application_id") != 1 {
			t.Errorf("app id %v", c.GetUint("application_id"))
		}
		// IAM principal chain (B-1)
		if c.GetString("principal_type") != "APPLICATION" {
			t.Errorf("principal_type=%s", c.GetString("principal_type"))
		}
		if c.GetString("principal_id") != "app_key_1" {
			t.Errorf("principal_id=%s", c.GetString("principal_id"))
		}
		if c.GetString("user_id") != "app:app_key_1" {
			t.Errorf("user_id=%s", c.GetString("user_id"))
		}
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-App-Key", "app_key_1")
	req.Header.Set("X-App-Secret", "secret-plain")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
}

func TestAgentAuthMiddleware_WithAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAgentAuthDB(t)
	r := gin.New()
	r.GET("/agents/:agent_id", AgentAuthMiddleware(db), func(c *gin.Context) {
		if c.GetString("agent_id") != "agent-1" {
			t.Errorf("agent %s", c.GetString("agent_id"))
		}
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents/agent-1", nil)
	req.Header.Set("X-App-Key", "app_key_1")
	req.Header.Set("X-App-Secret", "secret-plain")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
	}
}

func TestAgentAuthMiddleware_AgentWrongApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAgentAuthDB(t)
	r := gin.New()
	r.GET("/agents/:agent_id", AgentAuthMiddleware(db), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents/agent-other", nil)
	req.Header.Set("X-App-Key", "app_key_1")
	req.Header.Set("X-App-Secret", "secret-plain")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d %s", w.Code, w.Body.String())
	}
}

func TestAgentAuthMiddleware_MissingCreds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAgentAuthDB(t)
	r := gin.New()
	r.GET("/x", AgentAuthMiddleware(db), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestAgentAuthMiddleware_BadSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAgentAuthDB(t)
	r := gin.New()
	r.GET("/x", AgentAuthMiddleware(db), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-App-Key", "app_key_1")
	req.Header.Set("X-App-Secret", "wrong")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestAgentAuthMiddleware_ExpiredApp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAgentAuthDB(t)
	past := time.Now().Add(-time.Hour)
	_ = db.Exec(`UPDATE applications SET expires_at = ? WHERE id = 1`, past)
	r := gin.New()
	r.GET("/x", AgentAuthMiddleware(db), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-App-Key", "app_key_1")
	req.Header.Set("X-App-Secret", "secret-plain")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expired want 401 got %d %s", w.Code, w.Body.String())
	}
}

func TestAgentWorkspaceAuthMiddleware_Deprecated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", AgentWorkspaceAuthMiddleware(nil), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusGone {
		t.Fatalf("want 410 got %d", w.Code)
	}
}
