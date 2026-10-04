package migration

import (
	"context"
	"fmt"

	"iac-platform/internal/manifestbundle"

	"gorm.io/gorm"
)

// manifestSandboxSchemaStatements is the DDL contract of migration
// 20261004_02_manifest_sandbox_schema (docs/manifest/manifest-sandbox-spec.md §3).
// Additive only: new nullable/defaulted columns, new tables, new constraints
// guarded by existence checks. Every statement is idempotent so the same block
// can run from the seed, the psql patch and this job in any order.
//
// Keep backend/migrations/add_manifest_sandbox_schema.sql and the seed block
// in manifests/db/init_seed_data.sql in sync (TestManifestSandboxSQLInSync).
func manifestSandboxSchemaStatements() []string {
	return []string{
		// --- manifests: source (immutable after creation, enforced in UpdateManifest)
		`ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS source_type character varying(16) NOT NULL DEFAULT 'native'`,
		`ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_repo_url character varying(1024)`,
		`ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_subpath character varying(512)`,
		`ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS github_installation_id bigint`,
		addConstraintIfMissing("manifests", "chk_manifests_source_type",
			`CHECK (source_type IN ('native', 'git'))`),
		addConstraintIfMissing("manifests", "chk_manifests_git_fields",
			`CHECK ((source_type = 'native' AND git_repo_url IS NULL AND git_subpath IS NULL AND github_installation_id IS NULL) OR (source_type = 'git' AND git_repo_url IS NOT NULL))`),

		// --- manifest_versions: immutable bundle identity
		`ALTER TABLE public.manifest_versions ADD COLUMN IF NOT EXISTS bundle_hash character varying(64)`,
		`ALTER TABLE public.manifest_versions ADD COLUMN IF NOT EXISTS source_ref character varying(64)`,
		addConstraintIfMissing("manifest_versions", "chk_manifest_versions_bundle_hash",
			`CHECK (bundle_hash IS NULL OR bundle_hash ~ '^[0-9a-f]{64}$')`),
		addConstraintIfMissing("manifest_versions", "chk_manifest_versions_source_ref",
			`CHECK (source_ref IS NULL OR source_ref ~ '^[0-9a-f]{40}([0-9a-f]{24})?$')`),

		// --- manifest_deployments: dual-hash approval binding
		`ALTER TABLE public.manifest_deployments ADD COLUMN IF NOT EXISTS approved_bundle_hash character varying(64)`,
		`ALTER TABLE public.manifest_deployments ADD COLUMN IF NOT EXISTS approved_plan_hash character varying(64)`,
		// keys whose override values are sensitive (jsonb array of strings). Nullable
		// with no default and no SQL backfill: NULL = "not computed yet", treated as
		// all-sensitive by the API; filled by the startup backfill job in Go.
		`ALTER TABLE public.manifest_deployments ADD COLUMN IF NOT EXISTS sensitive_keys jsonb`,
		// task snapshot of the deployment overrides carries the same marker (NULL = all-sensitive)
		`ALTER TABLE public.workspace_tasks ADD COLUMN IF NOT EXISTS sensitive_keys jsonb`,

		// --- sandbox_sessions: user + workspace bound, VPC-only, no secrets stored
		`CREATE TABLE IF NOT EXISTS public.sandbox_sessions (
    id character varying(36) NOT NULL PRIMARY KEY,
    user_id character varying(20) NOT NULL,
    workspace_id character varying(50) NOT NULL,
    provider character varying(32) NOT NULL,
    network_mode character varying(16) NOT NULL DEFAULT 'vpc',
    status character varying(20) NOT NULL DEFAULT 'active',
    expires_at timestamp with time zone NOT NULL,
    closed_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT chk_sandbox_sessions_network_mode CHECK (network_mode = 'vpc'),
    CONSTRAINT uq_sandbox_sessions_id_workspace UNIQUE (id, workspace_id)
)`,
		`CREATE INDEX IF NOT EXISTS idx_sandbox_sessions_user_workspace ON public.sandbox_sessions (user_id, workspace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sandbox_sessions_open_expiry ON public.sandbox_sessions (expires_at) WHERE closed_at IS NULL`,

		// --- manifest_runs: one plan execution against an immutable bundle
		`CREATE TABLE IF NOT EXISTS public.manifest_runs (
    id character varying(36) NOT NULL PRIMARY KEY,
    manifest_id character varying(36) NOT NULL REFERENCES public.manifests(id) ON DELETE CASCADE,
    version_id character varying(36) REFERENCES public.manifest_versions(id) ON DELETE SET NULL,
    bundle_hash character varying(64) NOT NULL,
    workspace_id character varying(50) NOT NULL,
    runner character varying(16) NOT NULL,
    purpose character varying(16) NOT NULL,
    status character varying(20) NOT NULL DEFAULT 'pending',
    plan_hash character varying(64),
    plan_redacted jsonb,
    state_serial bigint,
    session_id character varying(36),
    created_by character varying(20) NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT chk_manifest_runs_runner CHECK (runner IN ('agent', 'sandbox')),
    CONSTRAINT chk_manifest_runs_purpose CHECK (purpose IN ('preview', 'approval')),
    CONSTRAINT chk_manifest_runs_sandbox_preview_only CHECK (runner <> 'sandbox' OR purpose = 'preview'),
    CONSTRAINT chk_manifest_runs_sandbox_session CHECK (runner <> 'sandbox' OR session_id IS NOT NULL),
    CONSTRAINT chk_manifest_runs_bundle_hash CHECK (bundle_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT fk_manifest_runs_session_workspace FOREIGN KEY (session_id, workspace_id) REFERENCES public.sandbox_sessions(id, workspace_id),
    CONSTRAINT uq_manifest_runs_id_workspace_purpose UNIQUE (id, workspace_id, purpose)
)`,
		`CREATE INDEX IF NOT EXISTS idx_manifest_runs_manifest ON public.manifest_runs (manifest_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_manifest_runs_workspace ON public.manifest_runs (workspace_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_manifest_runs_session ON public.manifest_runs (session_id) WHERE session_id IS NOT NULL`,

		// --- run_tokens: per-run platform token (hash only); cannot outscope its run/session
		`CREATE TABLE IF NOT EXISTS public.run_tokens (
    id bigserial PRIMARY KEY,
    run_id character varying(36) NOT NULL,
    session_id character varying(36),
    workspace_id character varying(50) NOT NULL,
    purpose character varying(16) NOT NULL,
    token_hash character varying(64) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_run_tokens_token_hash UNIQUE (token_hash),
    CONSTRAINT chk_run_tokens_purpose CHECK (purpose IN ('preview', 'approval')),
    CONSTRAINT fk_run_tokens_run FOREIGN KEY (run_id, workspace_id, purpose) REFERENCES public.manifest_runs(id, workspace_id, purpose) ON DELETE CASCADE,
    CONSTRAINT fk_run_tokens_session_workspace FOREIGN KEY (session_id, workspace_id) REFERENCES public.sandbox_sessions(id, workspace_id)
)`,
		`CREATE INDEX IF NOT EXISTS idx_run_tokens_run ON public.run_tokens (run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_run_tokens_session_active ON public.run_tokens (session_id) WHERE session_id IS NOT NULL AND revoked_at IS NULL`,
	}
}

