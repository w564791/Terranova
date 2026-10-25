package sandboxreg

import (
	"testing"
)

func TestValidateNetworkModeForAttach_ProductionRejectsPublic(t *testing.T) {
	t.Setenv("ENV", "production")
	err := ValidateNetworkModeForAttach(NetworkModePublic)
	if err == nil {
		t.Fatal("expected error")
	}
	ve, ok := err.(*ValidationError)
	if !ok || ve.Code != ErrNetworkModeNotAllowed {
		t.Fatalf("got %#v", err)
	}
	if err := ValidateNetworkModeForAttach(NetworkModeVPC); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNetworkModeForAttach(""); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNetworkModeForAttach_NonProductionAllowsPublic(t *testing.T) {
	t.Setenv("ENV", "development")
	if err := ValidateNetworkModeForAttach(NetworkModePublic); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "")
	if err := ValidateNetworkModeForAttach("public"); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeNetworkMode(t *testing.T) {
	if got := NormalizeNetworkMode(" public "); got != NetworkModePublic {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeNetworkMode("vpc"); got != NetworkModeVPC {
		t.Fatalf("got %q", got)
	}
}
