package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"iac-platform/services"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ccTokenHash(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return base64.StdEncoding.EncodeToString(h[:])
}

// C&C authentication: the agent token identifies the agent (agent_id query,
// if any, must match); a pool token is refused for agents that have their
// own token; revoked agents are refused. An accepted request reaches the
// WebSocket upgrade (400 here: plain HTTP, no handshake).
func TestRawCC_AgentTokenAuth(t *testing.T) {
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	const pool = "apt_pool-1_secret"
	for _, s := range []string{
		`CREATE TABLE pool_tokens (token_hash TEXT PRIMARY KEY, token_name TEXT, token_type TEXT, pool_id TEXT, is_active INTEGER, expires_at DATETIME, last_used_at DATETIME, k8s_namespace TEXT, revoked_at DATETIME, created_at DATETIME, created_by TEXT, revoked_by TEXT, k8s_config TEXT)`,
		`CREATE TABLE agents (agent_id TEXT PRIMARY KEY, pool_id TEXT, name TEXT, capabilities TEXT, token_generation INTEGER NOT NULL DEFAULT 0, revoked_at DATETIME, pool_token_hash TEXT, status TEXT, updated_at DATETIME)`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(`INSERT INTO pool_tokens (token_hash, token_name, token_type, pool_id, is_active) VALUES (?,?,?,?,1)`, ccTokenHash(pool), "n", "static", "pool-1")
	db.Exec(`INSERT INTO agents (agent_id, pool_id, capabilities, pool_token_hash) VALUES ('agent-t','pool-1','["agent_token_v1"]',?),('agent-l','pool-1','[]',?),('agent-r','pool-1','["agent_token_v1"]',?)`,
		ccTokenHash(pool), ccTokenHash(pool), ccTokenHash(pool))
	h := NewRawAgentCCHandler(db, nil)
	tokT, _, _ := services.IssueAgentToken("agent-t", "pool-1", 0)
	tokR, _, _ := services.IssueAgentToken("agent-r", "pool-1", 0)
	if err := services.RevokeAgentTokens(t.Context(), db, "agent-r"); err != nil {
		t.Fatal(err)
	}

	call := func(token, agentID string) int {
		url := "/api/v1/agents/control"
		if agentID != "" {
			url += "?agent_id=" + agentID
		}
		req := httptest.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	for _, tc := range []struct {
		name, token, agentID string
		want                 int
	}{
		{"agent token, no agent_id", tokT, "", http.StatusBadRequest},
		{"agent token, matching agent_id", tokT, "agent-t", http.StatusBadRequest},
		{"agent token, other agent_id", tokT, "agent-l", http.StatusForbidden},
		{"revoked agent", tokR, "agent-r", http.StatusUnauthorized},
		{"pool token for agent with own token", pool, "agent-t", http.StatusForbidden},
		{"pool token, older agent", pool, "agent-l", http.StatusBadRequest},
		{"garbage jwt", "a.b.c", "", http.StatusUnauthorized},
	} {
		if got := call(tc.token, tc.agentID); got != tc.want {
			t.Errorf("%s: want %d, got %d", tc.name, tc.want, got)
		}
	}
}
