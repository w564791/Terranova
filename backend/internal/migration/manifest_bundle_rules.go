package migration

import (
	"context"
	"fmt"
	"log"

	"iac-platform/internal/manifestbundle"

	"gorm.io/gorm"
)

// manifestBundleRulesStatements is the DDL contract of migration
// 20261004_03_manifest_bundle_rules (docs/manifest/manifest-sandbox-spec.md
// §3.3). Additive and idempotent. Keep
// backend/migrations/add_manifest_bundle_rules.sql and the seed block in
// manifests/db/init_seed_data.sql in sync (TestManifestBundleRulesSQLInSync).
func manifestBundleRulesStatements() []string {
	return []string{
		// why a version has no bundle_hash: "rule @ path; ..." (never file content)
		`ALTER TABLE public.manifest_versions ADD COLUMN IF NOT EXISTS bundle_invalid_reason text`,
		// content-addressed lookup (manifestbundle.OpenBundle)
		`CREATE INDEX IF NOT EXISTS idx_manifest_versions_bundle_hash ON public.manifest_versions (bundle_hash) WHERE bundle_hash IS NOT NULL`,
	}
}

// bundleRulesLogf is swapped by tests to capture the migration log.
var (
	defaultBundleRulesLogf = log.Printf
	bundleRulesLogf        = defaultBundleRulesLogf
)

// applyManifestBundleRules adds the column/index, then recomputes bundle_hash
// of every version under the bundle rules (manifestbundle.Validate + Hash, the
// same code publish uses). A version that fails the rules never fails the
// migration: its files stay untouched, bundle_hash becomes NULL and the reason
// (rule names and paths only) is written. Only DB errors abort.
func applyManifestBundleRules(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range manifestBundleRulesStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("manifest bundle rules schema: %w", err)
		}
	}
	return recomputeManifestBundleHashes(ctx, tx)
}

func recomputeManifestBundleHashes(ctx context.Context, tx *gorm.DB) error {
	var versionIDs []string
	if err := tx.WithContext(ctx).Table("manifest_versions").Order("id").
		Pluck("id", &versionIDs).Error; err != nil {
		return fmt.Errorf("list manifest versions: %w", err)
	}
	invalid := 0
	for _, id := range versionIDs {
		hash, problems, err := manifestbundle.CheckVersion(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("check bundle of version %s: %w", id, err)
		}
		updates := map[string]interface{}{"bundle_hash": hash, "bundle_invalid_reason": nil}
		if len(problems) > 0 {
			reason := manifestbundle.Reason(problems)
			updates = map[string]interface{}{"bundle_hash": nil, "bundle_invalid_reason": reason}
			invalid++
			bundleRulesLogf("[migration] manifest version %s: bundle invalid, republish required: %s", id, reason)
		}
		if err := tx.WithContext(ctx).Table("manifest_versions").Where("id = ?", id).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("write bundle_hash of version %s: %w", id, err)
		}
	}
	if invalid > 0 {
		bundleRulesLogf("[migration] manifest bundle rules: %d of %d versions need republishing", invalid, len(versionIDs))
	}
	return nil
}
