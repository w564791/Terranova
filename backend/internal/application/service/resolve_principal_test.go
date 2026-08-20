package service

import (
	"context"
	"testing"

	"iac-platform/internal/domain/valueobject"
)

func TestResolvePrincipal_User(t *testing.T) {
	c := newTestChecker(t, nil, []string{"t1"}, nil)
	pt, id, teams, err := c.resolvePrincipal(context.Background(), &CheckPermissionRequest{
		UserID: "user-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pt != valueobject.PrincipalTypeUser || id != "user-1" || len(teams) != 1 || teams[0] != "t1" {
		t.Fatalf("got %s %s %v", pt, id, teams)
	}
}

func TestResolvePrincipal_TeamFromPrefix(t *testing.T) {
	c := newTestChecker(t, nil, nil, nil)
	pt, id, teams, err := c.resolvePrincipal(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeTeam,
		UserID:        "team:team-x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pt != valueobject.PrincipalTypeTeam || id != "team-x" || len(teams) != 1 {
		t.Fatalf("got %s %s %v", pt, id, teams)
	}
}

func TestResolvePrincipal_UserRejectsTeamPrefix(t *testing.T) {
	c := newTestChecker(t, nil, nil, nil)
	_, _, _, err := c.resolvePrincipal(context.Background(), &CheckPermissionRequest{
		UserID: "team:evil",
	})
	if err == nil {
		t.Fatal("USER with team: prefix must fail")
	}
}

func TestResolvePrincipal_Application(t *testing.T) {
	c := newTestChecker(t, nil, nil, nil)
	pt, id, teams, err := c.resolvePrincipal(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeApplication,
		PrincipalID:   "app-9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pt != valueobject.PrincipalTypeApplication || id != "app-9" || teams != nil {
		t.Fatalf("got %s %s %v", pt, id, teams)
	}
}
