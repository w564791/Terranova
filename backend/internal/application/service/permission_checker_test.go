package service

import (
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

func TestCalculateEffectiveLevel_NoGrantsIsNone(t *testing.T) {
	c := &PermissionCheckerImpl{}
	if got := c.calculateEffectiveLevel(nil); got != valueobject.PermissionLevelNone {
		t.Fatalf("expected NONE, got %s", got)
	}
	if got := c.calculateEffectiveLevel([]*entity.PermissionGrant{}); got != valueobject.PermissionLevelNone {
		t.Fatalf("expected NONE for empty, got %s", got)
	}
}

func TestCalculateEffectiveLevel_ScopePriority(t *testing.T) {
	c := &PermissionCheckerImpl{}
	grants := []*entity.PermissionGrant{
		{ScopeType: valueobject.ScopeTypeOrganization, PermissionLevel: valueobject.PermissionLevelWrite},
		{ScopeType: valueobject.ScopeTypeWorkspace, PermissionLevel: valueobject.PermissionLevelRead},
	}
	// Workspace 更精确，取 READ 而非 Org WRITE
	if got := c.calculateEffectiveLevel(grants); got != valueobject.PermissionLevelRead {
		t.Fatalf("expected WS READ to win over Org WRITE, got %s", got)
	}
}

func TestCalculateEffectiveLevel_OrgOnlyAllowsWrite(t *testing.T) {
	c := &PermissionCheckerImpl{}
	grants := []*entity.PermissionGrant{
		{ScopeType: valueobject.ScopeTypeOrganization, PermissionLevel: valueobject.PermissionLevelWrite},
	}
	if got := c.calculateEffectiveLevel(grants); got != valueobject.PermissionLevelWrite {
		t.Fatalf("expected Org WRITE, got %s", got)
	}
}

func TestCalculateEffectiveLevel_SameScopeMax(t *testing.T) {
	c := &PermissionCheckerImpl{}
	grants := []*entity.PermissionGrant{
		{ScopeType: valueobject.ScopeTypeWorkspace, PermissionLevel: valueobject.PermissionLevelRead},
		{ScopeType: valueobject.ScopeTypeWorkspace, PermissionLevel: valueobject.PermissionLevelAdmin},
	}
	if got := c.calculateEffectiveLevel(grants); got != valueobject.PermissionLevelAdmin {
		t.Fatalf("expected max ADMIN, got %s", got)
	}
}

func TestCalculateEffectiveLevel_ExpiredIgnored(t *testing.T) {
	c := &PermissionCheckerImpl{}
	past := time.Now().Add(-time.Hour)
	grants := []*entity.PermissionGrant{
		{ScopeType: valueobject.ScopeTypeOrganization, PermissionLevel: valueobject.PermissionLevelAdmin, ExpiresAt: &past},
	}
	if got := c.calculateEffectiveLevel(grants); got != valueobject.PermissionLevelNone {
		t.Fatalf("expected NONE after expiry, got %s", got)
	}
}

func TestCalculateEffectiveLevel_NoneLevelIgnored(t *testing.T) {
	c := &PermissionCheckerImpl{}
	grants := []*entity.PermissionGrant{
		{ScopeType: valueobject.ScopeTypeOrganization, PermissionLevel: valueobject.PermissionLevelNone},
		{ScopeType: valueobject.ScopeTypeOrganization, PermissionLevel: valueobject.PermissionLevelRead},
	}
	if got := c.calculateEffectiveLevel(grants); got != valueobject.PermissionLevelRead {
		t.Fatalf("expected READ (NONE row ignored), got %s", got)
	}
}

func TestGetDenyReason(t *testing.T) {
	c := &PermissionCheckerImpl{}
	if r := c.getDenyReason(valueobject.PermissionLevelNone, valueobject.PermissionLevelRead); r != "No permission" {
		t.Fatalf("unexpected reason: %q", r)
	}
	r := c.getDenyReason(valueobject.PermissionLevelRead, valueobject.PermissionLevelWrite)
	if r == "" || r == "Access explicitly denied" {
		t.Fatalf("unexpected insufficient reason: %q", r)
	}
}

func TestValidateRequest_Fields(t *testing.T) {
	c := &PermissionCheckerImpl{}
	base := &CheckPermissionRequest{
		ResourceType:  valueobject.ResourceTypeModules,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       1,
		RequiredLevel: valueobject.PermissionLevelRead,
	}
	if err := c.validateRequest(base, valueobject.PrincipalTypeUser, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := c.validateRequest(base, valueobject.PrincipalTypeUser, ""); err == nil {
		t.Fatal("empty principal")
	}
	bad := *base
	bad.ScopeID = 0
	if err := c.validateRequest(&bad, valueobject.PrincipalTypeUser, "u1"); err == nil {
		t.Fatal("scope_id 0")
	}
	bad2 := *base
	bad2.RequiredLevel = valueobject.PermissionLevel(99)
	if err := c.validateRequest(&bad2, valueobject.PrincipalTypeUser, "u1"); err == nil {
		t.Fatal("invalid level")
	}
}

func TestIsAllowedSemantics(t *testing.T) {
	// 与 CheckPermission 中 isAllowed 公式对齐
	cases := []struct {
		effective, required valueobject.PermissionLevel
		want                bool
	}{
		{valueobject.PermissionLevelNone, valueobject.PermissionLevelRead, false},
		{valueobject.PermissionLevelRead, valueobject.PermissionLevelRead, true},
		{valueobject.PermissionLevelWrite, valueobject.PermissionLevelRead, true},
		{valueobject.PermissionLevelRead, valueobject.PermissionLevelWrite, false},
	}
	for _, tc := range cases {
		got := tc.effective >= tc.required && tc.effective != valueobject.PermissionLevelNone
		if got != tc.want {
			t.Fatalf("eff=%s req=%s: got %v want %v", tc.effective, tc.required, got, tc.want)
		}
	}
}
