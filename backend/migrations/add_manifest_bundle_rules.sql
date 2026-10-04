-- Manifest bundle rules (docs/manifest/manifest-sandbox-spec.md §3.3)
-- (kept in sync with versioned migration 20261004_03_manifest_bundle_rules).
-- Additive and idempotent: one nullable column and one partial index.
--
-- bundle_hash recompute under the bundle rules (NFC paths, case duplicates,
-- denylisted files, size limits, secret scan) needs the Go implementation
-- (manifestbundle.Validate) and runs only in the migration job; this patch
-- writes no hashes. Run the migration job after applying it.

ALTER TABLE public.manifest_versions ADD COLUMN IF NOT EXISTS bundle_invalid_reason text;

CREATE INDEX IF NOT EXISTS idx_manifest_versions_bundle_hash ON public.manifest_versions (bundle_hash) WHERE bundle_hash IS NOT NULL;
