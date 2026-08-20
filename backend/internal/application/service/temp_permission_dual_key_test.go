package service

import (
	"context"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// dualKeyTempRepo records lookup args and returns configured temp row
type dualKeyTempRepo struct {
	stubPermRepo
	temp       *entity.TaskTemporaryPermission
	lastEmail  string
	lastUserID string
	marked     bool
}

func (d *dualKeyTempRepo) CheckTemporaryPermission(ctx context.Context, taskID uint, email, userID, permType string) (*entity.TaskTemporaryPermission, error) {
	d.lastEmail = email
	d.lastUserID = userID
	if d.temp == nil || d.temp.TaskID != taskID || d.temp.PermissionType != permType {
		return nil, nil
	}
	emailOK := email != "" && d.temp.UserEmail != "" && equalFoldEmail(email, d.temp.UserEmail)
	uidOK := userID != "" && d.temp.UserID != "" && userID == d.temp.UserID
	if !emailOK && !uidOK {
		return nil, nil
	}
	return d.temp, nil
}
func (d *dualKeyTempRepo) MarkTemporaryPermissionUsed(ctx context.Context, id uint) error {
	d.marked = true
	return nil
}

func TestTemporaryPermission_MatchByUserIDOnly(t *testing.T) {
	repo := &dualKeyTempRepo{
		temp: &entity.TaskTemporaryPermission{
			ID: 1, TaskID: 7, UserID: "user-abc", UserEmail: "",
			PermissionType: "APPLY", ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	c := &PermissionCheckerImpl{
		permissionRepo: repo,
		teamRepo:       &stubTeamRepo{},
		projectRepo:    &stubProjectRepo{},
		auditRepo:      &stubAuditRepo{},
		// no email lookup — should still match via user_id
	}
	tid := uint(7)
	res, err := c.CheckPermissionWithTemporary(context.Background(), &CheckPermissionRequest{
		UserID: "user-abc", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelWrite,
	}, &tid)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.Source != "temporary" {
		t.Fatalf("want temp allow by user_id: %+v (lookup email=%q uid=%q)", res, repo.lastEmail, repo.lastUserID)
	}
	if repo.lastUserID != "user-abc" {
		t.Fatalf("expected user_id passed, got %q", repo.lastUserID)
	}
	if !repo.marked {
		t.Fatal("should mark used")
	}
}

func TestTemporaryPermission_MatchByEmail(t *testing.T) {
	repo := &dualKeyTempRepo{
		temp: &entity.TaskTemporaryPermission{
			ID: 2, TaskID: 8, UserEmail: "Bob@Example.COM", UserID: "",
			PermissionType: "APPLY", ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	c := &PermissionCheckerImpl{
		permissionRepo: repo,
		teamRepo:       &stubTeamRepo{},
		projectRepo:    &stubProjectRepo{},
		auditRepo:      &stubAuditRepo{},
		userEmails:     staticEmailLookup{"user-1": "bob@example.com"},
	}
	tid := uint(8)
	res, err := c.CheckPermissionWithTemporary(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelWrite,
	}, &tid)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.Source != "temporary" {
		t.Fatalf("want temp allow by email: %+v", res)
	}
}

func TestTemporaryPermission_NoMatch(t *testing.T) {
	repo := &dualKeyTempRepo{
		temp: &entity.TaskTemporaryPermission{
			ID: 3, TaskID: 9, UserID: "other", UserEmail: "x@y.z",
			PermissionType: "APPLY", ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	c := &PermissionCheckerImpl{
		permissionRepo: repo,
		teamRepo:       &stubTeamRepo{},
		projectRepo:    &stubProjectRepo{},
		auditRepo:      &stubAuditRepo{},
		userEmails:     staticEmailLookup{"user-1": "a@b.c"},
	}
	tid := uint(9)
	res, err := c.CheckPermissionWithTemporary(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelWrite,
	}, &tid)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed {
		t.Fatal("should deny")
	}
}
