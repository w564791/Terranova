package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"iac-platform/internal/keys"
)

// Envelope encryption of the binary Terraform plan (workspace_tasks.plan_data).
//
// Every plan gets its own random 256-bit data key (DEK). The plan is sealed
// with AES-256-GCM under the DEK; the DEK is wrapped with AES-256-GCM under
// the plan-data key-encryption key (KEK). The KEK is HMAC-SHA256(root,
// "terranova/plan-data/kek/v1"), so it never equals the root used for
// variable values. Both seals authenticate the header (including the key
// version) and the owning task ID as additional data: a blob copied to another
// task, or with an edited expiry or key version, does not open.
//
// Layout, version 2 (current; root = DATA_ENCRYPTION_KEY version key_version):
//
//	"TNPD" | 0x02 | key_version (uint16 BE) | expires_at (int64 unix s, BE)   15 B header
//	| wrap nonce (12) | wrapped DEK (32 + 16 tag)
//	| data nonce (12) | sealed plan (+16 tag)
//
// Version 1 (legacy, key version 0; root = SHA-256(JWT_SECRET)): the same
// without the key_version field (13 B header). Decrypt only once
// DATA_ENCRYPTION_KEY is set; written only in development legacy mode.
// CleanupPlanData re-encrypts version-1 envelopes as version 2.
//
// The key version lives in the authenticated header rather than a separate
// column: plan_data is one bytea with several writers, and SQL can still
// select legacy rows (get_byte(plan_data, 4) = 1).
//
// A Terraform plan file is a zip archive ("PK\x03\x04"), so a legacy
// plaintext row can never be mistaken for an envelope.

var planDataMagic = []byte("TNPD")

const (
	planDataVersion1    = 1
	planDataVersion2    = 2
	planDataHeaderLenV1 = 4 + 1 + 8
	planDataHeaderLenV2 = 4 + 1 + 2 + 8
	// planDataHeaderLen the longest header (prefix length cleanup reads)
	planDataHeaderLen = planDataHeaderLenV2
	gcmNonceLen       = 12
	dekLen            = 32
	wrappedDEKLen     = dekLen + 16
	planDataBodyMin   = gcmNonceLen + wrappedDEKLen + gcmNonceLen + 16
)

var (
	// ErrPlanDataNotSealed the bytes are not a plan-data envelope (legacy
	// plaintext, or garbage).
	ErrPlanDataNotSealed = errors.New("plan_data is not encrypted")
	// ErrPlanDataExpired the envelope's expiry has passed.
	ErrPlanDataExpired = errors.New("plan_data expired")
	// ErrPlanDataIntegrity the envelope does not open (wrong task, tampered,
	// or a different master key).
	ErrPlanDataIntegrity = errors.New("plan_data failed authentication")
)

func planDataKEKFrom(root []byte) []byte {
	m := hmac.New(sha256.New, root)
	m.Write([]byte("terranova/plan-data/kek/v1"))
	return m.Sum(nil)
}

// planDataKEK the KEK of key version kv (0 = legacy JWT_SECRET root).
func planDataKEK(kv int) ([]byte, error) {
	if kv == 0 {
		return planDataKEKFrom(legacyKey()), nil
	}
	k, err := keys.DataKeys.ByVersion(kv)
	if err != nil {
		return nil, err
	}
	return planDataKEKFrom(k.Material), nil
}

// planDataHeader parsed envelope header.
type planDataHeader struct {
	keyVersion int
	expires    time.Time
	len        int
}

func parsePlanDataHeader(b []byte) (planDataHeader, bool) {
	if len(b) < 5 || !bytes.Equal(b[:4], planDataMagic) {
		return planDataHeader{}, false
	}
	switch b[4] {
	case planDataVersion1:
		if len(b) < planDataHeaderLenV1 {
			return planDataHeader{}, false
		}
		return planDataHeader{0, time.Unix(int64(binary.BigEndian.Uint64(b[5:13])), 0), planDataHeaderLenV1}, true
	case planDataVersion2:
		if len(b) < planDataHeaderLenV2 {
			return planDataHeader{}, false
		}
		kv := int(binary.BigEndian.Uint16(b[5:7]))
		if kv < 1 {
			return planDataHeader{}, false
		}
		return planDataHeader{kv, time.Unix(int64(binary.BigEndian.Uint64(b[7:15])), 0), planDataHeaderLenV2}, true
	}
	return planDataHeader{}, false
}

