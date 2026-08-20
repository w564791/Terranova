package service

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

func TestGenerateToken_CleansExpiredActiveTokens(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")

	// Insert two expired-but-still-active tokens occupying quota + names
	past := time.Now().Add(-2 * time.Hour)
	for i, name := range []string{"old-a", "old-b"} {
		hash := base64.StdEncoding.EncodeToString([]byte{byte(i + 1)})
		if err := db.Exec(`
INSERT INTO team_tokens (token_id_hash, team_id, token_name, token_hash, is_active, created_at, expires_at)
VALUES (?,?,?,?,1,?,?)`, hash, "team-1", name, "th-"+name, past.Add(-time.Hour), past).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Without cleanup, active count would be 2 and new generate would fail
	resp, err := svc.GenerateToken(context.Background(), "team-1", "fresh", "user-1", 1)
	if err != nil {
		t.Fatalf("generate after expire cleanup: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected token string")
	}

	var active int64
	if err := db.Model(&models.TeamToken{}).Where("team_id = ? AND is_active = ?", "team-1", true).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("want 1 active after cleanup+generate, got %d", active)
	}

	// same name as expired should be reusable
	if _, err := svc.GenerateToken(context.Background(), "team-1", "old-a", "user-1", 1); err != nil {
		t.Fatalf("reuse expired name: %v", err)
	}
}

func TestGenerateToken_MaxActiveQuota(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	if _, err := svc.GenerateToken(context.Background(), "team-1", "a", "u", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GenerateToken(context.Background(), "team-1", "b", "u", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GenerateToken(context.Background(), "team-1", "c", "u", 1); err == nil {
		t.Fatal("3rd active token must fail quota")
	}
}

func TestValidateToken_InvalidJWT(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	if _, err := svc.ValidateToken(context.Background(), "not-a-jwt"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestValidateToken_WrongType(t *testing.T) {
	db := setupTeamTokenDB(t)
	secret := "test-secret-key-at-least-32-bytes!!"
	svc := NewTeamTokenService(db, secret)

	claims := TeamTokenClaims{
		TeamID:    "team-1",
		TokenID:   "token-t-x",
		TokenType: "login_token",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(context.Background(), signed); err == nil {
		t.Fatal("wrong type must fail")
	}
}

func TestValidateToken_NotInDB(t *testing.T) {
	db := setupTeamTokenDB(t)
	secret := "test-secret-key-at-least-32-bytes!!"
	svc := NewTeamTokenService(db, secret)

	claims := TeamTokenClaims{
		TeamID:    "team-1",
		TokenID:   "token-t-missing",
		TokenType: "team_token",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(secret))
	if _, err := svc.ValidateToken(context.Background(), signed); err == nil {
		t.Fatal("token not in DB must fail")
	}
}

func TestValidateToken_SuccessAndRevokedAndExpired(t *testing.T) {
	db := setupTeamTokenDB(t)
	secret := "test-secret-key-at-least-32-bytes!!"
	svc := NewTeamTokenService(db, secret)

	// 经真实 GenerateToken 路径写入 token_id_hash + token_hash（禁止假列）
	created, err := svc.GenerateToken(context.Background(), "team-1", "v", "user-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ValidateToken(context.Background(), created.Token)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.TeamID != "team-1" || got.TokenType != "team_token" {
		t.Fatalf("claims=%+v", got)
	}

	// revoked
	if err := svc.RevokeTokenByName(context.Background(), "team-1", "v", "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(context.Background(), created.Token); err == nil {
		t.Fatal("revoked must fail")
	}

	// 重新生成后手工过期 DB 行
	created2, err := svc.GenerateToken(context.Background(), "team-1", "v2", "user-1", 1)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Minute)
	if err := db.Exec(`UPDATE team_tokens SET expires_at = ? WHERE is_active = 1 AND token_name = 'v2'`, past).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(context.Background(), created2.Token); err == nil {
		t.Fatal("DB-expired must fail")
	}
}

func TestValidateToken_NullExpiresRejected(t *testing.T) {
	db := setupTeamTokenDB(t)
	secret := "test-secret-key-at-least-32-bytes!!"
	svc := NewTeamTokenService(db, secret)
	created, err := svc.GenerateToken(context.Background(), "team-1", "null-exp", "u", 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`UPDATE team_tokens SET expires_at = NULL WHERE token_name = 'null-exp'`)
	if _, err := svc.ValidateToken(context.Background(), created.Token); err == nil {
		t.Fatal("NULL expires_at must fail")
	}
}

func TestGetTokenByID_NotFound(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	// PK is token_id_hash string; numeric First won't match
	if _, err := svc.GetTokenByID(context.Background(), 999); err == nil {
		t.Fatal("expected not found")
	}
}

func TestRevokeTokenByName_AfterGenerate_AllowsNewSameName(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	if _, err := svc.GenerateToken(context.Background(), "team-1", "reuse", "u1", 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeTokenByName(context.Background(), "team-1", "reuse", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GenerateToken(context.Background(), "team-1", "reuse", "u2", 1); err != nil {
		t.Fatalf("name should be reusable after revoke: %v", err)
	}
}
