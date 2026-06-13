package services

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestResolveModuleVersionSkillsForCheckMatchesModuleVersionSource(t *testing.T) {
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
		`INSERT INTO module_versions (id, module_id, version, source, module_source, is_default, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"modv-s3", 54, "1.0.0", "platform/aws-s3", "platform/aws-s3", true, "active", now.Add(-time.Hour), now.Add(-time.Hour),
	).Error; err != nil {
		t.Fatalf("create module version: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO module_version_skills (id, module_version_id, schema_generated_content, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"mvs-s3", "modv-s3", "s3 module constraints", now, now,
	).Error; err != nil {
		t.Fatalf("create module version skill: %v", err)
	}

	svc := NewManifestCheckService(db)
	content, names := svc.resolveModuleVersionSkillsForCheck([]CheckFileInput{{
		Path: "s3/main.tf",
		Content: `
module "bucket" {
  source = "platform/aws-s3"
}
`,
		StartLine: 1,
	}})

	if len(names) != 1 || names[0] != "module_54_version_skill" {
		t.Fatalf("resolveModuleVersionSkillsForCheck() names = %#v, want [module_54_version_skill]", names)
	}
	if !strings.Contains(content, "s3 module constraints") {
		t.Fatalf("resolveModuleVersionSkillsForCheck() content missing skill text")
	}
}

func TestParseCheckIssuesDropsFixOnBlockHeaderWithAttributeReplacement(t *testing.T) {
	file := CheckFileInput{
		Path: "main.tf",
		Content: `module "app" {
  source = "platform/app"
}`,
		StartLine: 1,
	}
	aiResult := `{
  "issues": [
    {
      "file": "main.tf",
      "line": 2,
      "level": "warning",
      "message": "module 缺少 policy 配置",
      "fix": {
        "file": "main.tf",
        "start_line": 1,
        "end_line": 1,
        "new_text": "  policy = \"strict\""
      }
    }
  ]
}`

	issues, ok := parseCheckIssues(
		aiResult,
		"main.tf",
		false,
		map[string]bool{"main.tf": true},
		map[string]CheckFileInput{"main.tf": file},
	)
	if !ok {
		t.Fatalf("parseCheckIssues() ok = false")
	}
	if len(issues) != 1 {
		t.Fatalf("parseCheckIssues() issues length = %d, want 1", len(issues))
	}
	if issues[0].Fix != nil {
		t.Fatalf("unsafe fix should be dropped, got %#v", issues[0].Fix)
	}
}

func TestParseCheckIssuesKeepsWholeBlockReplacementFromHeader(t *testing.T) {
	file := CheckFileInput{
		Path: "main.tf",
		Content: `module "app" {
  source = "platform/app"
}`,
		StartLine: 1,
	}
	aiResult := `{
  "issues": [
    {
      "file": "main.tf",
      "line": 1,
      "level": "warning",
      "message": "module 缺少 policy 配置",
      "fix": {
        "file": "main.tf",
        "start_line": 1,
        "end_line": 3,
        "new_text": "module \"app\" {\n  source = \"platform/app\"\n  policy = \"strict\"\n}"
      }
    }
  ]
}`

	issues, ok := parseCheckIssues(
		aiResult,
		"main.tf",
		false,
		map[string]bool{"main.tf": true},
		map[string]CheckFileInput{"main.tf": file},
	)
	if !ok {
		t.Fatalf("parseCheckIssues() ok = false")
	}
	if len(issues) != 1 {
		t.Fatalf("parseCheckIssues() issues length = %d, want 1", len(issues))
	}
	if issues[0].Fix == nil {
		t.Fatalf("safe whole-block fix should be kept")
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
			is_default INTEGER DEFAULT 0,
			status TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE module_version_skills (
			id TEXT PRIMARY KEY,
			module_version_id TEXT,
			schema_generated_content TEXT,
			custom_content TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE skills (
			id TEXT PRIMARY KEY,
			name TEXT,
			display_name TEXT,
			description TEXT,
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