// addConstraintIfMissing renders an idempotent ADD CONSTRAINT for an existing
// table (CREATE TABLE IF NOT EXISTS covers constraints of new tables).
func addConstraintIfMissing(table, name, definition string) string {
	return fmt.Sprintf(`DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s' AND conrelid = 'public.%s'::regclass) THEN
        ALTER TABLE public.%s ADD CONSTRAINT %s %s;
    END IF;
END $$`, name, table, table, name, definition)
}

// applyManifestSandboxSchema applies the DDL, then backfills bundle_hash of
// existing versions in the step-2 encoding (manifestbundle.VersionHashV1,
// identical to the SQL patch); 20261004_04_manifest_bundle_v2 verifies these
// v1 hashes and moves them to v2. Existing manifests become 'native' through the
// column default. Only rows with bundle_hash IS NULL are touched.
func applyManifestSandboxSchema(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range manifestSandboxSchemaStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("manifest sandbox schema: %w", err)
		}
	}
	return backfillManifestVersionBundleHashes(ctx, tx)
}

func backfillManifestVersionBundleHashes(ctx context.Context, tx *gorm.DB) error {
	var versionIDs []string
	q := tx.WithContext(ctx).Table("manifest_versions").Where("bundle_hash IS NULL")
	// a version that already carries a reason (bundle rules, or the sticky
	// hash_mismatch) stays NULL; the column only exists once 20261004_04 (or its
	// SQL patch) has run
	if tx.Migrator().HasColumn("manifest_versions", "bundle_invalid_reason") {
		q = q.Where("bundle_invalid_reason IS NULL")
	}
	if err := q.Order("id").Pluck("id", &versionIDs).Error; err != nil {
		return fmt.Errorf("list versions without bundle_hash: %w", err)
	}
	for _, id := range versionIDs {
		hash, err := manifestbundle.VersionHashV1(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Table("manifest_versions").
			Where("id = ? AND bundle_hash IS NULL", id).
			Update("bundle_hash", hash).Error; err != nil {
			return fmt.Errorf("backfill bundle_hash of version %s: %w", id, err)
		}
	}
	return nil
}
