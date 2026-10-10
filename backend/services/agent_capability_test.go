package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"iac-platform/internal/models"
)

func setAgentCaps(t *testing.T, db *gorm.DB, agentID string, caps []string) {
	t.Helper()
	var v interface{}
	if caps != nil {
		b, _ := json.Marshal(caps)
		v = string(b)
	}
	require.NoError(t, db.Exec(`UPDATE agents SET capabilities = ? WHERE agent_id = ?`, v, agentID).Error)
}

func manifestWS(id, pool string) *models.Workspace {
	dep, tag := "mfd-1", "v1"
	return &models.Workspace{WorkspaceID: id, ExecutionMode: models.ExecutionModeAgent, CurrentPoolID: &pool,
		ManifestDeploymentID: &dep, ManifestActiveTag: &tag}
}

// Only outdated agents in the pool: a manifest-bound task fails with
// agent_upgrade_required (reason = first missing capability), nothing is sent.
func TestPushTaskToAgent_ManifestBound_OldAgentsOnly_Fails(t *testing.T) {
	db := setupTestDB(t)
	pool := "pool-cap-1"
	createTestWorkspace(t, db, "ws-cap-1", func(ws *testWorkspace) { ws.CurrentPoolID = &pool })
	task := createTestTask(t, db, "ws-cap-1", models.TaskTypePlan, models.TaskStatusPending)
	createTestAgent(t, db, "agent-old", pool) // reports no capabilities
	setAgentCaps(t, db, "agent-old", nil)
	h := &mockAgentCCHandler{connectedAgents: []string{"agent-old"}}

	require.NoError(t, newTestManager(db, h, nil).pushTaskToAgent(task, manifestWS("ws-cap-1", pool)))
	assert.Empty(t, h.getSentTasks())
	var got models.WorkspaceTask
	require.NoError(t, db.First(&got, task.ID).Error)
	assert.Equal(t, models.TaskStatusFailed, got.Status)
	assert.Equal(t, models.TaskErrorCodeAgentUpgradeRequired, got.ErrorCode)
	assert.Equal(t, models.AgentCapabilityManifestBundleV1, got.ErrorReason)
	assert.Contains(t, got.ErrorMessage, "agent_upgrade_required:")
}

// An agent with the manifest capabilities but without per-agent tokens
// (agent_token_v1) is still outdated for manifest-bound tasks.
func TestPushTaskToAgent_ManifestBound_NoAgentToken_Fails(t *testing.T) {
	db := setupTestDB(t)
	pool := "pool-cap-tok"
	createTestWorkspace(t, db, "ws-cap-tok", func(ws *testWorkspace) { ws.CurrentPoolID = &pool })
	task := createTestTask(t, db, "ws-cap-tok", models.TaskTypePlan, models.TaskStatusPending)
	createTestAgent(t, db, "agent-pooltok", pool)
	setAgentCaps(t, db, "agent-pooltok", []string{models.AgentCapabilityManifestBundleV1, models.AgentCapabilityTaskDataOverridesV1})
	h := &mockAgentCCHandler{connectedAgents: []string{"agent-pooltok"}}

	require.NoError(t, newTestManager(db, h, nil).pushTaskToAgent(task, manifestWS("ws-cap-tok", pool)))
	assert.Empty(t, h.getSentTasks())
	var got models.WorkspaceTask
	require.NoError(t, db.First(&got, task.ID).Error)
	assert.Equal(t, models.TaskErrorCodeAgentUpgradeRequired, got.ErrorCode)
	assert.Equal(t, models.AgentCapabilityAgentTokenV1, got.ErrorReason)
}

