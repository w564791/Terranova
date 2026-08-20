package service

import "testing"

func TestHashAndVerifyAppSecret(t *testing.T) {
	plain := "app_secret_plain_xyz"
	hashed := hashAppSecret(plain)
	if hashed == plain {
		t.Fatal("hash must not equal plaintext")
	}
	if !verifyAppSecret(hashed, plain) {
		t.Fatal("verify hashed secret failed")
	}
	if verifyAppSecret(hashed, "wrong") {
		t.Fatal("wrong secret must fail")
	}
	// legacy plaintext still accepted for migration
	if !verifyAppSecret(plain, plain) {
		t.Fatal("legacy plaintext verify failed")
	}
	if verifyAppSecret(plain, "wrong") {
		t.Fatal("legacy wrong secret must fail")
	}
}