func planDataAAD(header []byte, taskID uint) []byte {
	aad := make([]byte, 0, len(header)+24)
	aad = append(aad, header...)
	aad = append(aad, "task:"...)
	return strconv.AppendUint(aad, uint64(taskID), 10)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// SealPlanData encrypts plan (the plan.out bytes) of task taskID, valid until
// expiresAt.
//
// Sealed under the current DATA_ENCRYPTION_KEY (version 2); in development
// legacy mode (no DATA_ENCRYPTION_KEY) under the legacy root (version 1).
func SealPlanData(taskID uint, plan []byte, expiresAt time.Time) ([]byte, error) {
	var header []byte
	var kekKey []byte
	if keys.LegacyEncryptionMode() {
		header = make([]byte, planDataHeaderLenV1)
		copy(header, planDataMagic)
		header[4] = planDataVersion1
		binary.BigEndian.PutUint64(header[5:], uint64(expiresAt.Unix()))
		kekKey = planDataKEKFrom(legacyKey())
	} else {
		k, err := keys.DataKeys.Current()
		if err != nil {
			return nil, fmt.Errorf("seal plan_data: %w", err)
		}
		header = make([]byte, planDataHeaderLenV2)
		copy(header, planDataMagic)
		header[4] = planDataVersion2
		binary.BigEndian.PutUint16(header[5:7], uint16(k.Version))
		binary.BigEndian.PutUint64(header[7:], uint64(expiresAt.Unix()))
		kekKey = planDataKEKFrom(k.Material)
	}
	aad := planDataAAD(header, taskID)

	dek := make([]byte, dekLen)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, err
	}
	defer clear(dek)
	kek, err := newGCM(kekKey)
	if err != nil {
		return nil, err
	}
	data, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	nonces := make([]byte, 2*gcmNonceLen)
	if _, err := io.ReadFull(rand.Reader, nonces); err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(header)+planDataBodyMin+len(plan))
	out = append(out, header...)
	out = append(out, nonces[:gcmNonceLen]...)
	out = kek.Seal(out, nonces[:gcmNonceLen], dek, aad)
	out = append(out, nonces[gcmNonceLen:]...)
	out = data.Seal(out, nonces[gcmNonceLen:], plan, aad)
	return out, nil
}

// PlanDataHeaderLen bytes of an envelope ParsePlanDataHeader needs (the
// longest header version).
const PlanDataHeaderLen = planDataHeaderLen

// ParsePlanDataHeader reads the envelope header from the first
// PlanDataHeaderLen bytes of a stored plan_data (cleanup jobs read only the
// prefix): whether it is an envelope, and its expiry (unauthenticated).
func ParsePlanDataHeader(prefix []byte) (bool, time.Time) {
	h, ok := parsePlanDataHeader(prefix)
	return ok, h.expires
}

// PlanDataKeyVersion the DATA_ENCRYPTION_KEY version of an envelope (0 =
// legacy JWT_SECRET root); ok false when prefix is not an envelope header.
func PlanDataKeyVersion(prefix []byte) (int, bool) {
	h, ok := parsePlanDataHeader(prefix)
	return h.keyVersion, ok
}

// IsSealedPlanData reports whether blob is a plan-data envelope, and its
// expiry (read from the header; authenticated only by OpenPlanData).
func IsSealedPlanData(blob []byte) (bool, time.Time) {
	h, ok := parsePlanDataHeader(blob)
	if !ok || len(blob) < h.len+planDataBodyMin {
		return false, time.Time{}
	}
	return true, h.expires
}

// OpenPlanData decrypts the envelope of task taskID. Expired envelopes are
// refused (ErrPlanDataExpired) even before cleanup deletes them.
func OpenPlanData(taskID uint, blob []byte, now time.Time) ([]byte, error) {
	plan, expires, err := openPlanData(taskID, blob)
	if err != nil {
		return nil, err
	}
	// checked after authentication: the header expiry is genuine
	if !now.Before(expires) {
		clear(plan)
		return nil, fmt.Errorf("%w at %s", ErrPlanDataExpired, expires.UTC().Format(time.RFC3339))
	}
	return plan, nil
}

// ReencryptPlanData re-seals an envelope of task taskID under the current
// DATA_ENCRYPTION_KEY with the same (authenticated) expiry. Returns
// ErrPlanDataExpired for an expired envelope (cleanup deletes those).
func ReencryptPlanData(taskID uint, blob []byte, now time.Time) ([]byte, error) {
	plan, expires, err := openPlanData(taskID, blob)
	if err != nil {
		return nil, err
	}
	defer clear(plan)
	if !now.Before(expires) {
		return nil, ErrPlanDataExpired
	}
	if keys.LegacyEncryptionMode() {
		return nil, errors.New("re-encrypting plan_data needs DATA_ENCRYPTION_KEY")
	}
	return SealPlanData(taskID, plan, expires)
}

func openPlanData(taskID uint, blob []byte) ([]byte, time.Time, error) {
	if sealed, _ := IsSealedPlanData(blob); !sealed {
		return nil, time.Time{}, ErrPlanDataNotSealed
	}
	h, _ := parsePlanDataHeader(blob)
	expires := h.expires
	header := blob[:h.len]
	aad := planDataAAD(header, taskID)
	rest := blob[h.len:]
	wrapNonce, rest := rest[:gcmNonceLen], rest[gcmNonceLen:]
	wrapped, rest := rest[:wrappedDEKLen], rest[wrappedDEKLen:]
	dataNonce, sealedPlan := rest[:gcmNonceLen], rest[gcmNonceLen:]

	kekKey, err := planDataKEK(h.keyVersion)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("%w: %v", ErrPlanDataIntegrity, err)
	}
	kek, err := newGCM(kekKey)
	if err != nil {
		return nil, time.Time{}, err
	}
	dek, err := kek.Open(nil, wrapNonce, wrapped, aad)
	if err != nil {
		return nil, time.Time{}, ErrPlanDataIntegrity
	}
	defer clear(dek)
	data, err := newGCM(dek)
	if err != nil {
		return nil, time.Time{}, err
	}
	plan, err := data.Open(nil, dataNonce, sealedPlan, aad)
	if err != nil {
		return nil, time.Time{}, ErrPlanDataIntegrity
	}
	return plan, expires, nil
}
