package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// manifestGitSourceImmutableStatements is the DDL contract of migration
// 20261010_16_manifest_git_source_immutable: a BEFORE UPDATE trigger that
// rejects changes to source_type / git_repo_url / git_subpath /
// github_installation_id after the row exists. Create uses INSERT (trigger
// does not fire). git_latest_* remain updatable (webhook hints).
//
// Additive and idempotent. Keep backend/migrations/add_manifest_git_source_immutable.sql
// and the seed block in manifests/db/init_seed_data.sql in sync.
func manifestGitSourceImmutableStatements() []string {
	return []string{
		`CREATE OR REPLACE FUNCTION public.trg_manifests_git_source_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.source_type IS DISTINCT FROM NEW.source_type
       OR OLD.git_repo_url IS DISTINCT FROM NEW.git_repo_url
       OR OLD.git_subpath IS DISTINCT FROM NEW.git_subpath
       OR OLD.github_installation_id IS DISTINCT FROM NEW.github_installation_id THEN
        RAISE EXCEPTION 'git_source_immutable: source_type, git_repo_url, git_subpath, github_installation_id are immutable after creation'
            USING ERRCODE = 'P0001';
    END IF;
    RETURN NEW;
END;
$$`,
		`DROP TRIGGER IF EXISTS manifests_git_source_immutable ON public.manifests`,
		`CREATE TRIGGER manifests_git_source_immutable
    BEFORE UPDATE ON public.manifests
    FOR EACH ROW
    EXECUTE FUNCTION public.trg_manifests_git_source_immutable()`,
	}
}

func applyManifestGitSourceImmutable(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range manifestGitSourceImmutableStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("manifest git source immutable: %w", err)
		}
	}
	return nil
}
