package router

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/keys"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	tokPool1 = "apt_pool-1_secretvalue"
	tokPool2 = "apt_pool-2_secretvalue" // another tenant's pool, also allowed on ws-1
)

func poolTokenHash(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return base64.StdEncoding.EncodeToString(h[:])
}

// Agent routes with: pool-1 agents agent-a, agent-b (agent tokens) and
// agent-l (older agent, pool token only); pool-2 agent agent-x (agent token).
// Task 10 (ws-1) runs on agent-b, task 12 (ws-2) on agent-l.
func setupAgentOwnershipRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE pool_tokens (token_hash TEXT PRIMARY KEY, token_name TEXT, token_type TEXT, pool_id TEXT, is_active INTEGER, expires_at DATETIME, last_used_at DATETIME, k8s_namespace TEXT)`,
		`CREATE TABLE pool_allowed_workspaces (id INTEGER PRIMARY KEY AUTOINCREMENT, pool_id TEXT, workspace_id TEXT, status TEXT)`,
		`CREATE TABLE run_tokens (id INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT, session_id TEXT, workspace_id TEXT, purpose TEXT, token_hash TEXT, expires_at DATETIME, revoked_at DATETIME, agent_id TEXT, created_at DATETIME)`,
		`CREATE TABLE agents (agent_id TEXT PRIMARY KEY, pool_id TEXT, capabilities TEXT, token_generation INTEGER NOT NULL DEFAULT 0, revoked_at DATETIME, pool_token_hash TEXT, status TEXT, updated_at DATETIME)`,
		`CREATE TABLE workspace_tasks (id INTEGER PRIMARY KEY, workspace_id TEXT, agent_id TEXT, status TEXT, completed_at DATETIME, updated_at DATETIME)`,
		`INSERT INTO pool_allowed_workspaces (pool_id, workspace_id, status) VALUES ('pool-1','ws-1','active'),('pool-2','ws-1','active'),('pool-1','ws-2','active')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	for tok, pool := range map[string]string{tokPool1: "pool-1", tokPool2: "pool-2"} {
		if err := db.Exec(`INSERT INTO pool_tokens (token_hash, token_name, token_type, pool_id, is_active) VALUES (?,?,?,?,1)`,
			poolTokenHash(tok), "n", models.PoolTokenTypeStatic, pool).Error; err != nil {
			t.Fatal(err)
		}
	}
	tokCaps := `["manifest_bundle_v1","task_data_overrides_v1","agent_token_v1"]`
	for _, a := range [][4]string{
		{"agent-a", "pool-1", tokCaps, tokPool1},
		{"agent-b", "pool-1", tokCaps, tokPool1},
		{"agent-l", "pool-1", `[]`, tokPool1},
		{"agent-x", "pool-2", tokCaps, tokPool2},
	} {
		db.Exec(`INSERT INTO agents (agent_id, pool_id, capabilities, pool_token_hash, status) VALUES (?,?,?,?,'online')`, a[0], a[1], a[2], poolTokenHash(a[3]))
	}
	now := time.Now()
	db.Exec(`INSERT INTO workspace_tasks VALUES (10,'ws-1','agent-b','running',NULL,?)`, now)
	db.Exec(`INSERT INTO workspace_tasks VALUES (11,'ws-2','agent-b','success',?,?)`, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	db.Exec(`INSERT INTO workspace_tasks VALUES (12,'ws-2','agent-l','running',NULL,?)`, now)

	r := gin.New()
	r.Use(gin.Recovery())
	api := r.Group("/api/v1")
	setupAgentAPIRoutes(api, db, nil, nil, nil, nil, nil)
	return r, db
}

