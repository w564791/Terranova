package migration

import (
	"context"
	"fmt"
	"log"

	"iac-platform/internal/manifestbundle"

	"gorm.io/gorm"
)

// manifestBundleV2Statements is the DDL contract of migration
// 20261004_04_manifest_bundle_v2 (docs/manifest/manifest-sandbox-spec.md
// §3.3). Additive and idempotent. Keep
// backend/migrations/add_manifest_bundle_v2.sql and the seed block in
// manifests/db/init_seed_data.sql in sync (TestManifestBundleV2SQLInSync).
func manifestBundleV2Statements() []string {
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

// applyManifestBundleV2 adds the column/index, then re-evaluates every
// version (recomputeManifestBundleHashes). Only DB errors abort.
func applyManifestBundleV2(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range manifestBundleV2Statements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("manifest bundle v2 schema: %w", err)
		}
	}
	return recomputeManifestBundleHashes(ctx, tx)
}

// recomputeManifestBundleHashes moves every version to the v2 hash under the
// bundle rules without ever laundering tampered content:
//   - bundle_invalid_reason = 'hash_mismatch': never re-evaluated (sticky; only
//     publishing a new version produces a valid bundle); a stray bundle_hash is
//     reset to NULL;
//   - stored bundle_hash set (v1 from step 2, or v2): the current files must
//     reproduce it (manifestbundle.CheckStoredVersion). If not, bundle_hash =
//     NULL + 'hash_mismatch' and a WARN security log line; the hash is never
//     re-derived from the current content;
//   - stored hash matched, or no stored hash (never hashed / earlier rule
//     failure): the bundle rules decide: valid => v2 hash, reason NULL;
//     violations => NULL + "rule @ path" reason + republish log line.
//
// Files are never modified, and rule violations never fail the migration.
func recomputeManifestBundleHashes(ctx context.Context, tx *gorm.DB) error {
	var versions []struct {
		ID                  string
		ManifestID          string
		BundleHash          *string
		BundleInvalidReason *string
	}
	if err := tx.WithContext(ctx).Table("manifest_versions").
		Select("id, manifest_id, bundle_hash, bundle_invalid_reason").Order("id").
		Find(&versions).Error; err != nil {
		return fmt.Errorf("list manifest versions: %w", err)
	}
	invalid, mismatched := 0, 0
	for _, v := range versions {
		if v.BundleInvalidReason != nil && *v.BundleInvalidReason == manifestbundle.ReasonHashMismatch {
			mismatched++
			// Never re-evaluated. A stray hash (e.g. a manual re-run of the step 2
			// SQL backfill, which has no reason guard) is cleared again.
			if v.BundleHash != nil {
				if err := tx.WithContext(ctx).Table("manifest_versions").Where("id = ?", v.ID).
					Update("bundle_hash", nil).Error; err != nil {
					return fmt.Errorf("clear bundle hash of mismatched version %s: %w", v.ID, err)
				}
			}
			continue
		}
		stored := ""
		if v.BundleHash != nil {
			stored = *v.BundleHash
		}
		hash, problems, mismatch, err := manifestbundle.CheckStoredVersion(ctx, tx, v.ID, stored)
		if err != nil {
			return fmt.Errorf("check bundle of version %s: %w", v.ID, err)
		}
		var updates map[string]interface{}
		switch {
		case mismatch:
			mismatched++
			updates = map[string]interface{}{"bundle_hash": nil, "bundle_invalid_reason": manifestbundle.ReasonHashMismatch}
			bundleRulesLogf("[WARN] [security] [migration] manifest version %s (manifest_id=%s): stored bundle_hash does not match its files, marked %s, republish required",
				v.ID, v.ManifestID, manifestbundle.ReasonHashMismatch)
		case len(problems) > 0:
			invalid++
			reason := manifestbundle.Reason(problems)
			updates = map[string]interface{}{"bundle_hash": nil, "bundle_invalid_reason": reason}
			bundleRulesLogf("[migration] manifest version %s: bundle invalid, republish required: %s", v.ID, reason)
		default:
			updates = map[string]interface{}{"bundle_hash": hash, "bundle_invalid_reason": nil}
		}
		if err := tx.WithContext(ctx).Table("manifest_versions").Where("id = ?", v.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("write bundle_hash of version %s: %w", v.ID, err)
		}
	}
	if invalid > 0 || mismatched > 0 {
		bundleRulesLogf("[migration] manifest bundle rules: %d of %d versions need republishing (%d rule violations, %d hash mismatches)",
			invalid+mismatched, len(versions), invalid, mismatched)
	}
	return nil
}