// A capable agent is chosen over an outdated one; a capable-but-busy agent
// means wait (retry), not fail.
func TestPushTaskToAgent_ManifestBound_PicksCapableAgent(t *testing.T) {
	db := setupTestDB(t)
	pool := "pool-cap-2"
	createTestWorkspace(t, db, "ws-cap-2", func(ws *testWorkspace) { ws.CurrentPoolID = &pool })
	task := createTestTask(t, db, "ws-cap-2", models.TaskTypePlan, models.TaskStatusPending)
	createTestAgent(t, db, "agent-old", pool)
	createTestAgent(t, db, "agent-new", pool)
	setAgentCaps(t, db, "agent-old", []string{models.AgentCapabilityTaskDataOverridesV1}) // partial
	setAgentCaps(t, db, "agent-new", models.SupportedAgentCapabilities())

	busy := &mockAgentCCHandler{connectedAgents: []string{"agent-old", "agent-new"}, availableMap: map[string]bool{"agent-old": true, "agent-new": false}}
	require.NoError(t, newTestManager(db, busy, nil).pushTaskToAgent(task, manifestWS("ws-cap-2", pool)))
	assert.Empty(t, busy.getSentTasks())
	var got models.WorkspaceTask
	db.First(&got, task.ID)
	assert.Equal(t, models.TaskStatusPending, got.Status, "capable agent busy: retry, not fail")

	h := &mockAgentCCHandler{connectedAgents: []string{"agent-old", "agent-new"}}
	require.NoError(t, newTestManager(db, h, nil).pushTaskToAgent(task, manifestWS("ws-cap-2", pool)))
	sent := h.getSentTasks()
	require.Len(t, sent, 1)
	assert.Equal(t, "agent-new", sent[0].AgentID)
}

// Non-manifest workspaces are dispatched to any agent, as before.
func TestPushTaskToAgent_NonManifest_OldAgentUnchanged(t *testing.T) {
	db := setupTestDB(t)
	pool := "pool-cap-3"
	createTestWorkspace(t, db, "ws-cap-3", func(ws *testWorkspace) { ws.CurrentPoolID = &pool })
	task := createTestTask(t, db, "ws-cap-3", models.TaskTypePlan, models.TaskStatusPending)
	createTestAgent(t, db, "agent-old", pool)
	setAgentCaps(t, db, "agent-old", nil)
	h := &mockAgentCCHandler{connectedAgents: []string{"agent-old"}}
	ws := &models.Workspace{WorkspaceID: "ws-cap-3", ExecutionMode: models.ExecutionModeAgent, CurrentPoolID: &pool}

	require.NoError(t, newTestManager(db, h, nil).pushTaskToAgent(task, ws))
	require.Len(t, h.getSentTasks(), 1)
}

func TestRequiredAgentCapabilities(t *testing.T) {
	plain := &models.Workspace{WorkspaceID: "w"}
	assert.Nil(t, requiredAgentCapabilities(&models.WorkspaceTask{}, plain))
	run := &models.WorkspaceTask{ExternalFiles: models.JSONB{"files": []interface{}{map[string]interface{}{"path": "main.tf"}}}}
	assert.Equal(t, models.ManifestAgentCapabilities(), requiredAgentCapabilities(run, plain), "manifest Run task")
	ovr := &models.WorkspaceTask{VariableOverrides: models.JSONB{"a": "b"}}
	assert.Equal(t, models.ManifestAgentCapabilities(), requiredAgentCapabilities(ovr, plain), "task with overrides")
	assert.Equal(t, models.ManifestAgentCapabilities(), requiredAgentCapabilities(&models.WorkspaceTask{}, manifestWS("w", "p")))
}

