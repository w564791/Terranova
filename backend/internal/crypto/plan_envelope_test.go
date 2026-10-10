package crypto

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"
)

func TestPlanDataEnvelope(t *testing.T) {
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "plan-envelope-test-secret")
	}
	plan := append([]byte("PK\x03\x04"), bytes.Repeat([]byte("secret-plan "), 1000)...)
	exp := time.Now().Add(time.Hour)
	a, err := SealPlanData(42, plan, exp)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := SealPlanData(42, plan, exp)
	if bytes.Equal(a, b) || bytes.Contains(a, []byte("secret-plan")) {
		t.Fatal("envelopes must be randomized and opaque")
	}
	if ok, e := IsSealedPlanData(a); !ok || e.Unix() != exp.Unix() {
		t.Fatalf("header: %v %v", ok, e)
	}
	got, err := OpenPlanData(42, a, time.Now())
	if err != nil || !bytes.Equal(got, plan) {
		t.Fatalf("open: %v", err)
	}
	// bound to the task
	if _, err := OpenPlanData(43, a, time.Now()); !errors.Is(err, ErrPlanDataIntegrity) {
		t.Fatalf("other task: %v", err)
	}
	// tampered body / extended expiry
	for _, i := range []int{len(a) - 1, planDataHeaderLen + 3, 6} {
		c := append([]byte(nil), a...)
		c[i] ^= 1
		if _, err := OpenPlanData(42, c, time.Now()); !errors.Is(err, ErrPlanDataIntegrity) {
			t.Fatalf("tamper at %d: %v", i, err)
		}
	}
	// expired
	if _, err := OpenPlanData(42, a, exp.Add(time.Second)); !errors.Is(err, ErrPlanDataExpired) {
		t.Fatalf("expired: %v", err)
	}
	// legacy plaintext
	if ok, _ := IsSealedPlanData(plan); ok {
		t.Fatal("plaintext plan detected as sealed")
	}
	if _, err := OpenPlanData(42, plan, time.Now()); !errors.Is(err, ErrPlanDataNotSealed) {
		t.Fatalf("plaintext: %v", err)
	}
}
