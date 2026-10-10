package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// runTokenBindingStatements is the DDL contract of migration
// 20261010_12_run_token_binding: the agent a manifest run is assigned to
// (manifest_runs.agent_id; only that agent may obtain the run's token) and
// the agent that obtained a run token (run_tokens.agent_id; revoking the
// agent revokes its run tokens). Additive and idempotent. Keep
// backend/migrations/add_run_token_binding.sql and the seed block in
// manifests/db/init_seed_data.sql in sync.
func runTokenBindingStatements() []string {
	return []string{
		`ALTER TABLE public.manifest_runs ADD COLUMN IF NOT EXISTS agent_id character varying(50)`,
		`ALTER TABLE public.run_tokens ADD COLUMN IF NOT EXISTS agent_id character varying(50)`,
		`CREATE INDEX IF NOT EXISTS idx_run_tokens_agent_active ON public.run_tokens (agent_id) WHERE agent_id IS NOT NULL AND revoked_at IS NULL`,
	}
}

func applyRunTokenBinding(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range runTokenBindingStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("run token binding: %w", err)
		}
	}
	return nil
}
