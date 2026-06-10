package services

import (
	"strings"
	"testing"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupResourceDeleteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE workspace_resources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_name TEXT NOT NULL,
			current_version_id INTEGER,
			is_active BOOLEAN,
			description TEXT,
			tags TEXT,
			created_by TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			last_applied_at DATETIME,
			manifest_deployment_id TEXT
		);
		CREATE TABLE workspace_outputs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			output_id TEXT,
			resource_name TEXT,
			output_name TEXT NOT NULL,
			output_value TEXT NOT NULL,
			description TEXT,
			sensitive BOOLEAN,
			created_by TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);
	`).Error; err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestDeleteResource_AllowsWorkspaceResource(t *testing.T) {
	db := setupResourceDeleteTestDB(t)
	if err := db.Exec(`
		INSERT INTO workspace_resources (id, workspace_id, resource_id, resource_type, resource_name, is_active)
		VALUES (1, 'ws-test', 'aws_s3_bucket.main', 'aws_s3_bucket', 'main', true);
		INSERT INTO workspace_outputs (workspace_id, output_id, resource_name, output_name, output_value)
		VALUES ('ws-test', 'out-1', 'main', 'bucket_id', 'aws_s3_bucket.main.id');
	`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewResourceService(db, nil)
	if err := svc.DeleteResource(1, "user-1"); err != nil {
		t.Fatalf("delete workspace resource: %v", err)
	}

	var resource models.WorkspaceResource
	if err := db.First(&resource, 1).Error; err != nil {
		t.Fatalf("load resource: %v", err)
	}
	if resource.IsActive {
		t.Fatal("workspace resource should be soft-deleted")
	}
	var outputCount int64
	db.Model(&models.WorkspaceOutput{}).Where("workspace_id = ? AND resource_name = ?", "ws-test", "main").Count(&outputCount)
	if outputCount != 0 {
		t.Fatalf("workspace resource outputs should be deleted, got %d", outputCount)
	}
}

func TestDeleteResource_BlocksManifestManagedResource(t *testing.T) {
	db := setupResourceDeleteTestDB(t)
	if err := db.Exec(`
		INSERT INTO workspace_resources (id, workspace_id, resource_id, resource_type, resource_name, is_active, manifest_deployment_id)
		VALUES (1, 'ws-test', 'module.network', 'module', 'network', true, 'mfd-test');
		INSERT INTO workspace_outputs (workspace_id, output_id, resource_name, output_name, output_value)
		VALUES ('ws-test', 'out-1', 'network', 'vpc_id', 'module.network.vpc_id');
	`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewResourceService(db, nil)
	err := svc.DeleteResource(1, "user-1")
	if err == nil || !strings.Contains(err.Error(), "managed by a manifest deployment") {
		t.Fatalf("expected manifest-managed delete to be blocked, got %v", err)
	}

	var resource models.WorkspaceResource
	if err := db.First(&resource, 1).Error; err != nil {
		t.Fatalf("load resource: %v", err)
	}
	if !resource.IsActive {
		t.Fatal("manifest-managed resource should remain active")
	}
	var outputCount int64
	db.Model(&models.WorkspaceOutput{}).Where("workspace_id = ? AND resource_name = ?", "ws-test", "network").Count(&outputCount)
	if outputCount != 1 {
		t.Fatalf("manifest-managed resource outputs should be untouched, got %d", outputCount)
	}
}
