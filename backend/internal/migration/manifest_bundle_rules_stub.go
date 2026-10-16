package migration

import (
	"context"

	"gorm.io/gorm"
)

// applyManifestBundleRules is retained so schema_migrations checksums for
// 20261004_03 stay valid. The implementation was replaced by
// 20261004_04_manifest_bundle_v2; this no-op is only reached on fresh DBs
// that still walk the historical version list before v2.
func applyManifestBundleRules(context.Context, *gorm.DB) error { return nil }
