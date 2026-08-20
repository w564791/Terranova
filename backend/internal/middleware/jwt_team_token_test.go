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

func setupTeamTokenAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE team_tokens (
  token_id_hash TEXT PRIMARY KEY,
  team_id TEXT,
  is_active INTEGER,
  expires_at DATETIME,
  revoked_at DATETIME,
  revoked_by TEXT
);`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func signTeamJWT(t *testing.T, secret, teamID, tokenID string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"type":     "team_token",
		"team_id":  teamID,
		"token_id": tokenID,
		"exp":      exp.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func tokenIDHash(tokenID string) string {
	h := sha256.Sum256([]byte(tokenID))
	return base64.StdEncoding.EncodeToString(h[:])
}

func TestJWTAuth_TeamTokenWithoutUserID(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	db := setupTeamTokenAuthDB(t)
	tokenID := "token-t-abc"
	if err := db.Exec(`INSERT INTO team_tokens (token_id_hash, team_id, is_active, expires_at) VALUES (?,?,1,?)`,
		tokenIDHash(tokenID), "team-99", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	signed := signTeamJWT(t, secret, "team-99", tokenID, time.Now().Add(time.Hour))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) {
		if c.GetString("principal_type") != "TEAM" {
			t.Errorf("principal_type=%s", c.GetString("principal_type"))
		}
		if c.GetString("team_id") != "team-99" {
			t.Errorf("team_id=%s", c.GetString("team_id"))
		}
		if c.GetString("user_id") != "team:team-99" {
			t.Errorf("user_id=%s", c.GetString("user_id"))
		}
		if c.GetString("principal_id") != "team-99" {
			t.Errorf("principal_id=%s", c.GetString("principal_id"))
		}
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("team token should auth without user_id: %d %s", w.Code, w.Body.String())
	}
}

func TestJWTAuth_TeamToken_DBExpiredLazyRevoke(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	db := setupTeamTokenAuthDB(t)
	tokenID := "token-t-expired"
	hashStr := tokenIDHash(tokenID)
	// JWT still valid, but DB expires_at in the past
	if err := db.Exec(`INSERT INTO team_tokens (token_id_hash, team_id, is_active, expires_at) VALUES (?,?,1,?)`,
		hashStr, "team-exp", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	signed := signTeamJWT(t, secret, "team-exp", tokenID, time.Now().Add(time.Hour))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expired team token want 401 got %d body=%s", w.Code, w.Body.String())
	}

	// lazy revoke
	var row struct {
		IsActive  int
		RevokedBy string
	}
	if err := db.Raw(`SELECT is_active, revoked_by FROM team_tokens WHERE token_id_hash = ?`, hashStr).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.IsActive != 0 {
		t.Fatalf("expected lazy revoke is_active=0, got %d", row.IsActive)
	}
	if row.RevokedBy != "system:expired" {
		t.Fatalf("revoked_by=%q", row.RevokedBy)
	}
}

func TestJWTAuth_TeamToken_Revoked(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	db := setupTeamTokenAuthDB(t)
	tokenID := "token-t-rev"
	if err := db.Exec(`INSERT INTO team_tokens (token_id_hash, team_id, is_active, expires_at) VALUES (?,?,0,?)`,
		tokenIDHash(tokenID), "team-rev", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	signed := signTeamJWT(t, secret, "team-rev", tokenID, time.Now().Add(time.Hour))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked want 401 got %d", w.Code)
	}
}

func TestJWTAuth_TeamToken_NullExpiresRejected(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	db := setupTeamTokenAuthDB(t)
	tokenID := "token-t-null-exp"
	// is_active but expires_at NULL
	if err := db.Exec(`INSERT INTO team_tokens (token_id_hash, team_id, is_active, expires_at) VALUES (?,?,1,NULL)`,
		tokenIDHash(tokenID), "team-null").Error; err != nil {
		t.Fatal(err)
	}
	SetGlobalDB(db)
	t.Cleanup(func() { SetGlobalDB(nil) })

	signed := signTeamJWT(t, secret, "team-null", tokenID, time.Now().Add(time.Hour))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("NULL expires want 401 got %d %s", w.Code, w.Body.String())
	}
}

func TestJWTAuth_TeamToken_MissingTokenID(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	SetGlobalDB(setupTeamTokenAuthDB(t))
	t.Cleanup(func() { SetGlobalDB(nil) })

	claims := jwt.MapClaims{
		"type":    "team_token",
		"team_id": "team-1",
		// no token_id
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(secret))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token_id want 401 got %d", w.Code)
	}
}

func TestJWTAuth_LoginTokenRequiresUserID(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!!"
	os.Setenv("JWT_SECRET", secret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	claims := jwt.MapClaims{
		"type": "login_token",
		// no user_id
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(secret))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/p", JWTAuth(), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/p", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("login without user_id want 401 got %d", w.Code)
	}
}
