package controllers

import (
	"context"
	"encoding/json"
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

func setupRemoteDataTenantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Workspace{}, &models.WorkspaceOutputsAccess{}, &models.WorkspaceRemoteData{}); err != nil {
		t.Fatalf("migrate remote-data tables: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER NOT NULL)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT NOT NULL, project_id INTEGER NOT NULL)`,
		`INSERT INTO projects (id, org_id) VALUES (1, 1), (2, 2)`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES
			('ws-a', 1), ('ws-a-peer', 1), ('ws-b', 2)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("schema/fixture: %v\n%s", err, statement)
		}
	}
	now := time.Now()
	for _, ws := range []models.Workspace{
		{WorkspaceID: "ws-a", Name: "a", StateBackend: "local", OutputsSharing: "none", CreatedAt: now, UpdatedAt: now},
		{WorkspaceID: "ws-a-peer", Name: "a-peer", StateBackend: "local", OutputsSharing: "all", CreatedAt: now, UpdatedAt: now},
		{WorkspaceID: "ws-b", Name: "b", StateBackend: "local", OutputsSharing: "all", CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.Create(&ws).Error; err != nil {
			t.Fatalf("create workspace: %v", err)
		}
	}
	return db
}

func TestRemoteDataSharingNeverCrossesTenantBoundary(t *testing.T) {
	db := setupRemoteDataTenantDB(t)
	controller := NewWorkspaceRemoteDataController(db)

	if controller.canAccessOutputs(context.Background(), "ws-a", "ws-b") {
		t.Fatal("outputs_sharing=all must not allow a cross-tenant read")
	}
	if !controller.canAccessOutputs(context.Background(), "ws-a", "ws-a-peer") {
		t.Fatal("same-tenant workspace with outputs_sharing=all should remain available")
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/workspaces/:id/remote-data/accessible-workspaces", controller.GetAccessibleWorkspaces)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/workspaces/ws-a/remote-data/accessible-workspaces", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("accessible workspaces: want 200 got %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Workspaces []struct {
			WorkspaceID string `json:"workspace_id"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Workspaces) != 1 || body.Workspaces[0].WorkspaceID != "ws-a-peer" {
		t.Fatalf("cross-tenant workspace leaked through list: %+v", body.Workspaces)
	}
}

func TestUpdateOutputsSharingRejectsCrossTenantAllowList(t *testing.T) {
	db := setupRemoteDataTenantDB(t)
	controller := NewWorkspaceRemoteDataController(db)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/workspaces/:id/outputs-sharing", func(c *gin.Context) {
		c.Set("user_id", "user-a")
		c.Next()
	}, controller.UpdateOutputsSharing)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/workspaces/ws-a/outputs-sharing", strings.NewReader(`{"sharing_mode":"specific","allowed_workspace_ids":["ws-b"]}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant allow-list: want 404 got %d: %s", response.Code, response.Body.String())
	}

	var workspace models.Workspace
	if err := db.Where("workspace_id = ?", "ws-a").First(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	if workspace.OutputsSharing != "none" {
		t.Fatalf("rejected request must not change sharing mode, got %q", workspace.OutputsSharing)
	}
}

func TestListRemoteDataHidesLegacyCrossTenantReference(t *testing.T) {
	db := setupRemoteDataTenantDB(t)
	controller := NewWorkspaceRemoteDataController(db)
	now := time.Now()
	for _, remoteData := range []models.WorkspaceRemoteData{
		{WorkspaceID: "ws-a", RemoteDataID: "rd-local", SourceWorkspaceID: "ws-a-peer", DataName: "local", CreatedAt: now, UpdatedAt: now},
		{WorkspaceID: "ws-a", RemoteDataID: "rd-foreign", SourceWorkspaceID: "ws-b", DataName: "foreign", CreatedAt: now, UpdatedAt: now},
	} {
		if err := db.Create(&remoteData).Error; err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/workspaces/:id/remote-data", controller.ListRemoteData)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/workspaces/ws-a/remote-data", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list: want 200 got %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		RemoteData []struct {
			RemoteDataID string `json:"remote_data_id"`
		} `json:"remote_data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.RemoteData) != 1 || body.RemoteData[0].RemoteDataID != "rd-local" {
		t.Fatalf("legacy cross-tenant reference leaked: %+v", body.RemoteData)
	}
}
