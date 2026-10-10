package models

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"iac-platform/internal/crypto"
)

// A sensitive plaintext that is valid base64 is encrypted by the write hooks
// (it used to be stored as is, taken for a ciphertext) and reads back.
func TestSensitiveBase64PlaintextIsEncryptedByHooks(t *testing.T) {
	k := make([]byte, 32)
	rnd := make([]byte, 48)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(rnd); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "hooks-legacy-secret")
	t.Setenv("DATA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(k))
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "")
	plain := base64.StdEncoding.EncodeToString(rnd)

	wv := WorkspaceVariable{VariableID: "var-x", Value: plain, Sensitive: true}
	if err := wv.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(wv.Value, "tnk1:") || wv.KeyVersion != 1 {
		t.Fatalf("workspace variable stored as %q (key_version %d)", wv.Value, wv.KeyVersion)
	}
	stored := wv.Value
	if err := wv.BeforeSave(nil); err != nil || wv.Value != stored {
		t.Fatalf("second save re-encrypted a ciphertext: %v", err)
	}
	if err := wv.AfterFind(nil); err != nil || wv.Value != plain {
		t.Fatalf("read back %q %v", wv.Value, err)
	}

	vv := VarsetVariable{VariableID: "var-y", Value: plain, Sensitive: true}
	if err := vv.BeforeSave(nil); err != nil {
		t.Fatal(err)
	}
	if !crypto.IsVersionedCiphertext(vv.Value) || vv.KeyVersion != 1 {
		t.Fatalf("varset variable stored as %q", vv.Value)
	}
	if err := vv.AfterFind(nil); err != nil || vv.Value != plain {
		t.Fatalf("varset read back %q %v", vv.Value, err)
	}

	// a key_version 0 row holding base64 plaintext reads as that plaintext
	old := WorkspaceVariable{Value: plain, Sensitive: true, KeyVersion: 0}
	if err := old.AfterFind(nil); err != nil || old.Value != plain {
		t.Fatalf("key_version 0 plaintext read %q %v", old.Value, err)
	}
}
