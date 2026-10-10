package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"iac-platform/internal/keys"
	"iac-platform/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// Per-agent tokens.
//
// A pool token (apt_…) is shared by every agent of a pool and is only used to
// register. Registration returns an agent token: a JWT signed with the
// "agent" purpose key (keys.PurposeAgent, kid "agent-v<n>"), claims
// typ = "agent", agent_id, pool_id, gen, iat, exp (AGENT_TOKEN_TTL, default
// 15 min). The agent renews it before expiry (POST /api/v1/agents/token) with
// the token itself; an expired or revoked token means registering again with
// the pool token.
//
// Revocation is DB-backed: every use loads the agents row (one primary-key
// lookup joined with the registering pool token) and requires
//   - the row to exist in the token's pool (deregistration and the stale-agent
//     cleanup delete it),
//   - revoked_at IS NULL and token_generation = the token's gen (explicit
//     revocation sets both), and
//   - the pool token used for registration to be still active and unexpired
//     (revoking / rotating a pool token cuts off its agents).
// A database error refuses the token. The agent routes already query the task
// row on every call, so this adds one indexed lookup instead of a revocation
// list that would need cross-replica invalidation; the short TTL bounds a
// token copied off a host whose agent row survives.

// AgentTokenType the typ claim of agent tokens.
const AgentTokenType = "agent"

// DefaultAgentTokenTTL lifetime of an agent token.
const DefaultAgentTokenTTL = 15 * time.Minute

// AgentTokenClaims claims of a per-agent JWT.
type AgentTokenClaims struct {
	Type       string `json:"typ"`
	AgentID    string `json:"agent_id"`
	PoolID     string `json:"pool_id"`
	Generation int    `json:"gen"`
	jwt.RegisteredClaims
}

var (
	// ErrAgentTokenInvalid signature, typ, expiry or claims wrong.
	ErrAgentTokenInvalid = errors.New("invalid agent token")
	// ErrAgentTokenRevoked the agent was deregistered or revoked, or its pool
	// token is no longer active.
	ErrAgentTokenRevoked = errors.New("agent token revoked")
)

// AgentTokenTTL the configured agent token lifetime (AGENT_TOKEN_TTL).
func AgentTokenTTL() time.Duration {
	if v := os.Getenv("AGENT_TOKEN_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d >= time.Minute && d <= 24*time.Hour {
			return d
		}
		log.Printf("[WARN] invalid AGENT_TOKEN_TTL=%q, using %s", v, DefaultAgentTokenTTL)
	}
	return DefaultAgentTokenTTL
}

// LooksLikeJWT whether a bearer credential is a JWT (pool tokens start with
// apt_).
func LooksLikeJWT(tok string) bool {
	return strings.Count(tok, ".") == 2 && !strings.HasPrefix(tok, "apt_")
}

// IssueAgentToken signs a token for agent (its current generation).
func IssueAgentToken(agentID, poolID string, generation int) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(AgentTokenTTL())
	jti := make([]byte, 12)
	if _, err := rand.Read(jti); err != nil {
		return "", time.Time{}, err
	}
	tok, err := keys.Sign(keys.PurposeAgent, AgentTokenClaims{
		Type:       AgentTokenType,
		AgentID:    agentID,
		PoolID:     poolID,
		Generation: generation,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "agent:" + agentID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-30 * time.Second)),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        hex.EncodeToString(jti),
		},
	}, nil) // no legacy scheme: SIGNING_ROOT_KEY is required
	if err != nil {
		return "", time.Time{}, err
	}
	return tok, exp, nil
}

// ParseAgentToken verifies signature, typ, expiry and claims (no DB check).
func ParseAgentToken(tok string) (*AgentTokenClaims, error) {
	claims := &AgentTokenClaims{}
	t, err := jwt.ParseWithClaims(tok, claims, keys.Keyfunc(keys.PurposeAgent, nil),
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !t.Valid {
		return nil, fmt.Errorf("%w: %v", ErrAgentTokenInvalid, err)
	}
	if claims.Type != AgentTokenType || claims.AgentID == "" || claims.PoolID == "" {
		return nil, ErrAgentTokenInvalid
	}
	return claims, nil
}

// AgentIdentity an authenticated agent.
type AgentIdentity struct {
	AgentID      string
	PoolID       string
	Generation   int
	Capabilities *string
}

type agentTokenRow struct {
	AgentID         string
	PoolID          *string
	TokenGeneration int
	RevokedAt       *time.Time
	Capabilities    *string
}

// CheckAgentActive the DB part of agent token validation (also used to
// re-check long-lived C&C connections).
func CheckAgentActive(ctx context.Context, db *gorm.DB, agentID, poolID string, generation int) (*AgentIdentity, error) {
	var row agentTokenRow
	err := db.WithContext(ctx).Table("agents AS a").
		Select("a.agent_id, a.pool_id, a.token_generation, a.revoked_at, a.capabilities").
		Joins("JOIN pool_tokens pt ON pt.token_hash = a.pool_token_hash AND pt.pool_id = a.pool_id").
		Where("a.agent_id = ? AND a.pool_id = ?", agentID, poolID).
		Where("pt.is_active = ? AND (pt.expires_at IS NULL OR pt.expires_at > ?)", true, time.Now()).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAgentTokenRevoked
	}
	if err != nil {
		return nil, fmt.Errorf("agent token check: %w", err)
	}
	if row.RevokedAt != nil || row.TokenGeneration != generation {
		return nil, ErrAgentTokenRevoked
	}
	return &AgentIdentity{AgentID: row.AgentID, PoolID: poolID, Generation: generation, Capabilities: row.Capabilities}, nil
}

// ValidateAgentToken full validation of an agent token (signature, typ,
// expiry, then the DB-backed revocation check). Any DB error refuses.
func ValidateAgentToken(ctx context.Context, db *gorm.DB, tok string) (*AgentIdentity, error) {
	claims, err := ParseAgentToken(tok)
	if err != nil {
		return nil, err
	}
	return CheckAgentActive(ctx, db, claims.AgentID, claims.PoolID, claims.Generation)
}

// AgentRevocationHook called after an agent's tokens are revoked (run tokens
// are revoked through it; see run_token_service.go).
var AgentRevocationHook func(ctx context.Context, db *gorm.DB, agentID string) error

// RevokeAgentTokens invalidates every token of agent: revoked_at set,
// generation bumped (the row is kept so the revocation is visible; the agent
// may register again with its pool token as a new agent; revoke the pool
// token to stop a host).
func RevokeAgentTokens(ctx context.Context, db *gorm.DB, agentID string) error {
	res := db.WithContext(ctx).Model(&models.Agent{}).Where("agent_id = ?", agentID).Updates(map[string]interface{}{
		"revoked_at":       time.Now(),
		"token_generation": gorm.Expr("token_generation + 1"),
		"status":           models.AgentStatusOffline,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	if AgentRevocationHook != nil {
		return AgentRevocationHook(ctx, db, agentID)
	}
	return nil
}
