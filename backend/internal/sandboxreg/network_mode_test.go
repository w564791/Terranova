package sandboxreg

import (
	"testing"
)

func TestValidateNetworkModeForAttach_ProductionRequiresVPC(t *testing.T) {
	t.Setenv("ENV", "production")
	for _, mode := range []string{"", NetworkModePublic, "public", "bogus", "vpc-only"} {
		err := ValidateNetworkModeForAttach(mode)
		if err == nil {
			t.Fatalf("expected error for %q", mode)
		}
		ve, ok := err.(*ValidationError)
		if !ok || ve.Code != ErrNetworkModeNotAllowed {
			t.Fatalf("%q: got %#v", mode, err)
		}
	}
	if err := ValidateNetworkModeForAttach(NetworkModeVPC); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNetworkModeForAttach("vpc"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateNetworkModeForAttach_NonProductionAllowsEmptyPublicVPC(t *testing.T) {
	t.Setenv("ENV", "development")
	for _, mode := range []string{"", NetworkModePublic, "public", NetworkModeVPC} {
		if err := ValidateNetworkModeForAttach(mode); err != nil {
			t.Fatalf("%q: %v", mode, err)
		}
	}
	t.Setenv("ENV", "")
	if err := ValidateNetworkModeForAttach(""); err != nil {
		t.Fatal(err)
	}
	err := ValidateNetworkModeForAttach("weird")
	if err == nil {
		t.Fatal("expected invalid for unknown mode in non-prod")
	}
	ve, ok := err.(*ValidationError)
	if !ok || ve.Code != ErrInvalidNetworkMode {
		t.Fatalf("got %#v", err)
	}
}

func TestParseNetworkModeForCreate(t *testing.T) {
	got, err := ParseNetworkModeForCreate("")
	if err != nil || got != "" {
		t.Fatalf("empty: %q %v", got, err)
	}
	got, err = ParseNetworkModeForCreate(" vpc ")
	if err != nil || got != NetworkModeVPC {
		t.Fatalf("vpc: %q %v", got, err)
	}
	got, err = ParseNetworkModeForCreate("PUBLIC")
	if err != nil || got != NetworkModePublic {
		t.Fatalf("public: %q %v", got, err)
	}
	_, err = ParseNetworkModeForCreate("PRIVATE")
	if err == nil {
		t.Fatal("expected invalid")
	}
	ve, ok := err.(*ValidationError)
	if !ok || ve.Code != ErrInvalidNetworkMode {
		t.Fatalf("got %#v", err)
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
