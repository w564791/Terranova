package handlers

import (
	"context"
	"testing"

	"iac-platform/internal/manifestbundle"

	"gorm.io/gorm"
)

// sealFixtureVersions brings a test fixture to the post-step-3 state: the
// bundle columns exist on manifest_versions (created with one row per
// deployment version when the fixture had no versions table) and every
// version without a hash or reason gets the hash of its stored files, as the
// recompute migration would write.
func sealFixtureVersions(t *testing.T, db *gorm.DB) {
	t.Helper()
	if !db.Migrator().HasTable("manifest_versions") {
		if err := db.Exec(`CREATE TABLE manifest_versions (id TEXT PRIMARY KEY, manifest_id TEXT, version TEXT, variables TEXT, changelog TEXT, created_by TEXT, created_at DATETIME)`).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO manifest_versions (id, manifest_id, version, created_by)
			SELECT DISTINCT version_id, manifest_id, 'v0.0.1', 'u1' FROM manifest_deployments`).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, col := range []string{"bundle_hash", "bundle_invalid_reason"} {
		if !db.Migrator().HasColumn("manifest_versions", col) {
			if err := db.Exec(`ALTER TABLE manifest_versions ADD COLUMN ` + col + ` TEXT`).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	var ids []string
	db.Table("manifest_versions").Where("bundle_hash IS NULL AND bundle_invalid_reason IS NULL").Pluck("id", &ids)
	for _, id := range ids {
		hash, problems, err := manifestbundle.CheckVersion(context.Background(), db, id)
		if err != nil || len(problems) > 0 {
			t.Fatalf("fixture version %s: err=%v problems=%v", id, err, problems)
		}
		db.Table("manifest_versions").Where("id = ?", id).Update("bundle_hash", hash)
	}
}
