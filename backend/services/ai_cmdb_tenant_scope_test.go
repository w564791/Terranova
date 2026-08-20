package services

import (
	"errors"
	"testing"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAICMDBTenantScopeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER NOT NULL)`,
		`CREATE TABLE workspaces (workspace_id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT NOT NULL, project_id INTEGER NOT NULL)`,
		`INSERT INTO projects (id, org_id) VALUES (1, 1), (2, 2)`,
		`INSERT INTO workspaces (workspace_id, name) VALUES
			('ws-org-1', 'Organization 1'), ('ws-org-1-peer', 'Organization 1 peer'),
			('ws-org-2', 'Organization 2'), ('ws-corrupt', 'Corrupt')`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES
			('ws-org-1', 1), ('ws-org-1-peer', 1), ('ws-org-2', 2),
			('ws-corrupt', 1), ('ws-corrupt', 2)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed tenant fixture: %v\n%s", err, statement)
		}
	}
	if err := db.AutoMigrate(&models.ResourceIndex{}); err != nil {
		t.Fatalf("migrate resource index: %v", err)
	}
	for _, resource := range []models.ResourceIndex{
		{
			WorkspaceID: "ws-org-1", TerraformAddress: "aws_vpc.local", ResourceType: "aws_vpc", ResourceName: "local",
			ResourceMode: "managed", CloudResourceID: "vpc-shared", CloudResourceName: "local-vpc", CloudResourceARN: "arn:org-1:local",
		},
		{
			WorkspaceID: "__external__", TerraformAddress: "aws_vpc.external", ResourceType: "aws_vpc", ResourceName: "external",
			ResourceMode: "managed", CloudResourceID: "vpc-shared", CloudResourceName: "external-vpc", CloudResourceARN: "arn:external:shared",
		},
		{
			WorkspaceID: "ws-org-2", TerraformAddress: "aws_vpc.foreign", ResourceType: "aws_vpc", ResourceName: "foreign",
			ResourceMode: "managed", CloudResourceID: "vpc-foreign", CloudResourceName: "foreign-vpc", CloudResourceARN: "arn:org-2:foreign",
		},
		{
			WorkspaceID: "ws-org-1", TerraformAddress: "aws_vpc.duplicate_a", ResourceType: "aws_vpc", ResourceName: "duplicate-a",
			ResourceMode: "managed", CloudResourceID: "vpc-duplicated", CloudResourceName: "duplicate-a", CloudResourceARN: "arn:org-1:duplicate-a",
		},
		{
			WorkspaceID: "ws-org-1-peer", TerraformAddress: "aws_vpc.duplicate_b", ResourceType: "aws_vpc", ResourceName: "duplicate-b",
			ResourceMode: "managed", CloudResourceID: "vpc-duplicated", CloudResourceName: "duplicate-b", CloudResourceARN: "arn:org-1:duplicate-b",
		},
	} {
		if err := db.Create(&resource).Error; err != nil {
			t.Fatalf("create resource: %v", err)
		}
	}
	return db
}

func TestAICMDBScopeExcludesExternalAndForeignResources(t *testing.T) {
	db := setupAICMDBTenantScopeDB(t)
	scope, err := ResolveCMDBWorkspaceScope(db, 1, "ws-org-1")
	if err != nil {
		t.Fatalf("resolve scope: %v", err)
	}

	service := NewAICMDBService(db)
	result := service.executeQuery(CMDBQuery{Type: "aws_vpc", Keyword: "vpc-shared"}, nil, scope.workspaceIDs)
	if !result.Found || result.Resource == nil {
		t.Fatalf("expected organization resource, got %+v", result)
	}
	if result.Resource.ARN != "arn:org-1:local" || result.Resource.WorkspaceID != "ws-org-1" {
		t.Fatalf("query used external/global CMDB row: %+v", result.Resource)
	}

	skillService := NewAICMDBSkillService(db)
	if _, err := skillService.buildCMDBDataFromSelections(map[string]string{"vpc_id": "vpc-foreign"}, scope); !errors.Is(err, ErrCMDBResourceNotInScope) {
		t.Fatalf("foreign selection must be rejected without disclosure, got %v", err)
	}
	if _, err := skillService.buildCMDBDataFromSelections(map[string]string{"vpc_id": "vpc-duplicated"}, scope); !errors.Is(err, ErrCMDBResourceNotInScope) {
		t.Fatalf("ambiguous same-org resource selection must fail closed, got %v", err)
	}
}

func TestResolveCMDBWorkspaceScopeKeepsEmptyOrganizationScopeFailClosed(t *testing.T) {
	db := setupAICMDBTenantScopeDB(t)
	if err := db.Exec(`INSERT INTO projects (id, org_id) VALUES (3, 3)`).Error; err != nil {
		t.Fatal(err)
	}

	scope, err := ResolveCMDBWorkspaceScope(db, 3, "")
	if err != nil {
		t.Fatalf("empty organization scope must still resolve: %v", err)
	}
	if scope.workspaceIDs == nil || len(scope.workspaceIDs) != 0 {
		t.Fatalf("empty scope must be explicit rather than global: %#v", scope.workspaceIDs)
	}

	result := NewAICMDBService(db).executeQuery(CMDBQuery{Type: "aws_vpc", Keyword: "vpc-shared"}, nil, scope.workspaceIDs)
	if result.Found {
		t.Fatalf("empty tenant scope must not query global/external records: %+v", result)
	}
}
