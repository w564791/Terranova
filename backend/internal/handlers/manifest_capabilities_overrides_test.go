package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
)

// --- capabilities -----------------------------------------------------------

func TestListManifests_TopLevelCapabilitiesAndCanAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		level  valueobject.PermissionLevel
		access *service.WorkspaceListAccess
		org    string
		want   models.ManifestCapabilities
	}{
		{"read, no deploy target", valueobject.PermissionLevelRead, &service.WorkspaceListAccess{HasAccess: true}, "1",
			models.ManifestCapabilities{CanRead: true}},
		{"write + deploy", valueobject.PermissionLevelWrite, &service.WorkspaceListAccess{FullOrganization: true, HasAccess: true}, "1",
			models.ManifestCapabilities{CanRead: true, CanWrite: true, CanDeploy: true}},
		{"admin", valueobject.PermissionLevelAdmin, &service.WorkspaceListAccess{HasAccess: true, WorkspaceIDs: []string{"ws-a"}}, "1",
			models.ManifestCapabilities{CanRead: true, CanWrite: true, CanAdmin: true, CanDeploy: true}},
		// org without manifests: capabilities are still returned with an empty list
		{"empty list", valueobject.PermissionLevelAdmin, &service.WorkspaceListAccess{HasAccess: true}, "77",
			models.ManifestCapabilities{CanRead: true, CanWrite: true, CanAdmin: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupManifestIAMDB(t)
			perm := middleware.NewIAMPermissionMiddlewareWithChecker(&resourceChecker{}).
				WithWorkspaceListAccess(&fakeWorkspaceListResolver{access: tc.access})
			h := NewManifestHandler(db, perm)
			r := gin.New()
			r.GET("/organizations/:org_id/manifests", withCaller(tc.level), h.ListManifests)
			r.GET("/organizations/:org_id/manifests/:id", withCaller(tc.level), h.GetManifest)

			w := doJSON(r, "GET", "/organizations/"+tc.org+"/manifests", "")
			if w.Code != http.StatusOK {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(w.Body.Bytes(), &raw)
			if string(raw["items"]) == "null" {
				t.Fatal("items must be an array, not null")
			}
			var body struct {
				Items        []map[string]interface{}     `json:"items"`
				Capabilities *models.ManifestCapabilities `json:"capabilities"`
			}
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if body.Capabilities == nil || *body.Capabilities != tc.want {
				t.Fatalf("capabilities = %+v, want %+v", body.Capabilities, tc.want)
			}
			for _, it := range body.Items {
				if it["can_admin"] != tc.want.CanAdmin || it["can_write"] != tc.want.CanWrite || it["can_deploy"] != tc.want.CanDeploy {
					t.Fatalf("item flags %v disagree with capabilities %+v", it, tc.want)
				}
			}
			if tc.org == "1" {
				var one map[string]interface{}
				_ = json.Unmarshal(doJSON(r, "GET", "/organizations/1/manifests/mf-1", "").Body.Bytes(), &one)
				if one["can_admin"] != tc.want.CanAdmin {
					t.Fatalf("get can_admin = %v, want %v", one["can_admin"], tc.want.CanAdmin)
				}
			}
		})
	}
}

func TestDeleteAndArchiveStayAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupManifestIAMDB(t)
	h := NewManifestHandler(db, nil)
	r := gin.New()
	r.PUT("/organizations/:org_id/manifests/:id", withCaller(valueobject.PermissionLevelWrite), h.UpdateManifest)
	if w := doJSON(r, "PUT", "/organizations/1/manifests/mf-1", `{"status":"archived"}`); w.Code != http.StatusForbidden {
		t.Fatalf("archive with WRITE: %d", w.Code)
	}
}

// --- upgrade override merge -------------------------------------------------

func TestMergeOverrides(t *testing.T) {
	got := mergeOverrides(
		map[string]string{"secret": "s1", "plain": "p1", "gone": "x", "kept": "k"},
		map[string]string{"secret": "", "plain": "", "new_secret": "", "added": "a"},
		map[string]bool{"secret": true, "new_secret": true},
		[]string{"gone", "missing"},
	)
	want := map[string]string{"secret": "s1", "plain": "", "kept": "k", "added": "a"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q want %q (got %v)", k, got[k], v, got)
		}
	}
	if got2 := mergeOverrides(map[string]string{"secret": "s1"}, map[string]string{"secret": "new"}, map[string]bool{"secret": true}, nil); got2["secret"] != "new" {
		t.Fatal("a non-empty sensitive value must replace the stored one")
	}
}

