package handlers

import (
	"testing"
	"time"
)

func TestParseFlexibleTime(t *testing.T) {
	future := time.Now().UTC().Add(48 * time.Hour)
	cases := []string{
		future.Format(time.RFC3339),
		future.Format("2006-01-02T15:04"),
		future.Format("2006-01-02T15:04:05"),
		future.Format("2006-01-02"),
	}
	for _, s := range cases {
		if _, err := parseFlexibleTime(s); err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
	}
	if _, err := parseFlexibleTime("not-a-time"); err == nil {
		t.Fatal("expected error for invalid time")
	}
	// past must fail
	if _, err := parseFlexibleTime("2020-01-01T00:00:00Z"); err == nil {
		t.Fatal("expected error for past expires_at")
	}
}