// The agent reports its version and this build's capabilities at
// registration (with the pool token), then uses the agent token it got back;
// no X-Agent-ID header.
func TestAgentAPIClient_RegisterReportsCapabilities(t *testing.T) {
	var body map[string]interface{}
	var lastAuth, lastAgentHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth, lastAgentHeader = r.Header.Get("Authorization"), r.Header.Get("X-Agent-ID")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/agents/register" {
			json.NewDecoder(r.Body).Decode(&body)
			w.Write([]byte(`{"agent_id":"agent-1","pool_id":"pool-1","agent_token":"a.b.c","agent_token_expires_at":"` + time.Now().Add(15*time.Minute).UTC().Format(time.RFC3339) + `"}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client := NewAgentAPIClient(srv.URL, "apt_pool")
	id, pool, err := client.Register("a")
	require.NoError(t, err)
	assert.Equal(t, "Bearer apt_pool", lastAuth, "registration uses the pool token")
	assert.Equal(t, "agent-1", id)
	assert.Equal(t, "pool-1", pool)
	caps, _ := body["capabilities"].([]interface{})
	require.Len(t, caps, 4)
	assert.Equal(t, models.AgentCapabilityManifestBundleV1, caps[0])
	assert.Contains(t, caps, models.AgentCapabilityAgentTokenV1)
	assert.Contains(t, caps, models.AgentCapabilityManifestApprovalV1)
	assert.NotEmpty(t, body["version"])
	_, _ = client.GetTaskData(7)
	assert.Equal(t, "Bearer a.b.c", lastAuth, "later calls use the agent token")
	assert.Empty(t, lastAgentHeader)
	// registration again (re-register) still uses the pool token
	_, _, _ = client.Register("a")
	assert.Equal(t, "Bearer apt_pool", lastAuth)
}

// The agent token is renewed when a third of its lifetime is left; a 401 on
// renewal loses it at once (revoked agent) and calls OnAgentTokenLost.
func TestAgentAPIClient_AgentTokenRenewal(t *testing.T) {
	refuse := false
	renewals := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/agents/token" {
			renewals++
			assert.Equal(t, "Bearer old.tok.en", r.Header.Get("Authorization"))
			if refuse {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"agent token revoked"}`))
				return
			}
			w.Write([]byte(`{"agent_token":"new.tok.en","agent_token_expires_at":"` + time.Now().Add(15*time.Minute).UTC().Format(time.RFC3339) + `"}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client := NewAgentAPIClient(srv.URL, "apt_pool")
	set := func() {
		client.setAgentToken("old.tok.en", time.Now().Add(15*time.Minute).UTC().Format(time.RFC3339))
		client.tokenMu.Lock()
		client.agentTokenExp = time.Now().Add(2 * time.Minute) // < 1/3 of 15m left
		client.tokenMu.Unlock()
	}
	set()
	tok, err := client.BearerToken()
	require.NoError(t, err)
	assert.Equal(t, "new.tok.en", tok)
	assert.Equal(t, 1, renewals)

	set()
	refuse = true
	var lost error
	client.OnAgentTokenLost = func(err error) { lost = err }
	_, err = client.BearerToken()
	assert.ErrorIs(t, err, ErrAgentTokenLost)
	assert.Error(t, lost)
}

func TestAgentCapabilities_KnownAndMissing(t *testing.T) {
	assert.Equal(t, []string{models.AgentCapabilityManifestBundleV1}, models.KnownAgentCapabilities([]string{"bogus", models.AgentCapabilityManifestBundleV1, models.AgentCapabilityManifestBundleV1}))
	assert.Nil(t, models.KnownAgentCapabilities(nil))
	s := `["manifest_bundle_v1"]`
	a := &models.Agent{Capabilities: &s}
	assert.Equal(t, []string{models.AgentCapabilityTaskDataOverridesV1, models.AgentCapabilityAgentTokenV1, models.AgentCapabilityManifestApprovalV1}, a.MissingCapabilities(models.ManifestAgentCapabilities()))
	assert.False(t, a.HasCapability(models.AgentCapabilityAgentTokenV1))
	// the retired header capability is not known any more
	assert.Nil(t, models.KnownAgentCapabilities([]string{"agent_identity_header_v1"}))
	assert.True(t, a.HasCapability(models.AgentCapabilityManifestBundleV1))
	assert.Equal(t, models.ManifestAgentCapabilities(), (&models.Agent{}).MissingCapabilities(models.ManifestAgentCapabilities()))
}

// Run tokens are obtained with the agent token, bound to the run ID; without
// an agent token (dev legacy mode) the client does not ask.
func TestAgentAPIClient_ObtainRunToken(t *testing.T) {
	var path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"run_id":"mfr-1","run_token":"r.u.n","expires_at":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}`))
	}))
	defer srv.Close()
	client := NewAgentAPIClient(srv.URL, "apt_pool")
	_, _, err := client.ObtainRunToken("mfr-1")
	require.Error(t, err, "no agent token: no run token")
	client.setAgentToken("a.g.t", time.Now().Add(15*time.Minute).UTC().Format(time.RFC3339))
	tok, exp, err := client.ObtainRunToken("mfr-1")
	require.NoError(t, err)
	assert.Equal(t, "r.u.n", tok)
	assert.True(t, exp.After(time.Now()))
	assert.Equal(t, "/api/v1/agents/runs/mfr-1/token", path)
	assert.Equal(t, "Bearer a.g.t", auth)
}
