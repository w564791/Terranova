-- Manifest bundle v2 (docs/manifest/manifest-sandbox-spec.md §3.3)
-- (kept in sync with versioned migration 20261004_04_manifest_bundle_v2).
-- Additive and idempotent: one nullable column and one partial index.
--
-- Moving bundle_hash to terranova-bundle-v2 (verify the stored v1 hash
-- against the current files first, sticky hash_mismatch otherwise) and the
-- bundle rules (NFC paths, case duplicates, denylisted files, size limits,
-- secret scan) need the Go implementation (manifestbundle) and run only in
-- the migration job; this patch writes no hashes. Run the migration job
-- after applying it.

ALTER TABLE public.manifest_versions ADD COLUMN IF NOT EXISTS bundle_invalid_reason text;

CREATE INDEX IF NOT EXISTS idx_manifest_versions_bundle_hash ON public.manifest_versions (bundle_hash) WHERE bundle_hash IS NOT NULL;
