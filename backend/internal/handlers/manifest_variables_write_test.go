package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
)

// resourcesOnly: WORKSPACE_RESOURCES WRITE (+ read access), no WORKSPACE_VARIABLES WRITE.
var resourcesOnly = map[valueobject.ResourceType]valueobject.PermissionLevel{
	valueobject.ResourceTypeAllWorkspaces:      valueobject.PermissionLevelRead,
	valueobject.ResourceTypeWorkspaceVars:      valueobject.PermissionLevelRead,
	valueobject.ResourceTypeWorkspaceResources: valueobject.PermissionLevelWrite,
}

func variablesWriteRouter(t *testing.T, levels map[valueobject.ResourceType]valueobject.PermissionLevel) *gin.Engine {
	t.Helper()
	db := setupOverrideDB(t) // mfd-a on ws-a, stored varsets [vs-proj@1]
	for _, stmt := range []string{
		`INSERT INTO variable_sets (varset_id, name, scope) VALUES ('vs-two', 't', 'specific')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-two', 'project', 10)`,
		`INSERT INTO workspaces (id, workspace_id) VALUES (2, 'ws-new')`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	h := NewManifestDeploymentsV2Handler(db, middleware.NewIAMPermissionMiddlewareWithChecker(&resourceChecker{levels: levels}))
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.POST("/organizations/:org_id/manifests/:id/v2/deployments/install", withCaller(valueobject.PermissionLevelRead), h.Install)
	r.POST("/organizations/:org_id/manifests/:id/v2/deployments/:deployment_id/upgrade", withCaller(valueobject.PermissionLevelRead), h.Upgrade)
	return r
}

const upgradePath = "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/upgrade"

func TestUpgrade_VariableChangesRequireWorkspaceVariablesWrite(t *testing.T) {
	r := variablesWriteRouter(t, resourcesOnly)
	for name, body := range map[string]string{
		"override":         `{"target_version_id":"mfv-2","varsets":[{"varset_id":"vs-proj","priority":1}],"variable_overrides":{"region":"x"}}`,
		"unset_keys":       `{"target_version_id":"mfv-2","varsets":[{"varset_id":"vs-proj","priority":1}],"unset_keys":["region"]}`,
		"varset added":     `{"target_version_id":"mfv-2","varsets":[{"varset_id":"vs-proj","priority":1},{"varset_id":"vs-two","priority":2}]}`,
		"priority changed": `{"target_version_id":"mfv-2","varsets":[{"varset_id":"vs-proj","priority":5}]}`,
		"varset replaced":  `{"target_version_id":"mfv-2","varsets":[{"varset_id":"vs-two","priority":1}]}`,
		"varsets omitted":  `{"target_version_id":"mfv-2"}`,
		"varsets cleared":  `{"target_version_id":"mfv-2","varsets":[]}`,
	} {
		if w := doJSON(r, "POST", upgradePath, body); w.Code != http.StatusForbidden {
			t.Fatalf("%s: resources-only WRITE must get 403, got %d %s", name, w.Code, w.Body.String())
		}
	}
}

func TestUpgrade_EchoedVarsetsVersionOnlyKeepsResourcesRule(t *testing.T) {
	r := variablesWriteRouter(t, resourcesOnly)
	body := `{"target_version_id":"mfv-2","varsets":[{"varset_id":"vs-proj","priority":1}],"variable_overrides":{}}`
	if w := doJSON(r, "POST", upgradePath, body); w.Code != http.StatusOK {
		t.Fatalf("echoed varsets + version change must pass with resources WRITE: %d %s", w.Code, w.Body.String())
	}
}

// order among equal priorities is part of the effective list
func TestDeploymentVarsetsChanged_ComparesEffectiveOrder(t *testing.T) {
	db := setupOverrideDB(t)
	db.Exec(`INSERT INTO manifest_deployment_varsets (id, deployment_id, varset_id, priority) VALUES (2, 'mfd-a', 'vs-two', 1)`)
	db.Exec(`UPDATE manifest_deployment_varsets SET id = 1 WHERE varset_id = 'vs-proj'`)
	h := NewManifestDeploymentsV2Handler(db, nil)
	cases := map[string]struct {
		body    string
		changed bool
	}{
		"same order":        {`[{"varset_id":"vs-proj","priority":1},{"varset_id":"vs-two","priority":1}]`, false},
		"tie order swapped": {`[{"varset_id":"vs-two","priority":1},{"varset_id":"vs-proj","priority":1}]`, true},
	}
	for name, tc := range cases {
		var entries []struct {
			VarsetID string `json:"varset_id"`
			Priority int    `json:"priority"`
		}
		mustUnmarshal(t, tc.body, &entries)
		req := toVarsetEntries(entries)
		got, err := h.deploymentVarsetsChanged("mfd-a", req)
		if err != nil || got != tc.changed {
			t.Fatalf("%s: changed=%v err=%v, want %v", name, got, err, tc.changed)
		}
	}
}

func TestInstall_VarsetsRequireWorkspaceVariablesWrite(t *testing.T) {
	path := "/organizations/1/manifests/mf-1/v2/deployments/install"
	withVarsets := `{"version_id":"mfv-1","workspace_id":"ws-new","varsets":[{"varset_id":"vs-proj","priority":1}]}`
	withOverride := `{"version_id":"mfv-1","workspace_id":"ws-new","variable_overrides":{"region":"x"}}`
	plain := `{"version_id":"mfv-1","workspace_id":"ws-new"}`

	r := variablesWriteRouter(t, resourcesOnly)
	for name, body := range map[string]string{"varsets": withVarsets, "override": withOverride} {
		if w := doJSON(r, "POST", path, body); w.Code != http.StatusForbidden {
			t.Fatalf("first install with %s and no WORKSPACE_VARIABLES WRITE: want 403, got %d %s", name, w.Code, w.Body.String())
		}
	}
	if w := doJSON(r, "POST", path, plain); w.Code == http.StatusForbidden {
		t.Fatalf("install without variable changes keeps the resources rule: %d %s", w.Code, w.Body.String())
	}

	r = variablesWriteRouter(t, writeVars)
	if w := doJSON(r, "POST", path, withVarsets); w.Code == http.StatusForbidden {
		t.Fatalf("WORKSPACE_VARIABLES WRITE holder must pass the gate: %d %s", w.Code, w.Body.String())
	}
}

func mustUnmarshal(t *testing.T, s string, v interface{}) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatal(err)
	}
}

func toVarsetEntries(in []struct {
	VarsetID string `json:"varset_id"`
	Priority int    `json:"priority"`
}) []models.DeploymentVarsetEntry {
	out := make([]models.DeploymentVarsetEntry, len(in))
	for i, e := range in {
		out[i] = models.DeploymentVarsetEntry{VarsetID: e.VarsetID, Priority: e.Priority}
	}
	return out
}
