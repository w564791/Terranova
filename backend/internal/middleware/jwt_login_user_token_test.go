package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`
CREATE TABLE users (
  user_id TEXT PRIMARY KEY,
  is_active INTEGER,
  is_system_admin INTEGER
);
CREATE TABLE login_sessions (
  session_id TEXT PRIMARY KEY,
  user_id TEXT,
  is_active INTEGER,
  expires_at DATETIME,
  last_used_at DATETIME
);
CREATE TABLE user_tokens (
  token_id_hash TEXT PRIMARY KEY,
  user_id TEXT,
  is_active INTEGER
);`)
	return db
}

func signMap(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestJWTAuth_LoginTokenOK(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	db := setupAuthDB(t)
	_ = db.Exec(`INSERT INTO users (user_id, is_active, is_system_admin) VALUES ('user-1', 1, 0)`)
	_ = db.Exec(`INSERT INTO login_sessions (session_id, user_id, is_active, expires_at) VALUES ('sess-1', 'user-1', 1, ?)`,
		time.Now().Add(time.Hour))
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	token := signMap(t, secret, jwt.MapClaims{
		"type": "login_token", "user_id": "user-1", "session_id": "sess-1",
		"username": "alice", "exp": time.Now().Add(time.Hour).Unix(),
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) {
		if c.GetString("user_id") != "user-1" {
			t.Errorf("user_id=%s", c.GetString("user_id"))
		}
		if c.GetString("principal_type") != "USER" {
			t.Errorf("principal=%s", c.GetString("principal_type"))
		}
		if c.GetString("session_id") != "sess-1" {
			t.Errorf("session=%s", c.GetString("session_id"))
		}
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestJWTAuth_LoginToken_ExpiredSession(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })
	db := setupAuthDB(t)
	_ = db.Exec(`INSERT INTO users (user_id, is_active, is_system_admin) VALUES ('user-1', 1, 0)`)
	_ = db.Exec(`INSERT INTO login_sessions (session_id, user_id, is_active, expires_at) VALUES ('sess-e', 'user-1', 1, ?)`,
		time.Now().Add(-time.Hour))
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	token := signMap(t, secret, jwt.MapClaims{
		"type": "login_token", "user_id": "user-1", "session_id": "sess-e",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

func TestJWTAuth_LoginToken_InactiveUser(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })
	db := setupAuthDB(t)
	_ = db.Exec(`INSERT INTO users (user_id, is_active, is_system_admin) VALUES ('user-x', 0, 0)`)
	_ = db.Exec(`INSERT INTO login_sessions (session_id, user_id, is_active, expires_at) VALUES ('sess-x', 'user-x', 1, ?)`,
		time.Now().Add(time.Hour))
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	token := signMap(t, secret, jwt.MapClaims{
		"type": "login_token", "user_id": "user-x", "session_id": "sess-x",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d %s", w.Code, w.Body.String())
	}
}

func TestJWTAuth_UserToken_RequiresActiveSession(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })
	db := setupAuthDB(t)
	_ = db.Exec(`INSERT INTO users (user_id, is_active, is_system_admin) VALUES ('user-1', 1, 0)`)
	tokenID := "utok-abc"
	h := sha256.Sum256([]byte(tokenID))
	hashStr := base64.StdEncoding.EncodeToString(h[:])
	_ = db.Exec(`INSERT INTO user_tokens (token_id_hash, user_id, is_active) VALUES (?, 'user-1', 1)`, hashStr)
	// no active login session
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	token := signMap(t, secret, jwt.MapClaims{
		"type": "user_token", "user_id": "user-1", "token_id": tokenID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("user token without login session want 401 got %d %s", w.Code, w.Body.String())
	}
}

func TestJWTAuth_UserToken_OK(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })
	db := setupAuthDB(t)
	_ = db.Exec(`INSERT INTO users (user_id, is_active, is_system_admin) VALUES ('user-1', 1, 1)`)
	_ = db.Exec(`INSERT INTO login_sessions (session_id, user_id, is_active, expires_at) VALUES ('sess-live', 'user-1', 1, ?)`,
		time.Now().Add(time.Hour))
	tokenID := "utok-ok"
	h := sha256.Sum256([]byte(tokenID))
	hashStr := base64.StdEncoding.EncodeToString(h[:])
	_ = db.Exec(`INSERT INTO user_tokens (token_id_hash, user_id, is_active) VALUES (?, 'user-1', 1)`, hashStr)
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	token := signMap(t, secret, jwt.MapClaims{
		"type": "user_token", "user_id": "user-1", "token_id": tokenID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) {
		if c.GetBool("is_system_admin") != true {
			// GetBool may not work for interface{} true
			v, _ := c.Get("is_system_admin")
			if v != true {
				t.Errorf("is_system_admin=%v", v)
			}
		}
		c.Status(200)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}

func TestJWTAuth_MissingAuth(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-jwt-secret-at-least-32-bytes-long!!")
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/p", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
}

func TestJWTAuth_InvalidType(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })
	token := signMap(t, secret, jwt.MapClaims{
		"type": "weird", "user_id": "u1", "exp": time.Now().Add(time.Hour).Unix(),
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}
