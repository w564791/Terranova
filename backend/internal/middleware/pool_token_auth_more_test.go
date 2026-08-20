package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPoolTokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`
CREATE TABLE pool_tokens (
  token_hash TEXT PRIMARY KEY,
  token_name TEXT,
  token_type TEXT,
  pool_id TEXT,
  is_active INTEGER,
  expires_at DATETIME,
  last_used_at DATETIME,
  k8s_namespace TEXT DEFAULT 'terraform'
);
CREATE TABLE pool_allowed_workspaces (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  pool_id TEXT,
  workspace_id TEXT,
  status TEXT
);
CREATE TABLE agents (
  agent_id TEXT PRIMARY KEY,
  pool_id TEXT
);`)
	return db
}

func insertPoolToken(t *testing.T, db *gorm.DB, plain, poolID string, active bool, exp *time.Time) {
	t.Helper()
	h := sha256.Sum256([]byte(plain))
	hash := base64.StdEncoding.EncodeToString(h[:])
	activeInt := 0
	if active {
		activeInt = 1
	}
	if err := db.Exec(
		`INSERT INTO pool_tokens (token_hash, token_name, token_type, pool_id, is_active, expires_at) VALUES (?,?,?,?,?,?)`,
		hash, "n", models.PoolTokenTypeStatic, poolID, activeInt, exp,
	).Error; err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatePoolToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	plain := "apt_pool-test_secretvalue"
	insertPoolToken(t, db, plain, "pool-1", true, nil)

	t.Run("ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Request.Header.Set("Authorization", "Bearer "+plain)
		if !authenticatePoolToken(c, db) {
			t.Fatalf("auth failed: %s", w.Body.String())
		}
		if c.GetString("pool_id") != "pool-1" {
			t.Fatal(c.GetString("pool_id"))
		}
	})

	t.Run("missing header", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		if authenticatePoolToken(c, db) {
			t.Fatal("should fail")
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatal(w.Code)
		}
	})

	t.Run("bad format", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Request.Header.Set("Authorization", "Bearer not-apt")
		if authenticatePoolToken(c, db) {
			t.Fatal("should fail")
		}
	})

	t.Run("expired", func(t *testing.T) {
		db2 := setupPoolTokenDB(t)
		past := time.Now().Add(-time.Hour)
		p := "apt_pool-exp_secret"
		insertPoolToken(t, db2, p, "pool-e", true, &past)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Request.Header.Set("Authorization", "Bearer "+p)
		if authenticatePoolToken(c, db2) {
			t.Fatal("expired should fail")
		}
	})
}

func TestPoolTokenAuthWithWorkspaceCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	plain := "apt_pool-ws_secretvalue"
	insertPoolToken(t, db, plain, "pool-w", true, nil)
	_ = db.Exec(`INSERT INTO pool_allowed_workspaces (pool_id, workspace_id, status) VALUES ('pool-w','ws-ok','active')`)
	_ = db.Exec(`INSERT INTO pool_allowed_workspaces (pool_id, workspace_id, status) VALUES ('pool-w','ws-rev','revoked')`)

	mw := PoolTokenAuthWithWorkspaceCheck(db)

	t.Run("allowed workspace", func(t *testing.T) {
		r := gin.New()
		r.GET("/ws/:workspace_id/x", mw, func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/ws/ws-ok/x", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("revoked workspace", func(t *testing.T) {
		r := gin.New()
		r.GET("/ws/:workspace_id/x", mw, func(c *gin.Context) { c.Status(200) })
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/ws/ws-rev/x", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("want 403 got %d", w.Code)
		}
	})
}

func TestPoolTokenAuthWithAgentCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPoolTokenDB(t)
	plain := "apt_pool-agent_secretvalue"
	insertPoolToken(t, db, plain, "pool-a", true, nil)
	if err := db.Exec(`INSERT INTO agents (agent_id, pool_id) VALUES ('agent-a', 'pool-a'), ('agent-b', 'pool-b')`).Error; err != nil {
		t.Fatal(err)
	}

	mw := PoolTokenAuthWithAgentCheck(db)
	r := gin.New()
	r.GET("/agents/:agent_id", mw, func(c *gin.Context) {
		if c.GetString("authorized_agent_id") != "agent-a" {
			t.Fatalf("authorized agent id = %q", c.GetString("authorized_agent_id"))
		}
		c.Status(http.StatusOK)
	})

	t.Run("same pool", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/agents/agent-a", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("want 200 got %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("different pool is concealed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/agents/agent-b", nil)
		req.Header.Set("Authorization", "Bearer "+plain)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("want 404 got %d %s", w.Code, w.Body.String())
		}
	})
}

func TestRespondWithError_WebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("Upgrade", "websocket")
	respondWithError(c, http.StatusUnauthorized, "nope")
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
}
