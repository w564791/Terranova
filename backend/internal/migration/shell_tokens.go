package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// shellTokensStatements is the DDL of migration 20261025_02_shell_tokens
// (docs/manifest/manifest-sandbox-spec.md §10.2). Additive and idempotent.
// Stores SHA-256 of typ=shell JWTs bound to shell_sessions.
//
// Keep backend/migrations/add_shell_tokens.sql in sync.
func shellTokensStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS public.shell_tokens (
    id bigserial PRIMARY KEY,
    shell_session_id character varying(36) NOT NULL REFERENCES public.shell_sessions(id),
    sandbox_id character varying(36) NOT NULL,
    org_id bigint NOT NULL,
    workspace_id character varying(50) NOT NULL,
    token_hash character varying(64) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT uq_shell_tokens_token_hash UNIQUE (token_hash)
)`,
		`CREATE INDEX IF NOT EXISTS idx_shell_tokens_session ON public.shell_tokens (shell_session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_shell_tokens_sandbox_active ON public.shell_tokens (sandbox_id) WHERE revoked_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_shell_tokens_session_active ON public.shell_tokens (shell_session_id) WHERE revoked_at IS NULL`,
		`COMMENT ON TABLE public.shell_tokens IS 'Hash-only typ=shell JWTs; revoke when shell_session closes (§10.2)'`,
	}
}

func applyShellTokens(ctx context.Context, tx *gorm.DB) error {
	for _, s := range shellTokensStatements() {
		if err := tx.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("shell_tokens schema: %w", err)
		}
	}
	return nil
}
