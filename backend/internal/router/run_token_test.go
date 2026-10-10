package router

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/keys"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

type runTokenEnv struct {
	r   *gin.Engine
	db  *gorm.DB
	svc *services.StateTokenService
}

// Agent routes + a state backend with stub handlers. Runs (ws-1):
// mfr-appr approval on agent-b; mfr-prev / mfr-prev2 preview on agent-b in
// session sess-1 (expires in 30m); mfr-a approval on agent-a; mfr-sbx a
// sandbox preview run of sess-1.
func setupRunTokenEnv(t *testing.T) *runTokenEnv {
	t.Helper()
	_, db := setupAgentOwnershipRouter(t)
	now := time.Now()
	for _, stmt := range []string{
		`CREATE TABLE sandbox_sessions (id TEXT PRIMARY KEY, user_id TEXT, workspace_id TEXT, provider TEXT, network_mode TEXT DEFAULT 'vpc', status TEXT DEFAULT 'active', expires_at DATETIME, closed_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE manifest_runs (id TEXT PRIMARY KEY, manifest_id TEXT, version_id TEXT, bundle_hash TEXT, workspace_id TEXT, runner TEXT, purpose TEXT, status TEXT, plan_hash TEXT, plan_redacted TEXT, state_serial INTEGER, session_id TEXT, agent_id TEXT, created_by TEXT, created_at DATETIME, updated_at DATETIME)`,
		`ALTER TABLE workspace_tasks ADD COLUMN state_token_hash TEXT`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(`INSERT INTO sandbox_sessions (id, user_id, workspace_id, provider, status, expires_at) VALUES ('sess-1','u1','ws-1','agentcore','active',?)`, now.Add(30*time.Minute))
	for _, r := range [][6]interface{}{
		{"mfr-appr", "agent", "approval", nil, "agent-b", now.Add(-10 * time.Minute)},
		{"mfr-prev", "agent", "preview", "sess-1", "agent-b", now},
		{"mfr-prev2", "agent", "preview", "sess-1", "agent-b", now},
		{"mfr-a", "agent", "approval", nil, "agent-a", now},
		{"mfr-sbx", "sandbox", "preview", "sess-1", nil, now},
	} {
		if err := db.Exec(`INSERT INTO manifest_runs (id, manifest_id, bundle_hash, workspace_id, runner, purpose, status, session_id, agent_id, created_by, created_at) VALUES (?,'mf-1',?,'ws-1',?,?,'running',?,?,'u-creator',?)`,
			r[0], strings.Repeat("a", 64), r[1], r[2], r[3], r[4], r[5]).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := services.NewStateTokenService(db)
	r := gin.New()
	api := r.Group("/api/v1")
	setupAgentAPIRoutes(api, db, nil, nil, nil, nil, svc)
	ok := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	st := api.Group("/terraform/state", middleware.StateTokenAuth(svc))
	st.GET("/:workspace_id", ok)
	st.POST("/:workspace_id", ok)
	st.DELETE("/:workspace_id", ok)
	st.POST("/:workspace_id/lock", ok)
	st.POST("/:workspace_id/unlock", ok)
	return &runTokenEnv{r: r, db: db, svc: svc}
}

func (e *runTokenEnv) obtain(t *testing.T, bearer, runID string) (int, string, time.Time) {
	t.Helper()
	w := agentCall(e.r, "POST", "/api/v1/agents/runs/"+runID+"/token", bearer)
	var body struct {
		Token string `json:"run_token"`
		Exp   string `json:"expires_at"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	exp, _ := time.Parse(time.RFC3339, body.Exp)
	return w.Code, body.Token, exp
}

func (e *runTokenEnv) mustObtain(t *testing.T, agentID, runID string) string {
	t.Helper()
	code, tok, _ := e.obtain(t, agentToken(t, agentID, "pool-1"), runID)
	if code != http.StatusOK || tok == "" {
		t.Fatalf("obtain %s as %s: %d", runID, agentID, code)
	}
	return tok
}

func (e *runTokenEnv) state(method, path, token string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("x:"+token)))
	w := httptest.NewRecorder()
	e.r.ServeHTTP(w, req)
	return w.Code
}

var stateCalls = [][2]string{
	{"GET", "/api/v1/terraform/state/ws-1"},
	{"POST", "/api/v1/terraform/state/ws-1"},
	{"POST", "/api/v1/terraform/state/ws-1/lock"},
	{"POST", "/api/v1/terraform/state/ws-1/unlock"},
	{"DELETE", "/api/v1/terraform/state/ws-1"},
}

// Only the run's assigned agent, with its agent token, obtains a run token.
func TestRunToken_Issuance(t *testing.T) {
	e := setupRunTokenEnv(t)
	tokB := agentToken(t, "agent-b", "pool-1")
	if code, tok, exp := e.obtain(t, tokB, "mfr-appr"); code != http.StatusOK || tok == "" ||
		exp.Sub(time.Now().Add(110*time.Minute)).Abs() > time.Minute {
		t.Fatalf("approval run: %d exp=%v (want created_at+2h)", code, exp)
	}
	if _, _, exp := e.obtain(t, tokB, "mfr-prev"); exp.Sub(time.Now().Add(30*time.Minute)).Abs() > time.Minute {
		t.Fatalf("preview run in a session: exp %v, want the session expiry", exp)
	}
	for name, tc := range map[string][2]string{
		"other agent's run": {"agent-b", "mfr-a"},
		"not assigned":      {"agent-a", "mfr-appr"},
		"sandbox run":       {"agent-b", "mfr-sbx"},
		"unknown run":       {"agent-b", "mfr-none"},
	} {
		if code, _, _ := e.obtain(t, agentToken(t, tc[0], "pool-1"), tc[1]); code != http.StatusForbidden {
			t.Errorf("%s: want 403, got %d", name, code)
		}
	}
	if code, _, _ := e.obtain(t, tokPool1, "mfr-appr"); code != http.StatusUnauthorized {
		t.Errorf("pool token: want 401, got %d", code)
	}
	e.db.Exec(`UPDATE manifest_runs SET created_at = ? WHERE id = 'mfr-appr'`, time.Now().Add(-3*time.Hour))
	if code, _, _ := e.obtain(t, tokB, "mfr-appr"); code != http.StatusForbidden {
		t.Errorf("timed-out run: want 403, got %d", code)
	}
	e.db.Exec(`UPDATE manifest_runs SET status = 'succeeded' WHERE id = 'mfr-prev'`)
	if code, _, _ := e.obtain(t, tokB, "mfr-prev"); code != http.StatusForbidden {
		t.Errorf("ended run: want 403, got %d", code)
	}
}

// Approval tokens use the state backend of their workspace; preview tokens
// only read; neither crosses workspaces.
func TestRunToken_StateAccess(t *testing.T) {
	e := setupRunTokenEnv(t)
	appr, prev := e.mustObtain(t, "agent-b", "mfr-appr"), e.mustObtain(t, "agent-b", "mfr-prev")
	for _, c := range stateCalls {
		if got := e.state(c[0], c[1], appr); got != http.StatusOK {
			t.Errorf("approval %s %s: want 200, got %d", c[0], c[1], got)
		}
		want := http.StatusForbidden
		if c[0] == "GET" {
			want = http.StatusOK
		}
		if got := e.state(c[0], c[1], prev); got != want {
			t.Errorf("preview %s %s: want %d, got %d", c[0], c[1], want, got)
		}
	}
	if got := e.state("GET", "/api/v1/terraform/state/ws-2", appr); got != http.StatusForbidden {
		t.Errorf("run token on another workspace: want 403, got %d", got)
	}
}

// Run end revokes that run's token only; session end revokes all of the
// session's tokens and its STS credentials; an expired session refuses at
// once and the sweep revokes.
func TestRunToken_Revocation(t *testing.T) {
	e := setupRunTokenEnv(t)
	ctx := context.Background()
	appr, prev, prev2 := e.mustObtain(t, "agent-b", "mfr-appr"), e.mustObtain(t, "agent-b", "mfr-prev"), e.mustObtain(t, "agent-b", "mfr-prev2")

	if err := services.EndManifestRun(ctx, e.db, "mfr-prev", models.ManifestRunStatusSucceeded); err != nil {
		t.Fatal(err)
	}
	if e.state("GET", stateCalls[0][1], prev) != http.StatusUnauthorized || e.state("GET", stateCalls[0][1], prev2) != http.StatusOK ||
		e.state("GET", stateCalls[0][1], appr) != http.StatusOK {
		t.Fatal("run end must revoke only that run's token")
	}

	rec := &recordingRevoker{}
	old := services.SessionCredentials
	services.SessionCredentials = rec
	defer func() { services.SessionCredentials = old }()
	if err := services.EndSandboxSession(ctx, e.db, "sess-1", models.SandboxSessionStatusClosed); err != nil {
		t.Fatal(err)
	}
	if e.state("GET", stateCalls[0][1], prev2) != http.StatusUnauthorized || e.state("GET", stateCalls[0][1], appr) != http.StatusOK {
		t.Fatal("session end must revoke the session's tokens only")
	}
	if len(rec.sessions) != 1 || rec.sessions[0] != "sess-1" {
		t.Fatalf("STS revocation hook: %v", rec.sessions)
	}
	var open int64
	e.db.Table("run_tokens").Where("session_id = 'sess-1' AND revoked_at IS NULL").Count(&open)
	if open != 0 {
		t.Fatalf("%d session tokens left unrevoked", open)
	}
}

func TestRunToken_SessionExpiry(t *testing.T) {
	e := setupRunTokenEnv(t)
	prev := e.mustObtain(t, "agent-b", "mfr-prev")
	e.db.Exec(`UPDATE sandbox_sessions SET expires_at = ? WHERE id = 'sess-1'`, time.Now().Add(-time.Second))
	if got := e.state("GET", stateCalls[0][1], prev); got != http.StatusUnauthorized {
		t.Fatalf("expired session: want 401, got %d", got)
	}
	rec := &recordingRevoker{}
	old := services.SessionCredentials
	services.SessionCredentials = rec
	defer func() { services.SessionCredentials = old }()
	if n, err := services.ExpireSandboxSessions(context.Background(), e.db, time.Now()); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	var status string
	e.db.Raw(`SELECT status FROM sandbox_sessions WHERE id = 'sess-1'`).Scan(&status)
	var open int64
	e.db.Table("run_tokens").Where("session_id = 'sess-1' AND revoked_at IS NULL").Count(&open)
	if status != models.SandboxSessionStatusExpired || open != 0 || len(rec.sessions) != 1 {
		t.Fatalf("expired session: status=%s open=%d sts=%v", status, open, rec.sessions)
	}
}

// Revoking or deregistering the agent, or revoking its pool token, kills its
// run tokens; a DB error refuses (503).
func TestRunToken_AgentRevocationAndDBError(t *testing.T) {
	cases := map[string]struct {
		act  func(e *runTokenEnv)
		want int
	}{
		"agent revoked": {func(e *runTokenEnv) {
			if err := services.RevokeAgentTokens(context.Background(), e.db, "agent-b"); err != nil {
				panic(err)
			}
		}, http.StatusUnauthorized},
		"agent deregistered": {func(e *runTokenEnv) { e.db.Exec(`DELETE FROM agents WHERE agent_id = 'agent-b'`) }, http.StatusUnauthorized},
		"pool token revoked": {func(e *runTokenEnv) { e.db.Exec(`UPDATE pool_tokens SET is_active = 0 WHERE pool_id = 'pool-1'`) }, http.StatusUnauthorized},
		"db error":           {func(e *runTokenEnv) { e.db.Exec(`ALTER TABLE run_tokens RENAME TO run_tokens_gone`) }, http.StatusServiceUnavailable},
		"agents db error":    {func(e *runTokenEnv) { e.db.Exec(`ALTER TABLE agents RENAME TO agents_gone`) }, http.StatusServiceUnavailable},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupRunTokenEnv(t)
			tok := e.mustObtain(t, "agent-b", "mfr-appr")
			tc.act(e)
			if got := e.state("GET", stateCalls[0][1], tok); got != tc.want {
				t.Errorf("want %d, got %d", tc.want, got)
			}
		})
	}
	e := setupRunTokenEnv(t)
	e.mustObtain(t, "agent-b", "mfr-appr")
	_ = services.RevokeAgentTokens(context.Background(), e.db, "agent-b")
	var open int64
	e.db.Table("run_tokens").Where("agent_id = 'agent-b' AND revoked_at IS NULL").Count(&open)
	if open != 0 {
		t.Fatalf("agent revocation must revoke its run tokens, %d left", open)
	}
}

// typ selects the verification key: run claims under the state key, task
// claims under the run key are refused; task tokens (typ task or none) keep
// working, including cross-workspace reads.
func TestRunToken_TypSeparation(t *testing.T) {
	e := setupRunTokenEnv(t)
	exp := jwt.NewNumericDate(time.Now().Add(time.Hour))
	forged, _ := keys.Sign(keys.PurposeState, services.RunTokenClaims{Type: "run", RunID: "mfr-appr", WorkspaceID: "ws-1", Purpose: "approval",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp}}, nil)
	if got := e.state("POST", stateCalls[1][1], forged); got != http.StatusUnauthorized {
		t.Errorf("run claims signed with the state key: want 401, got %d", got)
	}
	taskUnderRunKey, _ := keys.Sign(keys.PurposeRun, services.StateTokenClaims{Type: "task", WorkspaceID: "ws-1", TaskID: 10,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp}}, nil)
	e.db.Exec(`UPDATE workspace_tasks SET state_token_hash = ? WHERE id = 10`, sha256Hex(taskUnderRunKey))
	if got := e.state("GET", stateCalls[0][1], taskUnderRunKey); got != http.StatusUnauthorized {
		t.Errorf("task claims signed with the run key: want 401, got %d", got)
	}

	task, err := e.svc.GenerateToken("ws-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range stateCalls {
		if got := e.state(c[0], c[1], task); got != http.StatusOK {
			t.Errorf("task token %s %s: want 200, got %d", c[0], c[1], got)
		}
	}
	if got := e.state("GET", "/api/v1/terraform/state/ws-2", task); got != http.StatusOK {
		t.Errorf("task token cross-workspace read unchanged: got %d", got)
	}
	untyped, _ := keys.Sign(keys.PurposeState, services.StateTokenClaims{WorkspaceID: "ws-1", TaskID: 10,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: exp}}, nil)
	e.db.Exec(`UPDATE workspace_tasks SET state_token_hash = ? WHERE id = 10`, sha256Hex(untyped))
	if got := e.state("GET", stateCalls[0][1], untyped); got != http.StatusOK {
		t.Errorf("task token without typ: want 200, got %d", got)
	}
}

type recordingRevoker struct{ sessions []string }

func (r *recordingRevoker) RevokeSessionCredentials(_ context.Context, s *models.SandboxSession) error {
	r.sessions = append(r.sessions, s.ID)
	return nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
