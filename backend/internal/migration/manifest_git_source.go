package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// manifestGitSourceStatements is the DDL contract of migration
// 20261010_14_manifest_git_source (spec §9 step 8, git source):
//   - github_app_installations: GitHub App installations an org ADMIN
//     registered for the organization (account_login as reported by GitHub).
//     An installation belongs to at most one organization (unique
//     installation_id), so one tenant cannot use another tenant's
//     installation. git manifests must reference a registered installation.
//   - manifests.git_latest_sha / git_latest_ref / git_latest_at: the last
//     push a verified webhook reported for the manifest's repo ("new commit
//     available"); never published automatically.
//
// Additive and idempotent. Keep backend/migrations/add_manifest_git_source.sql
// and the seed block in manifests/db/init_seed_data.sql in sync.
func manifestGitSourceStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS public.github_app_installations (
    id bigserial PRIMARY KEY,
    organization_id integer NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    installation_id bigint NOT NULL CHECK (installation_id > 0),
    account_login character varying(255) NOT NULL,
    created_by character varying(20) NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now()
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_github_app_installations_installation ON public.github_app_installations (installation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_github_app_installations_org ON public.github_app_installations (organization_id)`,
		`ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_latest_sha character varying(64)`,
		`ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_latest_ref character varying(255)`,
		`ALTER TABLE public.manifests ADD COLUMN IF NOT EXISTS git_latest_at timestamp with time zone`,
		`CREATE INDEX IF NOT EXISTS idx_manifests_git_installation ON public.manifests (github_installation_id) WHERE source_type = 'git'`,
		`DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_manifests_git_latest' AND conrelid = 'public.manifests'::regclass) THEN
        ALTER TABLE public.manifests ADD CONSTRAINT chk_manifests_git_latest CHECK (
            (git_latest_sha IS NULL AND git_latest_ref IS NULL AND git_latest_at IS NULL)
            OR (source_type = 'git' AND git_latest_sha ~ '^([0-9a-f]{40}|[0-9a-f]{64})$' AND git_latest_at IS NOT NULL));
    END IF;
END $$`,
	}
}

func applyManifestGitSource(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range manifestGitSourceStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("manifest git source: %w", err)
		}
	}
	return nil
}
