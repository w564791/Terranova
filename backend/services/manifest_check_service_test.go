package services

import (
	"testing"
	"time"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestResolveReferencedModuleSkillNamesMatchesModuleVersionSource(t *testing.T) {
	db := newManifestCheckTestDB(t)
	now := time.Now()

	if err := db.Exec(
		`INSERT INTO modules (id, name, provider, source, module_source, version, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		54, "s3", "aws", "catalog/aws-s3", "catalog/aws-s3", "1.0.0", "active", now.Add(-time.Hour), now.Add(-time.Hour),
	).Error; err != nil {
		t.Fatalf("create module: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO module_versions (id, module_id, version, source, module_source, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"modv-s3", 54, "1.0.0", "platform/aws-s3", "platform/aws-s3", "active", now.Add(-time.Hour), now.Add(-time.Hour),
	).Error; err != nil {
		t.Fatalf("create module version: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO skills (id, name, display_name, layer, content, version, is_active, priority, source_type, source_module_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"skill-module-54", "module_54_auto", "S3 Module", models.SkillLayerDomain, "s3 module constraints", "1.0.0", true, 100, models.SkillSourceModuleAuto, 54, now, now,
	).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}

	svc := NewManifestCheckService(db)
	got := svc.resolveReferencedModuleSkillNames([]CheckFileInput{{
		Path: "s3/main.tf",
		Content: `
module "bucket" {
  source = "platform/aws-s3"
}
`,
		StartLine: 1,
	}})

	if len(got) != 1 || got[0] != "module_54_auto" {
		t.Fatalf("resolveReferencedModuleSkillNames() = %#v, want [module_54_auto]", got)
	}
}

func newManifestCheckTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	statements := []string{
		`CREATE TABLE modules (
			id INTEGER PRIMARY KEY,
			name TEXT,
			provider TEXT,
			source TEXT,
			module_source TEXT,
			version TEXT,
			status TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE module_versions (
			id TEXT PRIMARY KEY,
			module_id INTEGER,
			version TEXT,
			source TEXT,
			module_source TEXT,
			status TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE skills (
			id TEXT PRIMARY KEY,
			name TEXT,
			display_name TEXT,
			layer TEXT,
			content TEXT,
			version TEXT,
			is_active INTEGER,
			priority INTEGER,
			source_type TEXT,
			source_module_id INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}
