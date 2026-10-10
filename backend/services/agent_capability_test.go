package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

// The agent reports its version and this build's capabilities at registration.
func TestAgentAPIClient_RegisterReportsCapabilities(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"agent_id":"agent-1","pool_id":"pool-1"}`))
	}))
	defer srv.Close()
	id, pool, err := NewAgentAPIClient(srv.URL, "tok").Register("a")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", id)
	assert.Equal(t, "pool-1", pool)
	caps, _ := body["capabilities"].([]interface{})
	require.Len(t, caps, 2)
	assert.Equal(t, models.AgentCapabilityManifestBundleV1, caps[0])
	assert.NotEmpty(t, body["version"])
}

func TestAgentCapabilities_KnownAndMissing(t *testing.T) {
	assert.Equal(t, []string{models.AgentCapabilityManifestBundleV1}, models.KnownAgentCapabilities([]string{"bogus", models.AgentCapabilityManifestBundleV1, models.AgentCapabilityManifestBundleV1}))
	assert.Nil(t, models.KnownAgentCapabilities(nil))
	s := `["manifest_bundle_v1"]`
	a := &models.Agent{Capabilities: &s}
	assert.Equal(t, []string{models.AgentCapabilityTaskDataOverridesV1}, a.MissingCapabilities(models.ManifestAgentCapabilities()))
	assert.Equal(t, models.ManifestAgentCapabilities(), (&models.Agent{}).MissingCapabilities(models.ManifestAgentCapabilities()))
}
