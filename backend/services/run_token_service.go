package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"iac-platform/internal/keys"
	"iac-platform/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// Run tokens (docs/manifest/manifest-sandbox-spec.md §6.1, §9 step 5).
//
// A run token is the only platform credential of a manifest run (agent
// runner or sandbox): HS256 under the "run" signing purpose (kid run-v<n>,
// no legacy scheme), claims typ=run, run_id, workspace_id, purpose
// (preview|approval), session_id (sandbox runs), agent_id (agent runs). Its
// SHA-256 is stored in run_tokens. It only opens the HTTP state backend of
// the run's workspace; preview tokens are read-only.
//
// Validation always consults the database and refuses on any DB error (no
// JWT-only fallback, unlike task tokens): the row must exist, not be revoked
// or expired, match the claims; the run must be pending/running; the session
// (if any) open and unexpired; the issuing agent (if any) present and not
// revoked, with its pool token active.
//
// Expiry: min(run created_at + MANIFEST_RUN_TIMEOUT, session expires_at).
// Revocation: EndManifestRun (that run's tokens only), EndSandboxSession /
// ExpireSandboxSessions (every token of the session, plus the session's STS
// credentials through SessionCredentialRevoker), RevokeAgentTokens (through
// AgentRevocationHook: every run token the agent obtained).

const (
	// StateTokenTypeTask the typ claim of task state tokens.
	StateTokenTypeTask = "task"
	// StateTokenTypeRun the typ claim of run tokens.
	StateTokenTypeRun = "run"

	// DefaultManifestRunTimeout the longest a manifest run (and so its
	// token) may last (MANIFEST_RUN_TIMEOUT).
	DefaultManifestRunTimeout = 2 * time.Hour
)

var (
	// ErrRunTokenInvalid signature, typ, expiry or claims wrong.
	ErrRunTokenInvalid = errors.New("invalid run token")
	// ErrRunTokenRevoked revoked / expired row, run ended, session closed or
	// expired, agent revoked.
	ErrRunTokenRevoked = errors.New("run token revoked or run not active")
	// ErrRunTokenUnavailable the database check failed: refused.
	ErrRunTokenUnavailable = errors.New("run token could not be verified")
	// ErrRunNotAssigned the run is not an active agent run assigned to the
	// calling agent.
	ErrRunNotAssigned = errors.New("run is not assigned to this agent or not active")
)

