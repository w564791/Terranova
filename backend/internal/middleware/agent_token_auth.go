package middleware

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Agent caller authentication (agent task / workspace API, pool secrets,
// Terraform versions, agent self-management).
//
// Two credentials:
//   - an agent token (per-agent JWT, typ agent; services.ValidateAgentToken):
//     the agent ID comes from the token; context "agent_auth" = "agent",
//     "token_agent_id" = the agent;
//   - a pool token (apt_…, shared by the pool's agents): pool only; context
//     "agent_auth" = "pool". Accepted for agents without
//     models.AgentCapabilityAgentTokenV1 (older agents); refused on the tasks
//     of an agent that has it (verifyTaskAgent).
//
// The X-Agent-ID header is not read.

const (
	agentAuthKey      = "agent_auth"
	agentAuthAgent    = "agent"
	agentAuthPool     = "pool"
	tokenAgentIDKey   = "token_agent_id"
	tokenAgentGenKey  = "token_agent_gen"
	tokenAgentCapsKey = "token_agent_caps"
)

// TokenAgentID the agent ID of an agent-token caller ("" for pool tokens).
func TokenAgentID(c *gin.Context) string {
	if c.GetString(agentAuthKey) != agentAuthAgent {
		return ""
	}
	return c.GetString(tokenAgentIDKey)
}

// TokenAgentGeneration the gen claim of an agent-token caller.
func TokenAgentGeneration(c *gin.Context) int {
	return c.GetInt(tokenAgentGenKey)
}

func bearer(c *gin.Context) (string, bool) {
	h := c.GetHeader("Authorization")
	tok := strings.TrimPrefix(h, "Bearer ")
	if h == "" || tok == h || tok == "" {
		return "", false
	}
	return tok, true
}

// authenticateAgentCaller accepts an agent token or a pool token.
func authenticateAgentCaller(c *gin.Context, db *gorm.DB) bool {
	tok, ok := bearer(c)
	if !ok {
		respondWithError(c, http.StatusUnauthorized, "Missing authorization header")
		return false
	}
	if !services.LooksLikeJWT(tok) {
		return authenticatePoolToken(c, db)
	}
	id, err := services.ValidateAgentToken(c.Request.Context(), db, tok)
	if err != nil {
		if errors.Is(err, services.ErrAgentTokenInvalid) || errors.Is(err, services.ErrAgentTokenRevoked) {
			respondWithError(c, http.StatusUnauthorized, err.Error())
			return false
		}
		// DB check failed: refuse, never fall back to the signature alone
		log.Printf("[AgentAuth] agent token check failed: %v", err)
		respondWithError(c, http.StatusServiceUnavailable, "agent token could not be verified")
		return false
	}
	c.Set(agentAuthKey, agentAuthAgent)
	c.Set(tokenAgentIDKey, id.AgentID)
	c.Set(tokenAgentGenKey, id.Generation)
	if id.Capabilities != nil {
		c.Set(tokenAgentCapsKey, *id.Capabilities)
	}
	c.Set("pool_id", id.PoolID)
	return true
}

// AgentOrPoolTokenAuth authenticates an agent token or a pool token (routes
// that are pool-scoped, not task-scoped).
func AgentOrPoolTokenAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authenticateAgentCaller(c, db) {
			return
		}
		c.Next()
	}
}

// RequireAgentToken authenticates an agent token only (token renewal).
func RequireAgentToken(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tok, ok := bearer(c)
		if !ok || !services.LooksLikeJWT(tok) {
			respondWithError(c, http.StatusUnauthorized, "agent token required")
			return
		}
		if !authenticateAgentCaller(c, db) {
			return
		}
		c.Next()
	}
}
