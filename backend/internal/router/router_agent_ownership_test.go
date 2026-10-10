package router

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	tokPool1 = "apt_pool-1_secretvalue"
	tokPool2 = "apt_pool-2_secretvalue" // another tenant's pool, also allowed on ws-1
)

// Agent routes with: pool-1 agents agent-a, agent-b (identity-capable
// agent-c); pool-2 agent agent-x. Task 10 (ws-1) is running on agent-b.
func setupAgentOwnershipRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE pool_tokens (token_hash TEXT PRIMARY KEY, token_name TEXT, token_type TEXT, pool_id TEXT, is_active INTEGER, expires_at DATETIME, last_used_at DATETIME, k8s_namespace TEXT)`,
		`CREATE TABLE pool_allowed_workspaces (id INTEGER PRIMARY KEY AUTOINCREMENT, pool_id TEXT, workspace_id TEXT, status TEXT)`,
		`CREATE TABLE agents (agent_id TEXT PRIMARY KEY, pool_id TEXT, capabilities TEXT)`,
		`CREATE TABLE workspace_tasks (id INTEGER PRIMARY KEY, workspace_id TEXT, agent_id TEXT, status TEXT, completed_at DATETIME, updated_at DATETIME)`,
		`INSERT INTO pool_allowed_workspaces (pool_id, workspace_id, status) VALUES ('pool-1','ws-1','active'),('pool-2','ws-1','active'),('pool-1','ws-2','active')`,
		`INSERT INTO agents VALUES ('agent-a','pool-1','[]'),('agent-b','pool-1','[]'),('agent-c','pool-1','["agent_identity_header_v1"]'),('agent-x','pool-2','[]')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	for tok, pool := range map[string]string{tokPool1: "pool-1", tokPool2: "pool-2"} {
		h := sha256.Sum256([]byte(tok))
		if err := db.Exec(`INSERT INTO pool_tokens (token_hash, token_name, token_type, pool_id, is_active) VALUES (?,?,?,?,1)`,
			base64.StdEncoding.EncodeToString(h[:]), "n", models.PoolTokenTypeStatic, pool).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	db.Exec(`INSERT INTO workspace_tasks VALUES (10,'ws-1','agent-b','running',NULL,?)`, now)
	db.Exec(`INSERT INTO workspace_tasks VALUES (11,'ws-2','agent-b','success',?,?)`, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	db.Exec(`INSERT INTO workspace_tasks VALUES (12,'ws-2','agent-c','running',NULL,?)`, now)

	r := gin.New()
	r.Use(gin.Recovery())
	setupAgentAPIRoutes(r.Group("/api/v1"), db, nil, nil, nil, nil, nil)
	return r, db
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

func agentCall(r *gin.Engine, method, path, token, agentID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if agentID != "" {
		req.Header.Set(models.AgentIDHeader, agentID)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Agent A (same pool, own header) and an agent of another pool that is also
// allowed on the workspace both get 403 on every task and workspace route of
// a task running on agent B; agent B passes the ownership check.
func TestAgentRoutes_RejectNonOwningAgent(t *testing.T) {
	r, _ := setupAgentOwnershipRouter(t)
	for _, rt := range agentScopedRoutes(t, r, "10", "ws-1") {
		if w := agentCall(r, rt[0], rt[1], tokPool1, "agent-a"); w.Code != http.StatusForbidden {
			t.Errorf("%s %s as agent-a: want 403, got %d %s", rt[0], rt[1], w.Code, w.Body.String())
		}
		if w := agentCall(r, rt[0], rt[1], tokPool2, ""); w.Code != http.StatusForbidden {
			t.Errorf("%s %s from pool-2: want 403, got %d %s", rt[0], rt[1], w.Code, w.Body.String())
		}
		if w := agentCall(r, rt[0], rt[1], tokPool2, "agent-b"); w.Code != http.StatusForbidden {
			t.Errorf("%s %s from pool-2 claiming agent-b: want 403, got %d", rt[0], rt[1], w.Code)
		}
		for _, id := range []string{"agent-b", ""} { // legacy agent: pool binding only
			if w := agentCall(r, rt[0], rt[1], tokPool1, id); w.Code == http.StatusForbidden || w.Code == http.StatusConflict {
				t.Errorf("%s %s as owner %q: ownership check must pass, got %d %s", rt[0], rt[1], id, w.Code, w.Body.String())
			}
		}
	}
}

// A task the agent has ended long ago: task routes answer 409; an
// identity-capable agent must send its header.
func TestAgentRoutes_StateAndIdentityHeader(t *testing.T) {
	r, _ := setupAgentOwnershipRouter(t)
	for _, p := range []string{"/api/v1/agents/tasks/11/data", "/api/v1/agents/tasks/11/plan-task"} {
		if w := agentCall(r, "GET", p, tokPool1, "agent-b"); w.Code != http.StatusConflict {
			t.Errorf("GET %s on ended task: want 409, got %d %s", p, w.Code, w.Body.String())
		}
	}
	if w := agentCall(r, "PUT", "/api/v1/agents/tasks/11/status", tokPool1, "agent-b"); w.Code != http.StatusConflict {
		t.Errorf("status update 2h after completion: want 409, got %d", w.Code)
	}
	// agent-c reported agent_identity_header_v1: no header → 403, header → passes
	if w := agentCall(r, "GET", "/api/v1/agents/tasks/12/data", tokPool1, ""); w.Code != http.StatusForbidden {
		t.Errorf("identity-capable agent without header: want 403, got %d", w.Code)
	}
	if w := agentCall(r, "GET", "/api/v1/agents/tasks/12/data", tokPool1, "agent-c"); w.Code == http.StatusForbidden || w.Code == http.StatusConflict {
		t.Errorf("agent-c with header: got %d %s", w.Code, w.Body.String())
	}
	// ws-2: only agent-c's running task counts; agent-b's task ended 2h ago
	if w := agentCall(r, "GET", "/api/v1/agents/workspaces/ws-2/state/max-version", tokPool1, "agent-b"); w.Code != http.StatusForbidden {
		t.Errorf("workspace route for an agent without an active task: want 403, got %d", w.Code)
	}
	if w := agentCall(r, "GET", "/api/v1/agents/workspaces/ws-2/state/max-version", tokPool1, ""); w.Code != http.StatusForbidden {
		t.Errorf("headerless call must not ride on an identity-capable agent's task: want 403, got %d", w.Code)
	}
}

// Within the grace period the task's agent may still report (status retry of
// the same final status, final logs) but not change the final status.
func TestAgentRoutes_ReportingGrace(t *testing.T) {
	r, db := setupAgentOwnershipRouter(t)
	now := time.Now()
	db.Exec(`UPDATE workspace_tasks SET status = 'applied', completed_at = ? WHERE id = 10`, now.Add(-time.Minute))
	if w := agentCall(r, "GET", "/api/v1/agents/tasks/10/data", tokPool1, "agent-b"); w.Code != http.StatusConflict {
		t.Errorf("task data after completion: want 409, got %d", w.Code)
	}
	if w := agentCall(r, "POST", "/api/v1/agents/tasks/10/logs/chunk", tokPool1, "agent-b"); w.Code == http.StatusForbidden || w.Code == http.StatusConflict {
		t.Errorf("final log chunk within grace: got %d %s", w.Code, w.Body.String())
	}
	if w := agentCall(r, "POST", "/api/v1/agents/workspaces/ws-1/unlock", tokPool1, "agent-b"); w.Code == http.StatusForbidden {
		t.Errorf("post-apply unlock within grace: got %d %s", w.Code, w.Body.String())
	}
	if w := agentCall(r, "POST", "/api/v1/agents/tasks/10/logs/chunk", tokPool1, "agent-a"); w.Code != http.StatusForbidden {
		t.Errorf("other agent within grace: want 403, got %d", w.Code)
	}
}
