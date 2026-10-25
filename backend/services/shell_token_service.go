package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"iac-platform/internal/keys"
	"iac-platform/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// Shell tokens (docs/manifest/manifest-sandbox-spec.md §10.2).
//
// A shell token is the platform credential of a UI Shell session on a
// shell_debug sandbox: HS256 under the "shell" signing purpose (kid shell-v<n>,
// no legacy / JWT_SECRET scheme), claims typ=shell, shell_session_id, sandbox_id,
// org_id, workspace_id. Its SHA-256 is stored in shell_tokens. It only opens the
// HTTP state backend of the session's workspace, read-only (no lock / apply).
//
// Lifetime: min(now+MaxShellSessionTTL, session.expires_at). Hard cap 15m.
// Revocation: EndShellSession / RevokeShellTokensForSandbox in the same
// transaction that closes the session (no orphan tokens).

const (
	// StateTokenTypeShell the typ claim of shell session tokens.
	StateTokenTypeShell = "shell"

	// MaxShellSessionTTL hard cap for shell sessions and their tokens (§10.2).
	MaxShellSessionTTL = 15 * time.Minute
)

var (
	// ErrShellTokenInvalid signature, typ, expiry or claims wrong.
	ErrShellTokenInvalid = errors.New("invalid shell token")
	// ErrShellTokenRevoked revoked / expired row or shell session closed.
	ErrShellTokenRevoked = errors.New("shell token revoked or session not active")
	// ErrShellTokenUnavailable the database check failed: refused.
	ErrShellTokenUnavailable = errors.New("shell token could not be verified")
	// ErrShellSessionNotActive the shell session cannot receive a token.
	ErrShellSessionNotActive = errors.New("shell session is not active")
)

// ShellTokenClaims the claims of a typ=shell JWT.
type ShellTokenClaims struct {
	Type           string `json:"typ"`
	ShellSessionID string `json:"shell_session_id"`
	SandboxID      string `json:"sandbox_id"`
	OrgID          uint   `json:"org_id"`
	WorkspaceID    string `json:"workspace_id"`
	jwt.RegisteredClaims
}

// IssueShellToken mints a typ=shell JWT for an active shell session and stores
// its hash. exp is capped at MaxShellSessionTTL and session.ExpiresAt.
// Call inside the same DB transaction as session create when possible (pass tx).
func (s *StateTokenService) IssueShellToken(ctx context.Context, tx *gorm.DB, sess *models.ShellSession, sandbox *models.Sandbox) (string, time.Time, error) {
	if sess == nil || sandbox == nil {
		return "", time.Time{}, ErrShellSessionNotActive
	}
	if sess.Status != models.ShellSessionStatusActive || sess.ClosedAt != nil {
		return "", time.Time{}, ErrShellSessionNotActive
	}
	db := s.db.WithContext(ctx)
	if tx != nil {
		db = tx.WithContext(ctx)
	}
	now := time.Now()
	exp := now.Add(MaxShellSessionTTL)
	if sess.ExpiresAt.Before(exp) {
		exp = sess.ExpiresAt
	}
	if !exp.After(now) {
		return "", time.Time{}, ErrShellSessionNotActive
	}
	jti := make([]byte, 12)
	if _, err := rand.Read(jti); err != nil {
		return "", time.Time{}, err
	}
	claims := ShellTokenClaims{
		Type:           StateTokenTypeShell,
		ShellSessionID: sess.ID,
		SandboxID:      sandbox.ID,
		OrgID:          sess.OrgID,
		WorkspaceID:    sandbox.WorkspaceID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "shell:" + sess.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        hex.EncodeToString(jti),
		},
	}
	tok, err := keys.Sign(keys.PurposeShell, claims, nil) // no legacy scheme
	if err != nil {
		return "", time.Time{}, err
	}
	row := models.ShellToken{
		ShellSessionID: sess.ID,
		SandboxID:      sandbox.ID,
		OrgID:          sess.OrgID,
		WorkspaceID:    sandbox.WorkspaceID,
		TokenHash:      sha256Hash(tok),
		ExpiresAt:      exp,
	}
	if err := db.Create(&row).Error; err != nil {
		return "", time.Time{}, fmt.Errorf("store shell token: %w", err)
	}
	return tok, exp, nil
}

