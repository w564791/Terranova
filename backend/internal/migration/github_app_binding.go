package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// githubAppBindingStatements is the DDL contract of migration
// 20261010_15_github_app_binding (GitHub App installations are bound to an
// organization only through the App's setup callback, with user-to-server
// OAuth proof that the installing GitHub user administers the account):
//   - github_app_installations: account_id / account_type from GitHub and the
//     proof (verified_github_user_id, verified_github_login, verified_at).
//     Rows without verified_at (registered manually before) are not usable
//     until the installation is connected again through the callback.
//   - uq_github_app_installations_installation becomes a UNIQUE constraint
//     (platform-wide: an installation belongs to at most one organization).
//   - github_app_setup_nonces: consumed state nonces (single use; rows are
//     deleted after the state expired).
//   - github_webhook_deliveries: X-GitHub-Delivery ids already processed
//     (replay protection; rows older than 72h are deleted).
//
// Additive and idempotent. Keep backend/migrations/add_github_app_binding.sql
// and the seed block in manifests/db/init_seed_data.sql in sync.
func githubAppBindingStatements() []string {
	return []string{
		`ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS account_id bigint`,
		`ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS account_type character varying(20)`,
		`ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS verified_github_user_id bigint`,
		`ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS verified_github_login character varying(255)`,
		`ALTER TABLE public.github_app_installations ADD COLUMN IF NOT EXISTS verified_at timestamp with time zone`,
		`DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uq_github_app_installations_installation' AND conrelid = 'public.github_app_installations'::regclass) THEN
        IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
                   WHERE n.nspname = 'public' AND c.relname = 'uq_github_app_installations_installation' AND c.relkind = 'i') THEN
            ALTER TABLE public.github_app_installations ADD CONSTRAINT uq_github_app_installations_installation UNIQUE USING INDEX uq_github_app_installations_installation;
        ELSE
            ALTER TABLE public.github_app_installations ADD CONSTRAINT uq_github_app_installations_installation UNIQUE (installation_id);
        END IF;
    END IF;
END $$`,
		`DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_github_app_installations_verified' AND conrelid = 'public.github_app_installations'::regclass) THEN
        ALTER TABLE public.github_app_installations ADD CONSTRAINT chk_github_app_installations_verified CHECK (
            verified_at IS NULL
            OR (verified_github_user_id IS NOT NULL AND verified_github_login IS NOT NULL
                AND account_id IS NOT NULL AND account_type IN ('Organization', 'User')));
    END IF;
END $$`,
		`CREATE TABLE IF NOT EXISTS public.github_app_setup_nonces (
    nonce character varying(64) PRIMARY KEY,
    organization_id integer NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    user_id character varying(20) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    consumed_at timestamp with time zone NOT NULL DEFAULT now()
)`,
		`CREATE INDEX IF NOT EXISTS idx_github_app_setup_nonces_expires ON public.github_app_setup_nonces (expires_at)`,
		`CREATE TABLE IF NOT EXISTS public.github_webhook_deliveries (
    delivery_id character varying(64) PRIMARY KEY,
    event character varying(64) NOT NULL,
    received_at timestamp with time zone NOT NULL DEFAULT now()
)`,
		`CREATE INDEX IF NOT EXISTS idx_github_webhook_deliveries_received ON public.github_webhook_deliveries (received_at)`,
	}
}

func applyGitHubAppBinding(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range githubAppBindingStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("github app binding: %w", err)
		}
	}
	return nil
}