// RunTokenClaims the claims of a run token.
type RunTokenClaims struct {
	Type        string `json:"typ"`
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
	Purpose     string `json:"purpose"`
	SessionID   string `json:"session_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	jwt.RegisteredClaims
}

// ManifestRunTimeout the configured run timeout (MANIFEST_RUN_TIMEOUT).
func ManifestRunTimeout() time.Duration {
	if v := os.Getenv("MANIFEST_RUN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= time.Minute && d <= 24*time.Hour {
			return d
		}
		log.Printf("[WARN] invalid MANIFEST_RUN_TIMEOUT=%q, using %s", v, DefaultManifestRunTimeout)
	}
	return DefaultManifestRunTimeout
}

func runActive(status string) bool {
	return status == models.ManifestRunStatusPending || status == models.ManifestRunStatusRunning
}

func sessionOpen(s *models.SandboxSession, now time.Time) bool {
	return s.ClosedAt == nil && s.Status == models.SandboxSessionStatusActive && s.ExpiresAt.After(now)
}

// IssueRunToken issues a token for run. agentID is the calling agent for
// agent runs (the run must be a runner=agent run assigned to it); "" when the
// platform issues it itself (sandbox runs, step 6).
func (s *StateTokenService) IssueRunToken(ctx context.Context, runID, agentID string) (string, time.Time, error) {
	db := s.db.WithContext(ctx)
	var run models.ManifestRun
	if err := db.Where("id = ?", runID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", time.Time{}, ErrRunNotAssigned
		}
		return "", time.Time{}, err
	}
	if !runActive(run.Status) {
		return "", time.Time{}, ErrRunNotAssigned
	}
	if agentID != "" {
		if run.Runner != models.ManifestRunRunnerAgent || run.AgentID == nil || *run.AgentID != agentID {
			return "", time.Time{}, ErrRunNotAssigned
		}
	} else if run.Runner == models.ManifestRunRunnerAgent {
		return "", time.Time{}, fmt.Errorf("agent runs obtain their token with the agent token")
	}
	now := time.Now()
	exp := run.CreatedAt.Add(ManifestRunTimeout())
	if run.SessionID != nil {
		var sess models.SandboxSession
		if err := db.Where("id = ?", *run.SessionID).Take(&sess).Error; err != nil {
			return "", time.Time{}, err
		}
		if !sessionOpen(&sess, now) {
			return "", time.Time{}, ErrRunNotAssigned
		}
		if sess.ExpiresAt.Before(exp) {
			exp = sess.ExpiresAt
		}
	}
	if !exp.After(now) {
		return "", time.Time{}, ErrRunNotAssigned // run timed out
	}
	jti := make([]byte, 12)
	if _, err := rand.Read(jti); err != nil {
		return "", time.Time{}, err
	}
	claims := RunTokenClaims{
		Type: StateTokenTypeRun, RunID: run.ID, WorkspaceID: run.WorkspaceID, Purpose: run.Purpose,
		AgentID: agentID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "run:" + run.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        hex.EncodeToString(jti),
		},
	}
	if run.SessionID != nil {
		claims.SessionID = *run.SessionID
	}
	tok, err := keys.Sign(keys.PurposeRun, claims, nil) // no legacy scheme
	if err != nil {
		return "", time.Time{}, err
	}
	row := models.RunToken{
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: run.WorkspaceID, Purpose: run.Purpose,
		TokenHash: sha256Hash(tok), ExpiresAt: exp,
	}
	if agentID != "" {
		row.AgentID = &agentID
	}
	if err := db.Create(&row).Error; err != nil {
		return "", time.Time{}, fmt.Errorf("store run token: %w", err)
	}
	return tok, exp, nil
}

// StateCaller an authenticated state backend caller.
type StateCaller struct {
	Type        string // StateTokenTypeTask or StateTokenTypeRun
	WorkspaceID string
	TaskID      uint   // task tokens
	RunID       string // run tokens
	Purpose     string // run tokens: preview | approval
	SessionID   string
	CreatedBy   string // run tokens: the run's creator
}

// ReadOnly whether the caller may only read state (preview runs).
func (c *StateCaller) ReadOnly() bool {
	return c.Type == StateTokenTypeRun && c.Purpose != models.ManifestRunPurposeApproval
}

// tokenType the unverified typ claim (selects the verification path; the
// signature is then checked with that path's purpose key, so a forged typ
// cannot cross paths).
func tokenType(tok string) string {
	var m jwt.MapClaims
	if _, _, err := jwt.NewParser().ParseUnverified(tok, &m); err != nil {
		return ""
	}
	typ, _ := m["typ"].(string)
	return typ
}

// Authenticate validates a state backend token: run tokens (typ run) on the
// run path, everything else on the unchanged task path (ValidateToken).
func (s *StateTokenService) Authenticate(ctx context.Context, tok string) (*StateCaller, error) {
	if tokenType(tok) == StateTokenTypeRun {
		return s.validateRunToken(ctx, tok)
	}
	ws, taskID, err := s.ValidateToken(tok)
	if err != nil {
		return nil, err
	}
	return &StateCaller{Type: StateTokenTypeTask, WorkspaceID: ws, TaskID: taskID}, nil
}

type runTokenRow struct {
	RunID         string
	SessionID     *string
	WorkspaceID   string
	Purpose       string
	ExpiresAt     time.Time
	RevokedAt     *time.Time
	AgentID       *string
	RunStatus     string
	RunCreatedBy  string
	SessClosedAt  *time.Time
	SessStatus    *string
	SessExpiresAt *time.Time
}

func (s *StateTokenService) validateRunToken(ctx context.Context, tok string) (*StateCaller, error) {
	claims := &RunTokenClaims{}
	t, err := jwt.ParseWithClaims(tok, claims, keys.Keyfunc(keys.PurposeRun, nil),
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !t.Valid || claims.Type != StateTokenTypeRun || claims.RunID == "" || claims.WorkspaceID == "" {
		return nil, ErrRunTokenInvalid
	}
	var row runTokenRow
	err = s.db.WithContext(ctx).Table("run_tokens AS rt").
		Select(`rt.run_id, rt.session_id, rt.workspace_id, rt.purpose, rt.expires_at, rt.revoked_at, rt.agent_id,
			r.status AS run_status, r.created_by AS run_created_by,
			ss.closed_at AS sess_closed_at, ss.status AS sess_status, ss.expires_at AS sess_expires_at`).
		Joins("JOIN manifest_runs r ON r.id = rt.run_id").
		Joins("LEFT JOIN sandbox_sessions ss ON ss.id = rt.session_id").
		Where("rt.token_hash = ?", sha256Hash(tok)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRunTokenRevoked
	}
	if err != nil {
		log.Printf("[RunToken] DB check failed, refusing run %s: %v", claims.RunID, err)
		return nil, ErrRunTokenUnavailable
	}
	now := time.Now()
	sessionID := ""
	if row.SessionID != nil {
		sessionID = *row.SessionID
	}
	switch {
	case row.RunID != claims.RunID || row.WorkspaceID != claims.WorkspaceID || row.Purpose != claims.Purpose || sessionID != claims.SessionID:
		return nil, ErrRunTokenInvalid
	case row.RevokedAt != nil || !row.ExpiresAt.After(now) || !runActive(row.RunStatus):
		return nil, ErrRunTokenRevoked
	case row.SessionID != nil && (row.SessClosedAt != nil || row.SessStatus == nil || *row.SessStatus != models.SandboxSessionStatusActive ||
		row.SessExpiresAt == nil || !row.SessExpiresAt.After(now)):
		return nil, ErrRunTokenRevoked
	}
	if row.AgentID != nil {
		if claims.AgentID != *row.AgentID {
			return nil, ErrRunTokenInvalid
		}
		ok, err := agentActive(ctx, s.db, *row.AgentID)
		if err != nil {
			log.Printf("[RunToken] agent check failed, refusing run %s: %v", claims.RunID, err)
			return nil, ErrRunTokenUnavailable
		}
		if !ok {
			return nil, ErrRunTokenRevoked
		}
	}
	return &StateCaller{
		Type: StateTokenTypeRun, WorkspaceID: row.WorkspaceID, RunID: row.RunID, Purpose: row.Purpose,
		SessionID: sessionID, CreatedBy: row.RunCreatedBy,
	}, nil
}

// agentActive the agent exists, is not revoked, and the pool token it
// registered with is active (the agent-token conditions, any generation).
func agentActive(ctx context.Context, db *gorm.DB, agentID string) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Table("agents AS a").
		Joins("JOIN pool_tokens pt ON pt.token_hash = a.pool_token_hash AND pt.pool_id = a.pool_id").
		Where("a.agent_id = ? AND a.revoked_at IS NULL", agentID).
		Where("pt.is_active = ? AND (pt.expires_at IS NULL OR pt.expires_at > ?)", true, time.Now()).
		Count(&n).Error
	return n > 0, err
}

// RevokeRunTokens revokes every token of run (run end).
func RevokeRunTokens(ctx context.Context, db *gorm.DB, runID string) error {
	return db.WithContext(ctx).Model(&models.RunToken{}).
		Where("run_id = ? AND revoked_at IS NULL", runID).
		Update("revoked_at", time.Now()).Error
}

// RevokeAgentRunTokens revokes every run token agentID obtained (agent
// revocation / deregistration; AgentRevocationHook).
func RevokeAgentRunTokens(ctx context.Context, db *gorm.DB, agentID string) error {
	return db.WithContext(ctx).Model(&models.RunToken{}).
		Where("agent_id = ? AND revoked_at IS NULL", agentID).
		Update("revoked_at", time.Now()).Error
}

func init() {
	AgentRevocationHook = RevokeAgentRunTokens
}

// EndManifestRun moves run to a final status and revokes its own tokens
// (only this run's; the session keeps running).
func EndManifestRun(ctx context.Context, db *gorm.DB, runID, status string) error {
	switch status {
	case models.ManifestRunStatusSucceeded, models.ManifestRunStatusFailed, models.ManifestRunStatusCancelled:
	default:
		return fmt.Errorf("not a final run status: %q", status)
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.ManifestRun{}).Where("id = ?", runID).
			Updates(map[string]interface{}{"status": status, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return RevokeRunTokens(ctx, tx, runID)
	})
}

// SessionCredentialRevoker revokes the cloud credentials (read-only STS) of
// a sandbox session. Step 6 (AgentCore provider) plugs the real one in.
type SessionCredentialRevoker interface {
	RevokeSessionCredentials(ctx context.Context, session *models.SandboxSession) error
}

type stubSessionCredentialRevoker struct{}

func (stubSessionCredentialRevoker) RevokeSessionCredentials(_ context.Context, session *models.SandboxSession) error {
	log.Printf("[SandboxSession] STS revocation for session %s (provider %s): no provider wired yet (step 6)", session.ID, session.Provider)
	return nil
}

// SessionCredentials the active SessionCredentialRevoker (stub until step 6).
var SessionCredentials SessionCredentialRevoker = stubSessionCredentialRevoker{}

// EndSandboxSession closes a session (status closed or expired): every run
// token of the session is revoked, then its STS credentials. Idempotent.
func EndSandboxSession(ctx context.Context, db *gorm.DB, sessionID, status string) error {
	if status != models.SandboxSessionStatusClosed && status != models.SandboxSessionStatusExpired {
		return fmt.Errorf("not a final session status: %q", status)
	}
	var sess models.SandboxSession
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", sessionID).Take(&sess).Error; err != nil {
			return err
		}
		now := time.Now()
		if sess.ClosedAt == nil {
			if err := tx.Model(&models.SandboxSession{}).Where("id = ? AND closed_at IS NULL", sessionID).
				Updates(map[string]interface{}{"status": status, "closed_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&models.RunToken{}).Where("session_id = ? AND revoked_at IS NULL", sessionID).
			Update("revoked_at", now).Error
	})
	if err != nil {
		return err
	}
	return SessionCredentials.RevokeSessionCredentials(ctx, &sess)
}

// ExpireSandboxSessions ends every open session past its expiry. Returns how
// many were ended.
func ExpireSandboxSessions(ctx context.Context, db *gorm.DB, now time.Time) (int, error) {
	var ids []string
	if err := db.WithContext(ctx).Model(&models.SandboxSession{}).
		Where("closed_at IS NULL AND expires_at <= ?", now).Limit(500).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if err := EndSandboxSession(ctx, db, id, models.SandboxSessionStatusExpired); err != nil {
			log.Printf("[SandboxSession] expire %s: %v", id, err)
			continue
		}
		n++
	}
	return n, nil
}

// StartSandboxSessionExpiry runs ExpireSandboxSessions every interval until
// ctx ends (leader only). Validation already refuses tokens of expired
// sessions; this revokes them and the session's STS credentials.
func StartSandboxSessionExpiry(ctx context.Context, db *gorm.DB, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if n, err := ExpireSandboxSessions(ctx, db, time.Now()); err != nil {
				log.Printf("[SandboxSession] expiry sweep: %v", err)
			} else if n > 0 {
				log.Printf("[SandboxSession] expired %d session(s)", n)
			}
		}
	}
}
