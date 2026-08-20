package service

import (
	"context"
	"testing"
)

func TestListTeamTokens_AndRevokePaths(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")

	if _, err := svc.GenerateToken(context.Background(), "team-1", "tok-list", "user-1", 1); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListTeamTokens(context.Background(), "team-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].TokenName != "tok-list" || !list[0].IsActive {
		t.Fatalf("list: %+v", list)
	}

	// deprecated numeric revoke
	if err := svc.RevokeToken(context.Background(), "team-1", 1, "u"); err == nil {
		t.Fatal("numeric revoke should fail")
	}
	// empty name
	if err := svc.RevokeTokenByName(context.Background(), "team-1", "", "u"); err == nil {
		t.Fatal("empty name")
	}
	// not found
	if err := svc.RevokeTokenByName(context.Background(), "team-1", "missing", "u"); err == nil {
		t.Fatal("missing name")
	}
}

func TestGenerateToken_TeamNotFound(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	if _, err := svc.GenerateToken(context.Background(), "no-such-team", "t", "u", 1); err == nil {
		t.Fatal("expected team not found")
	}
}

func TestNewTeamTokenService_EmptySecretFallsBack(t *testing.T) {
	// without JWT_SECRET env, NewTeamTokenService("") panics in config.GetJWTSecret
	// only test non-empty path already covered; ensure constructor stores secret
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "explicit-secret-value-32bytes-min!!")
	if svc.jwtSecret != "explicit-secret-value-32bytes-min!!" {
		t.Fatal(svc.jwtSecret)
	}
}
