package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// agentTokenStatements is the DDL contract of migration
// 20261010_11_agent_token (per-agent JWT revocation state): the token
// generation carried in agent tokens, the revocation time, and the pool token
// the agent registered with. Additive and idempotent. Keep
// backend/migrations/add_agent_token.sql and the seed block in
// manifests/db/init_seed_data.sql in sync.
func agentTokenStatements() []string {
	return []string{
		`ALTER TABLE public.agents ADD COLUMN IF NOT EXISTS token_generation integer DEFAULT 0 NOT NULL`,
		`ALTER TABLE public.agents ADD COLUMN IF NOT EXISTS revoked_at timestamp with time zone`,
		`ALTER TABLE public.agents ADD COLUMN IF NOT EXISTS pool_token_hash character varying(64)`,
	}
}

func applyAgentToken(ctx context.Context, tx *gorm.DB) error {
	for _, stmt := range agentTokenStatements() {
		if err := tx.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("agent token columns: %w", err)
		}
	}
	return nil
}
