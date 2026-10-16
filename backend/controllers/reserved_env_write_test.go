package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupReservedEnvWriteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, s := range []string{
		`CREATE TABLE workspaces (
			id INTEGER PRIMARY KEY, workspace_id TEXT, name TEXT,
			state_backend TEXT, created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE workspace_variables (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT, variable_id TEXT, key TEXT, value TEXT,
			variable_type TEXT, value_format TEXT, sensitive INTEGER DEFAULT 0,
			description TEXT, is_deleted INTEGER DEFAULT 0, version INTEGER DEFAULT 1,
			key_version INTEGER DEFAULT 0, created_at DATETIME, updated_at DATETIME, created_by TEXT
		)`,
		`CREATE TABLE variable_sets (
			id INTEGER PRIMARY KEY AUTOINCREMENT, varset_id TEXT UNIQUE, name TEXT,
			description TEXT, scope TEXT, org_id INTEGER, is_deleted INTEGER DEFAULT 0,
			created_at DATETIME, updated_at DATETIME, created_by TEXT
		)`,
		`CREATE TABLE varset_variables (
			id INTEGER PRIMARY KEY AUTOINCREMENT, variable_id TEXT, varset_id TEXT,
			key TEXT, value TEXT, variable_type TEXT, value_format TEXT,
			sensitive INTEGER DEFAULT 0, description TEXT, is_deleted INTEGER DEFAULT 0,
			version INTEGER DEFAULT 1, key_version INTEGER DEFAULT 0,
			created_at DATETIME, updated_at DATETIME, created_by TEXT
		)`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("%v\n%s", err, s)
		}
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id, name, state_backend, created_at, updated_at)
		VALUES (1, 'ws-1', 'w', 'local', ?, ?)`, now, now).Error; err != nil {
		t.Fatal(err)
	}
	org := uint(1)
	vs := models.VariableSet{VarsetID: "vs-1", Name: "v", Scope: "specific", OrgID: &org}
	if err := db.Create(&vs).Error; err != nil {
		// BeforeCreate generates id - raw insert instead
		_ = db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, is_deleted) VALUES ('vs-1', 'v', 'specific', 1, 0)`)
	}
	return db
}

func assertReservedEnv422(t *testing.T, w *httptest.ResponseRecorder, wantKey, wantPrefix string) {
	t.Helper()
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code    string `json:"code"`
		Details struct {
			Key            string `json:"key"`
			ReservedPrefix string `json:"reserved_prefix"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "reserved_env_var" {
		t.Fatalf("code = %q", body.Code)
	}
	if body.Details.Key != wantKey {
		t.Fatalf("details.key = %q, want %q", body.Details.Key, wantKey)
	}
	if body.Details.ReservedPrefix != wantPrefix {
		t.Fatalf("details.reserved_prefix = %q, want %q", body.Details.ReservedPrefix, wantPrefix)
	}
}

func TestWorkspaceCreateVariable_ReservedEnv422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupReservedEnvWriteDB(t)
	ctrl := NewWorkspaceVariableController(services.NewWorkspaceVariableService(db))
	r := gin.New()
	r.POST("/workspaces/:id/variables", func(c *gin.Context) {
		c.Set("user_id", "u1")
		ctrl.CreateVariable(c)
	})
	w := httptest.NewRecorder()
	body := `{"key":"TF_CLI_ARGS_init","value":"-upgrade","variable_type":"environment"}`
	req := httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/variables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assertReservedEnv422(t, w, "TF_CLI_ARGS_init", "TF_CLI_ARGS")
}

func TestWorkspaceUpdateVariable_ReservedEnv422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupReservedEnvWriteDB(t)
	db.Exec(`INSERT INTO workspace_variables (workspace_id, variable_id, key, value, variable_type, value_format, version, is_deleted)
		VALUES ('ws-1', 'var-1', 'AWS_REGION', 'us-east-1', 'environment', 'string', 1, 0)`)
	ctrl := NewWorkspaceVariableController(services.NewWorkspaceVariableService(db))
	r := gin.New()
	r.PUT("/workspaces/:id/variables/:var_id", func(c *gin.Context) {
		c.Set("user_id", "u1")
		ctrl.UpdateVariable(c)
	})
	w := httptest.NewRecorder()
	body := `{"version":1,"key":"HTTP_PROXY","value":"http://evil","variable_type":"environment"}`
	req := httptest.NewRequest(http.MethodPut, "/workspaces/ws-1/variables/var-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assertReservedEnv422(t, w, "HTTP_PROXY", "HTTP_PROXY")
}

func TestVarsetCreateVariable_ReservedEnv422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupReservedEnvWriteDB(t)
	_ = db.Exec(`INSERT INTO variable_sets (varset_id, name, scope, org_id, is_deleted) VALUES ('vs-1', 'v', 'specific', 1, 0)`)
	ctrl := NewVarsetVariableController(db)
	r := gin.New()
	r.POST("/variable-sets/:varset_id/variables", func(c *gin.Context) {
		c.Set("user_id", "u1")
		ctrl.Create(c)
	})
	w := httptest.NewRecorder()
	body := `{"key":"TF_PLUGIN_CACHE_DIR","value":"/tmp","variable_type":"environment"}`
	req := httptest.NewRequest(http.MethodPost, "/variable-sets/vs-1/variables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assertReservedEnv422(t, w, "TF_PLUGIN_CACHE_DIR", "TF_PLUGIN_CACHE")
}

func TestBulkCreateVariables_ReservedEnv(t *testing.T) {
	db := setupReservedEnvWriteDB(t)
	svc := services.NewWorkspaceVariableService(db)
	err := svc.BulkCreateVariables([]*models.WorkspaceVariable{{
		WorkspaceID:  "ws-1",
		Key:          "TF_CLI_ARGS_plan",
		Value:        "-lock=false",
		VariableType: models.VariableTypeEnvironment,
	}})
	if err == nil {
		t.Fatal("expected reserved_env_var error")
	}
	if !strings.Contains(err.Error(), "reserved_env_var") || !strings.Contains(err.Error(), "TF_CLI_ARGS") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkspaceCreateVariable_NonEnvReservedKeyAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupReservedEnvWriteDB(t)
	ctrl := NewWorkspaceVariableController(services.NewWorkspaceVariableService(db))
	r := gin.New()
	r.POST("/workspaces/:id/variables", func(c *gin.Context) {
		c.Set("user_id", "u1")
		ctrl.CreateVariable(c)
	})
	w := httptest.NewRecorder()
	// terraform category may use any key name; reserved check only for environment
	body := `{"key":"TF_CLI_ARGS_init","value":"x","variable_type":"terraform"}`
	req := httptest.NewRequest(http.MethodPost, "/workspaces/ws-1/variables", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code == http.StatusUnprocessableEntity {
		t.Fatalf("terraform-category must not hit reserved_env_var: %s", w.Body.String())
	}
}