func TestUpgrade_MergesOverridesInsteadOfWiping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "test-jwt-secret-at-least-32-bytes-long!!") // variable decryption key
	db := setupManifestIAMDB(t)
	for _, stmt := range []string{
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, variables TEXT, changelog TEXT, created_by TEXT, created_at DATETIME)`,
		`INSERT INTO manifest_versions (id, manifest_id, version, created_by) VALUES ('mfv-1', 'mf-1', 'v1.0.0', 'u1'), ('mfv-2', 'mf-1', 'v1.1.0', 'u1')`,
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY AUTOINCREMENT, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary INTEGER, mode INTEGER, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT, manifest_subpath TEXT, manifest_active_tag TEXT, manifest_deployment_id TEXT, updated_at DATETIME)`,
		`INSERT INTO workspaces (id, workspace_id, manifest_deployment_id, manifest_active_tag) VALUES (1, 'ws-a', 'mfd-a', 'v1.0.0')`,
		`CREATE TABLE workspace_resources (id INTEGER PRIMARY KEY, workspace_id TEXT, resource_id TEXT, manifest_deployment_id TEXT)`,
		`ALTER TABLE manifest_deployment_varsets ADD COLUMN id INTEGER`,
		`ALTER TABLE manifest_deployment_varsets ADD COLUMN created_at DATETIME`,
		`INSERT INTO variable_sets (varset_id, name, scope) VALUES ('vs-proj', 'p', 'specific')`,
		`INSERT INTO varset_assignments (varset_id, scope_type, project_id) VALUES ('vs-proj', 'project', 10)`,
		`INSERT INTO varset_variables (variable_id, varset_id, key, value, variable_type, value_format, sensitive, version) VALUES
		   ('v2', 'vs-proj', 'db_password', 'from-varset', 'terraform', 'string', 1, 1)`,
		`INSERT INTO workspace_variables (variable_id, workspace_id, key, version, value, variable_type, value_format, sensitive) VALUES
		   ('wv1', 'ws-a', 'api_token', 1, 'tok', 'terraform', 'string', 1),
		   ('wv2', 'ws-a', 'other_secret', 1, 'real', 'terraform', 'string', 1)`,
		`UPDATE manifest_deployments SET variable_overrides = CAST('{"api_token":"stored-token","db_password":"stored-pw","region":"eu-west-1","gone":"x"}' AS BLOB) WHERE id = 'mfd-a'`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	checker := &resourceChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeWorkspaceResources: valueobject.PermissionLevelWrite,
	}}
	h := NewManifestDeploymentsV2Handler(db, middleware.NewIAMPermissionMiddlewareWithChecker(checker))
	r := gin.New()
	r.POST("/organizations/:org_id/manifests/:id/v2/deployments/:deployment_id/upgrade", withCaller(valueobject.PermissionLevelRead), h.Upgrade)

	// api_token omitted -> kept; db_password sent as the sensitive placeholder "" -> kept;
	// other_secret placeholder without a stored override -> not added (would mask the real value);
	// region explicitly set (non-sensitive) -> updated; gone in unset_keys -> removed.
	w := doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/upgrade",
		`{"target_version_id":"mfv-2","variable_overrides":{"db_password":"","other_secret":"","region":"us-east-1","new_key":"v"},"unset_keys":["gone"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("upgrade: %d %s", w.Code, w.Body.String())
	}
	var dep models.ManifestDeployment
	db.First(&dep, "id = ?", "mfd-a")
	got := map[string]string{}
	if err := json.Unmarshal(dep.VariableOverrides, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"api_token": "stored-token", "db_password": "stored-pw", "region": "us-east-1", "new_key": "v"}
	if len(got) != len(want) {
		t.Fatalf("overrides = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	if dep.VersionID != "mfv-2" {
		t.Fatalf("version not switched: %s", dep.VersionID)
	}

	// a second upgrade without variable_overrides keeps everything
	if w := doJSON(r, "POST", "/organizations/1/manifests/mf-1/v2/deployments/mfd-a/upgrade", `{"target_version_id":"mfv-1"}`); w.Code != http.StatusOK {
		t.Fatalf("upgrade 2: %d %s", w.Code, w.Body.String())
	}
	db.First(&dep, "id = ?", "mfd-a")
	got2 := map[string]string{}
	_ = json.Unmarshal(dep.VariableOverrides, &got2)
	if len(got2) != len(want) || got2["api_token"] != "stored-token" {
		t.Fatalf("omitting variable_overrides must not wipe them: %v", got2)
	}
}
