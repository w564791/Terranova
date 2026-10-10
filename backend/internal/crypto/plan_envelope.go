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
)

// Envelope encryption of the binary Terraform plan (workspace_tasks.plan_data).
//
// Every plan gets its own random 256-bit data key (DEK). The plan is sealed
// with AES-256-GCM under the DEK; the DEK is wrapped with AES-256-GCM under
// the plan-data key-encryption key (KEK). The KEK is derived from the
// platform master key of this package (the variable-encryption key, itself
// derived from JWT_SECRET) with HMAC-SHA256 and a purpose label, so the two
// uses never share a key. Both seals authenticate the header and the owning
// task ID as additional data: a blob copied to another task, or with an
// edited expiry, does not open.
//
// Layout (version 1):
//
//	"TNPD" | 0x01 | expires_at (int64 unix seconds, big endian)        13 B header
//	| wrap nonce (12) | wrapped DEK (32 + 16 tag)
//	| data nonce (12) | sealed plan (+16 tag)
//
// A Terraform plan file is a zip archive ("PK\x03\x04"), so a legacy
// plaintext row can never be mistaken for an envelope.

var planDataMagic = []byte("TNPD")

const (
	planDataVersion   = 1
	planDataHeaderLen = 4 + 1 + 8
	gcmNonceLen       = 12
	dekLen            = 32
	wrappedDEKLen     = dekLen + 16
	planDataMinLen    = planDataHeaderLen + gcmNonceLen + wrappedDEKLen + gcmNonceLen + 16
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

func planDataKEK() []byte {
	m := hmac.New(sha256.New, legacyKey())
	m.Write([]byte("terranova/plan-data/kek/v1"))
	return m.Sum(nil)
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
func SealPlanData(taskID uint, plan []byte, expiresAt time.Time) ([]byte, error) {
	header := make([]byte, planDataHeaderLen)
	copy(header, planDataMagic)
	header[4] = planDataVersion
	binary.BigEndian.PutUint64(header[5:], uint64(expiresAt.Unix()))
	aad := planDataAAD(header, taskID)

	dek := make([]byte, dekLen)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, err
	}
	defer clear(dek)
	kek, err := newGCM(planDataKEK())
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
	out := make([]byte, 0, planDataMinLen+len(plan))
	out = append(out, header...)
	out = append(out, nonces[:gcmNonceLen]...)
	out = kek.Seal(out, nonces[:gcmNonceLen], dek, aad)
	out = append(out, nonces[gcmNonceLen:]...)
	out = data.Seal(out, nonces[gcmNonceLen:], plan, aad)
	return out, nil
}

// PlanDataHeaderLen bytes of an envelope ParsePlanDataHeader needs.
const PlanDataHeaderLen = planDataHeaderLen

// ParsePlanDataHeader reads the envelope header from the first
// PlanDataHeaderLen bytes of a stored plan_data (cleanup jobs read only the
// prefix): whether it is an envelope, and its expiry (unauthenticated).
func ParsePlanDataHeader(prefix []byte) (bool, time.Time) {
	if len(prefix) < planDataHeaderLen || !bytes.Equal(prefix[:4], planDataMagic) || prefix[4] != planDataVersion {
		return false, time.Time{}
	}
	return true, time.Unix(int64(binary.BigEndian.Uint64(prefix[5:planDataHeaderLen])), 0)
}

// IsSealedPlanData reports whether blob is a plan-data envelope, and its
// expiry (read from the header; authenticated only by OpenPlanData).
func IsSealedPlanData(blob []byte) (bool, time.Time) {
	if len(blob) < planDataMinLen || !bytes.Equal(blob[:4], planDataMagic) || blob[4] != planDataVersion {
		return false, time.Time{}
	}
	return true, time.Unix(int64(binary.BigEndian.Uint64(blob[5:planDataHeaderLen])), 0)
}

// OpenPlanData decrypts the envelope of task taskID. Expired envelopes are
// refused (ErrPlanDataExpired) even before cleanup deletes them.
func OpenPlanData(taskID uint, blob []byte, now time.Time) ([]byte, error) {
	sealed, expires := IsSealedPlanData(blob)
	if !sealed {
		return nil, ErrPlanDataNotSealed
	}
	header := blob[:planDataHeaderLen]
	aad := planDataAAD(header, taskID)
	rest := blob[planDataHeaderLen:]
	wrapNonce, rest := rest[:gcmNonceLen], rest[gcmNonceLen:]
	wrapped, rest := rest[:wrappedDEKLen], rest[wrappedDEKLen:]
	dataNonce, sealedPlan := rest[:gcmNonceLen], rest[gcmNonceLen:]

	kek, err := newGCM(planDataKEK())
	if err != nil {
		return nil, err
	}
	dek, err := kek.Open(nil, wrapNonce, wrapped, aad)
	if err != nil {
		return nil, ErrPlanDataIntegrity
	}
	defer clear(dek)
	data, err := newGCM(dek)
	if err != nil {
		return nil, err
	}
	plan, err := data.Open(nil, dataNonce, sealedPlan, aad)
	if err != nil {
		return nil, ErrPlanDataIntegrity
	}
	// checked after authentication: the header expiry is genuine
	if !now.Before(expires) {
		return nil, fmt.Errorf("%w at %s", ErrPlanDataExpired, expires.UTC().Format(time.RFC3339))
	}
	return plan, nil
}