func agentToken(t *testing.T, agentID, poolID string) string {
	t.Helper()
	tok, _, err := services.IssueAgentToken(agentID, poolID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// every agent task / workspace route, with its path filled in
func agentScopedRoutes(t *testing.T, r *gin.Engine, taskID, ws string) [][2]string {
	var out [][2]string
	for _, rt := range r.Routes() {
		p := rt.Path
		switch {
		case strings.HasPrefix(p, "/api/v1/agents/tasks/:task_id"):
			out = append(out, [2]string{rt.Method, strings.Replace(p, ":task_id", taskID, 1)})
		case strings.HasPrefix(p, "/api/v1/agents/workspaces/:workspace_id"):
			out = append(out, [2]string{rt.Method, strings.Replace(p, ":workspace_id", ws, 1)})
		}
	}
	if len(out) < 19 {
		t.Fatalf("expected all agent task/workspace routes, got %d", len(out))
	}
	return out
}

func agentCall(r *gin.Engine, method, path, token string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func passesOwnership(code int) bool {
	return code != http.StatusUnauthorized && code != http.StatusForbidden && code != http.StatusConflict && code != http.StatusServiceUnavailable
}

// Agent A's token (same pool), an agent token of another pool allowed on the
// workspace, and the pool tokens (even with an X-Agent-ID header naming the
// owner) all get 403 on every task and workspace route of a task running on
// agent B; agent B's token passes.
func TestAgentRoutes_RejectNonOwningAgent(t *testing.T) {
	r, _ := setupAgentOwnershipRouter(t)
	tokA, tokB, tokX := agentToken(t, "agent-a", "pool-1"), agentToken(t, "agent-b", "pool-1"), agentToken(t, "agent-x", "pool-2")
	for _, rt := range agentScopedRoutes(t, r, "10", "ws-1") {
		for name, tok := range map[string]string{"agent-a token": tokA, "agent-x token": tokX, "pool-1 token": tokPool1, "pool-2 token": tokPool2} {
			if w := agentCall(r, rt[0], rt[1], tok, "X-Agent-ID", "agent-b"); w.Code != http.StatusForbidden {
				t.Errorf("%s %s with %s: want 403, got %d %s", rt[0], rt[1], name, w.Code, w.Body.String())
			}
		}
		if w := agentCall(r, rt[0], rt[1], tokB); !passesOwnership(w.Code) {
			t.Errorf("%s %s as agent-b: ownership check must pass, got %d %s", rt[0], rt[1], w.Code, w.Body.String())
		}
	}
}

// Ended tasks answer 409; an older agent (no agent_token_v1) still works with
// the pool token, agent tokens of other agents do not.
func TestAgentRoutes_StateAndLegacyAgent(t *testing.T) {
	r, _ := setupAgentOwnershipRouter(t)
	tokA, tokB := agentToken(t, "agent-a", "pool-1"), agentToken(t, "agent-b", "pool-1")
	for _, p := range []string{"/api/v1/agents/tasks/11/data", "/api/v1/agents/tasks/11/plan-task"} {
		if w := agentCall(r, "GET", p, tokB); w.Code != http.StatusConflict {
			t.Errorf("GET %s on ended task: want 409, got %d %s", p, w.Code, w.Body.String())
		}
	}
	if w := agentCall(r, "PUT", "/api/v1/agents/tasks/11/status", tokB); w.Code != http.StatusConflict {
		t.Errorf("status update 2h after completion: want 409, got %d", w.Code)
	}
	if w := agentCall(r, "GET", "/api/v1/agents/tasks/12/data", tokPool1); !passesOwnership(w.Code) {
		t.Errorf("older agent with pool token: got %d %s", w.Code, w.Body.String())
	}
	if w := agentCall(r, "GET", "/api/v1/agents/tasks/12/data", tokA); w.Code != http.StatusForbidden {
		t.Errorf("agent-a token on agent-l's task: want 403, got %d", w.Code)
	}
	// ws-2: only agent-l's running task counts; agent-b's task ended 2h ago
	if w := agentCall(r, "GET", "/api/v1/agents/workspaces/ws-2/state/max-version", tokB); w.Code != http.StatusForbidden {
		t.Errorf("workspace route for an agent without an active task: want 403, got %d", w.Code)
	}
	if w := agentCall(r, "GET", "/api/v1/agents/workspaces/ws-2/state/max-version", tokPool1); !passesOwnership(w.Code) {
		t.Errorf("older agent's workspace route with pool token: got %d %s", w.Code, w.Body.String())
	}
}

// Within the grace period the task's agent may still report (status retry of
// the same final status, final logs) but not change the final status.
func TestAgentRoutes_ReportingGrace(t *testing.T) {
	r, db := setupAgentOwnershipRouter(t)
	tokA, tokB := agentToken(t, "agent-a", "pool-1"), agentToken(t, "agent-b", "pool-1")
	now := time.Now()
	db.Exec(`UPDATE workspace_tasks SET status = 'applied', completed_at = ? WHERE id = 10`, now.Add(-time.Minute))
	if w := agentCall(r, "GET", "/api/v1/agents/tasks/10/data", tokB); w.Code != http.StatusConflict {
		t.Errorf("task data after completion: want 409, got %d", w.Code)
	}
	if w := agentCall(r, "POST", "/api/v1/agents/tasks/10/logs/chunk", tokB); !passesOwnership(w.Code) {
		t.Errorf("final log chunk within grace: got %d %s", w.Code, w.Body.String())
	}
	if w := agentCall(r, "POST", "/api/v1/agents/workspaces/ws-1/unlock", tokB); w.Code == http.StatusForbidden {
		t.Errorf("post-apply unlock within grace: got %d %s", w.Code, w.Body.String())
	}
	if w := agentCall(r, "POST", "/api/v1/agents/tasks/10/logs/chunk", tokA); w.Code != http.StatusForbidden {
		t.Errorf("other agent within grace: want 403, got %d", w.Code)
	}
}

// Revocation, deregistration, pool token revocation, expiry, wrong purpose or
// typ: 401 on every agent route. A DB error refuses (503).
func TestAgentToken_Invalidation(t *testing.T) {
	const path = "/api/v1/agents/tasks/10/data"
	cases := []struct {
		name  string
		setup func(t *testing.T, db *gorm.DB) string
		want  int
	}{
		{"revoked", func(t *testing.T, db *gorm.DB) string {
			tok := agentToken(t, "agent-b", "pool-1")
			if err := services.RevokeAgentTokens(t.Context(), db, "agent-b"); err != nil {
				t.Fatal(err)
			}
			return tok
		}, http.StatusUnauthorized},
		{"generation bumped", func(t *testing.T, db *gorm.DB) string {
			tok := agentToken(t, "agent-b", "pool-1")
			db.Exec(`UPDATE agents SET token_generation = 1 WHERE agent_id = 'agent-b'`)
			return tok
		}, http.StatusUnauthorized},
		{"deregistered", func(t *testing.T, db *gorm.DB) string {
			tok := agentToken(t, "agent-b", "pool-1")
			db.Exec(`DELETE FROM agents WHERE agent_id = 'agent-b'`)
			return tok
		}, http.StatusUnauthorized},
		{"pool token revoked", func(t *testing.T, db *gorm.DB) string {
			tok := agentToken(t, "agent-b", "pool-1")
			db.Exec(`UPDATE pool_tokens SET is_active = 0 WHERE pool_id = 'pool-1'`)
			return tok
		}, http.StatusUnauthorized},
		{"pool token expired", func(t *testing.T, db *gorm.DB) string {
			tok := agentToken(t, "agent-b", "pool-1")
			db.Exec(`UPDATE pool_tokens SET expires_at = ? WHERE pool_id = 'pool-1'`, time.Now().Add(-time.Minute))
			return tok
		}, http.StatusUnauthorized},
		{"pool claim forged", func(t *testing.T, db *gorm.DB) string {
			return agentToken(t, "agent-b", "pool-2")
		}, http.StatusUnauthorized},
		{"expired", func(t *testing.T, db *gorm.DB) string {
			return signAgentClaims(t, keys.PurposeAgent, services.AgentTokenType, time.Now().Add(-time.Minute))
		}, http.StatusUnauthorized},
		{"user-purpose key", func(t *testing.T, db *gorm.DB) string {
			return signAgentClaims(t, keys.PurposeUser, services.AgentTokenType, time.Now().Add(time.Hour))
		}, http.StatusUnauthorized},
		{"wrong typ", func(t *testing.T, db *gorm.DB) string {
			return signAgentClaims(t, keys.PurposeAgent, "run", time.Now().Add(time.Hour))
		}, http.StatusUnauthorized},
		{"db error", func(t *testing.T, db *gorm.DB) string {
			tok := agentToken(t, "agent-b", "pool-1")
			db.Exec(`ALTER TABLE agents RENAME TO agents_unavailable`)
			return tok
		}, http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, db := setupAgentOwnershipRouter(t)
			tok := tc.setup(t, db)
			if w := agentCall(r, "GET", path, tok); w.Code != tc.want {
				t.Errorf("want %d, got %d %s", tc.want, w.Code, w.Body.String())
			}
			if w := agentCall(r, "POST", "/api/v1/agents/token", tok); w.Code != tc.want {
				t.Errorf("renewal: want %d, got %d", tc.want, w.Code)
			}
		})
	}
}

func signAgentClaims(t *testing.T, p keys.Purpose, typ string, exp time.Time) string {
	t.Helper()
	tok, err := keys.Sign(p, services.AgentTokenClaims{
		Type: typ, AgentID: "agent-b", PoolID: "pool-1",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(exp), IssuedAt: jwt.NewNumericDate(exp.Add(-time.Hour))},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// Renewal takes the agent token only and returns a fresh token for the same
// agent; the pool token is refused there.
func TestAgentToken_Renewal(t *testing.T) {
	r, db := setupAgentOwnershipRouter(t)
	w := agentCall(r, "POST", "/api/v1/agents/token", agentToken(t, "agent-b", "pool-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("renew: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		AgentID string `json:"agent_id"`
		Token   string `json:"agent_token"`
		Expires string `json:"agent_token_expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.AgentID != "agent-b" || body.Expires == "" {
		t.Fatalf("renew body: %v %s", err, w.Body.String())
	}
	id, err := services.ValidateAgentToken(t.Context(), db, body.Token)
	if err != nil || id.AgentID != "agent-b" || id.PoolID != "pool-1" {
		t.Fatalf("renewed token: %v %+v", err, id)
	}
	if w := agentCall(r, "POST", "/api/v1/agents/token", tokPool1); w.Code != http.StatusUnauthorized {
		t.Errorf("renew with pool token: want 401, got %d", w.Code)
	}
	if w := agentCall(r, "GET", "/api/v1/agents/tasks/10/data", body.Token); !passesOwnership(w.Code) {
		t.Errorf("renewed token on own task: %d %s", w.Code, w.Body.String())
	}
}
