package migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"iac-platform/internal/manifestbundle"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestManifestBundleRulesSchemaIsAdditive(t *testing.T) {
	for _, stmt := range manifestBundleRulesStatements() {
		upper := strings.ToUpper(stmt)
		for _, forbidden := range []string{"DROP ", "DELETE FROM", "TRUNCATE ", "UPDATE ", "RENAME ", "ALTER COLUMN", " TYPE "} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("statement must be additive, found %q in:\n%s", forbidden, stmt)
			}
		}
		if strings.HasPrefix(upper, "ALTER TABLE") && strings.Contains(upper, "NOT NULL") {
			t.Fatalf("new column must be nullable:\n%s", stmt)
		}
		if !strings.Contains(upper, "IF NOT EXISTS") || !strings.Contains(stmt, "public.") {
			t.Fatalf("statement must be idempotent and schema-qualified:\n%s", stmt)
		}
	}
}

func TestManifestBundleRulesSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_manifest_bundle_rules.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range manifestBundleRulesStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
	}
	// the step-2 SQL backfill must not re-hash versions the rules marked invalid
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_manifest_sandbox_schema.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, _ := os.ReadFile(path)
		if !strings.Contains(normalizeSQL(string(content)), "AND to_jsonb(v) ->> 'bundle_invalid_reason' IS NULL;") {
			t.Fatalf("%s: step-2 bundle_hash backfill lacks the bundle_invalid_reason guard", path)
		}
	}
}

const fakeAWSKey = "AKIAQWERTYUIOPASDFGH" // matches the aws_access_key format; not a real key

type fileRow struct {
	ID        int64
	VersionID string
	Path      string
	Content   []byte
	Mime      string
	Size      int
}

func setupBundleRulesDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, bundle_hash TEXT, bundle_invalid_reason TEXT, created_at DATETIME)`,
		`CREATE TABLE manifest_files (id INTEGER PRIMARY KEY AUTOINCREMENT, manifest_id TEXT, version_id TEXT, owner_user_id TEXT, path TEXT, content BLOB, mime TEXT, size INTEGER, is_binary BOOLEAN DEFAULT 0, mode INTEGER DEFAULT 420, created_at DATETIME, updated_at DATETIME)`,
		// mfv-good: valid, with a stale hash that must be recomputed
		`INSERT INTO manifest_versions (id, manifest_id, version, bundle_hash) VALUES
		   ('mfv-good', 'mf-1', 'v1.0.0', '0000000000000000000000000000000000000000000000000000000000000000'),
		   ('mfv-bad', 'mf-1', 'v1.1.0', 'd9bbb44c83b611cca6924418c94982c70829682413fc12f76147a40a43cd2ddb'),
		   ('mfv-secret', 'mf-1', 'v1.2.0', 'd9bbb44c83b611cca6924418c94982c70829682413fc12f76147a40a43cd2ddb'),
		   ('mfv-empty', 'mf-1', 'v0.0.1', NULL)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	ins := func(vid, path, content string) {
		if err := db.Exec(`INSERT INTO manifest_files (manifest_id, version_id, path, content, mime, size) VALUES ('mf-1', ?, ?, ?, 'text/plain', ?)`,
			vid, path, []byte(content), len(content)).Error; err != nil {
			t.Fatal(err)
		}
	}
	ins("mfv-good", "main.tf", `resource "null_resource" "a" {}`)
	ins("mfv-good", "modules/x/variables.tf", `variable "x" {}`)
	ins("mfv-bad", "main.tf", `resource "null_resource" "a" {}`)
	ins("mfv-bad", "prod.tfvars", `region = "eu-west-1"`)
	ins("mfv-bad", "cafe\u0301.tf", "") // NFD "café"
	ins("mfv-bad", "README.md", "a")
	ins("mfv-bad", "readme.md", "b")
	ins("mfv-secret", "main.tf", "provider \"aws\" {\n  access_key = \""+fakeAWSKey+"\"\n}\n")
	return db
}

