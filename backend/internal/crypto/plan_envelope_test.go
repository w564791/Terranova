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

func TestPlanEnvelope_DataKeyVersioned(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "jwt-a")
	old := dataKey(t)
	t.Setenv("DATA_ENCRYPTION_KEY", old)
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "1")
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS", "")
	plan := []byte("PK\x03\x04plan")
	exp := time.Now().Add(time.Hour)
	blob, err := SealPlanData(7, plan, exp)
	if err != nil {
		t.Fatal(err)
	}
	if kv, ok := PlanDataKeyVersion(blob); !ok || kv != 1 {
		t.Fatalf("key version %d %v", kv, ok)
	}
	// JWT_SECRET is irrelevant
	t.Setenv("JWT_SECRET", "jwt-b")
	if got, err := OpenPlanData(7, blob, time.Now()); err != nil || string(got) != string(plan) {
		t.Fatalf("open after JWT_SECRET change: %v", err)
	}
	// header key version is authenticated
	tampered := append([]byte(nil), blob...)
	tampered[6] = 2
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS", dataKey(t))
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS_VERSION", "2")
	if _, err := OpenPlanData(7, tampered, time.Now()); err == nil {
		t.Fatal("tampered key version opened")
	}
	// rotation: v1 kept as previous still opens
	t.Setenv("DATA_ENCRYPTION_KEY", dataKey(t))
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "3")
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS", old)
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS_VERSION", "1")
	if _, err := OpenPlanData(7, blob, time.Now()); err != nil {
		t.Fatalf("previous key: %v", err)
	}
	re, err := ReencryptPlanData(7, blob, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if kv, _ := PlanDataKeyVersion(re); kv != 3 {
		t.Fatalf("re-encrypted to %d", kv)
	}
	if _, e := ParsePlanDataHeader(re); e.Unix() != exp.Unix() {
		t.Fatal("expiry not kept")
	}
}

func TestPlanEnvelope_LegacyV1OpensAndReencrypts(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("JWT_SECRET", "jwt-legacy")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	plan := []byte("PK\x03\x04legacy")
	blob, err := SealPlanData(9, plan, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if kv, ok := PlanDataKeyVersion(blob); !ok || kv != 0 || blob[4] != 1 {
		t.Fatalf("legacy envelope: kv=%d ok=%v ver=%d", kv, ok, blob[4])
	}
	t.Setenv("ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", dataKey(t))
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "")
	t.Setenv("DATA_ENCRYPTION_KEY_PREVIOUS", "")
	if got, err := OpenPlanData(9, blob, time.Now()); err != nil || string(got) != string(plan) {
		t.Fatalf("legacy open: %v", err)
	}
	if _, err := ReencryptPlanData(9, blob, time.Now().Add(2*time.Hour)); !errors.Is(err, ErrPlanDataExpired) {
		t.Fatalf("expired re-encrypt: %v", err)
	}
	re, err := ReencryptPlanData(9, blob, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JWT_SECRET", "")
	if got, err := OpenPlanData(9, re, time.Now()); err != nil || string(got) != string(plan) {
		t.Fatalf("re-encrypted open without JWT_SECRET: %v", err)
	}
}