// RevokeShellTokensForSession sets revoked_at on every active token of the session.
func RevokeShellTokensForSession(ctx context.Context, db *gorm.DB, shellSessionID string) error {
	return db.WithContext(ctx).Model(&models.ShellToken{}).
		Where("shell_session_id = ? AND revoked_at IS NULL", shellSessionID).
		Update("revoked_at", time.Now()).Error
}

// RevokeShellTokensForSandbox revokes every active shell token of a sandbox
// (unbind / DELETE sandbox).
func RevokeShellTokensForSandbox(ctx context.Context, db *gorm.DB, sandboxID string) error {
	return db.WithContext(ctx).Model(&models.ShellToken{}).
		Where("sandbox_id = ? AND revoked_at IS NULL", sandboxID).
		Update("revoked_at", time.Now()).Error
}

// EndShellSession revokes typ=shell tokens then marks the session closed in one
// transaction. Idempotent if already closed.
func EndShellSession(ctx context.Context, db *gorm.DB, shellSessionID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sess models.ShellSession
		if err := tx.Where("id = ?", shellSessionID).Take(&sess).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := RevokeShellTokensForSession(ctx, tx, shellSessionID); err != nil {
			return err
		}
		if sess.ClosedAt == nil {
			if err := tx.Model(&models.ShellSession{}).Where("id = ? AND closed_at IS NULL", shellSessionID).
				Updates(map[string]interface{}{
					"status":     models.ShellSessionStatusClosed,
					"closed_at":  now,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
		// TODO(step-6): revoke STS AssumeRole credentials bound to this session.
		return nil
	})
}

type shellTokenRow struct {
	ShellSessionID string
	SandboxID      string
	OrgID          uint
	WorkspaceID    string
	ExpiresAt      time.Time
	RevokedAt      *time.Time
	SessStatus     string
	SessClosedAt   *time.Time
	SessExpiresAt  time.Time
}

func (s *StateTokenService) validateShellToken(ctx context.Context, tok string) (*StateCaller, error) {
	claims := &ShellTokenClaims{}
	t, err := jwt.ParseWithClaims(tok, claims, keys.Keyfunc(keys.PurposeShell, nil),
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !t.Valid || claims.Type != StateTokenTypeShell ||
		claims.ShellSessionID == "" || claims.WorkspaceID == "" || claims.SandboxID == "" {
		return nil, ErrShellTokenInvalid
	}
	var row shellTokenRow
	err = s.db.WithContext(ctx).Table("shell_tokens AS st").
		Select(`st.shell_session_id, st.sandbox_id, st.org_id, st.workspace_id, st.expires_at, st.revoked_at,
			ss.status AS sess_status, ss.closed_at AS sess_closed_at, ss.expires_at AS sess_expires_at`).
		Joins("JOIN shell_sessions ss ON ss.id = st.shell_session_id").
		Where("st.token_hash = ?", sha256Hash(tok)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrShellTokenRevoked
	}
	if err != nil {
		log.Printf("[ShellToken] DB check failed, refusing session %s: %v", claims.ShellSessionID, err)
		return nil, ErrShellTokenUnavailable
	}
	now := time.Now()
	switch {
	case row.ShellSessionID != claims.ShellSessionID || row.WorkspaceID != claims.WorkspaceID ||
		row.SandboxID != claims.SandboxID || row.OrgID != claims.OrgID:
		return nil, ErrShellTokenInvalid
	case row.RevokedAt != nil || !row.ExpiresAt.After(now):
		return nil, ErrShellTokenRevoked
	case row.SessClosedAt != nil || row.SessStatus != models.ShellSessionStatusActive || !row.SessExpiresAt.After(now):
		return nil, ErrShellTokenRevoked
	}
	return &StateCaller{
		Type:           StateTokenTypeShell,
		WorkspaceID:    row.WorkspaceID,
		SessionID:      row.ShellSessionID,
		ShellSessionID: row.ShellSessionID,
		SandboxID:      row.SandboxID,
		OrgID:          row.OrgID,
	}, nil
}