func snapshotFiles(t *testing.T, db *gorm.DB) []fileRow {
	t.Helper()
	var rows []fileRow
	if err := db.Table("manifest_files").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func versionState(t *testing.T, db *gorm.DB, id string) (hash, reason *string) {
	t.Helper()
	var v struct {
		BundleHash          *string
		BundleInvalidReason *string
	}
	if err := db.Table("manifest_versions").Select("bundle_hash, bundle_invalid_reason").Where("id = ?", id).Take(&v).Error; err != nil {
		t.Fatal(err)
	}
	return v.BundleHash, v.BundleInvalidReason
}

func TestRecomputeBundleHashes_BadOldVersionsDoNotFailAndFilesUntouched(t *testing.T) {
	db := setupBundleRulesDB(t)
	var logs []string
	bundleRulesLogf = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { bundleRulesLogf = defaultBundleRulesLogf })

	before := snapshotFiles(t, db)
	if err := recomputeManifestBundleHashes(context.Background(), db); err != nil {
		t.Fatalf("migration must not fail on bad old versions: %v", err)
	}
	after := snapshotFiles(t, db)
	if fmt.Sprintf("%v", before) != fmt.Sprintf("%v", after) {
		t.Fatal("migration modified manifest_files")
	}

	// valid version: stale hash replaced by manifestbundle.Hash of its files
	want, _ := manifestbundle.Hash([]manifestbundle.File{
		{Path: "main.tf", Content: []byte(`resource "null_resource" "a" {}`)},
		{Path: "modules/x/variables.tf", Content: []byte(`variable "x" {}`)},
	})
	if h, r := versionState(t, db, "mfv-good"); h == nil || *h != want || r != nil {
		t.Fatalf("mfv-good: hash=%v reason=%v want %s", h, r, want)
	}
	if h, r := versionState(t, db, "mfv-empty"); h == nil || r != nil {
		t.Fatalf("empty bundle is valid: hash=%v reason=%v", h, r)
	}

	h, r := versionState(t, db, "mfv-bad")
	if h != nil || r == nil {
		t.Fatalf("mfv-bad: want NULL hash and a reason, got hash=%v reason=%v", h, r)
	}
	for _, wantPart := range []string{
		"denylisted_file @ prod.tfvars",
		"path_not_nfc @ cafe\u0301.tf",
		"path_case_duplicate @ README.md",
		"path_case_duplicate @ readme.md",
	} {
		if !strings.Contains(*r, wantPart) {
			t.Fatalf("mfv-bad reason %q lacks %q", *r, wantPart)
		}
	}
	if strings.Contains(*r, "eu-west-1") {
		t.Fatalf("reason leaks file content: %q", *r)
	}

	// idempotent
	if err := recomputeManifestBundleHashes(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if h2, _ := versionState(t, db, "mfv-good"); *h2 != want {
		t.Fatal("second run changed a valid hash")
	}
	_ = logs
}

func TestRecomputeBundleHashes_SecretHitReasonAndLogCarryNoSecret(t *testing.T) {
	db := setupBundleRulesDB(t)
	var logs []string
	bundleRulesLogf = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { bundleRulesLogf = defaultBundleRulesLogf })

	if err := recomputeManifestBundleHashes(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	h, r := versionState(t, db, "mfv-secret")
	if h != nil || r == nil || *r != "secret_scan:aws_access_key @ main.tf" {
		t.Fatalf("mfv-secret: hash=%v reason=%v", h, r)
	}
	var secretLine string
	for _, l := range logs {
		if strings.Contains(l, "mfv-secret") {
			secretLine = l
		}
		for _, leak := range []string{fakeAWSKey, "AKIA", "access_key =", `provider "aws"`, "{"} {
			if strings.Contains(l, leak) {
				t.Fatalf("log line leaks %q: %s", leak, l)
			}
		}
	}
	if secretLine != "[migration] manifest version mfv-secret: bundle invalid, republish required: secret_scan:aws_access_key @ main.tf" {
		t.Fatalf("log line = %q", secretLine)
	}
}
