package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// variableSetOrgRequiredStatements is the DDL contract of migration
// 20261016_02_variable_set_org_required: specific varsets must carry org_id.
// Added NOT VALID so legacy orphans (specific + NULL org_id, left by the
// 20261016_01 backfill) do not block the release; every new INSERT/UPDATE is
// checked. Orphans are healed on write (creator only). Ops: once
// scripts/varset_orphan_stats.sql reports 0, run
//
//	ALTER TABLE public.variable_sets VALIDATE CONSTRAINT chk_variable_sets_specific_org;
//
// Keep backend/migrations/add_variable_set_org_required.sql in sync.
func variableSetOrgRequiredStatements() []string {
	return []string{
		`DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
     WHERE conname = 'chk_variable_sets_specific_org'
       AND conrelid = 'public.variable_sets'::regclass
  ) THEN
    ALTER TABLE public.variable_sets
      ADD CONSTRAINT chk_variable_sets_specific_org
      CHECK (scope <> 'specific' OR org_id IS NOT NULL) NOT VALID;
  END IF;
END $$`,
	}
}

func applyVariableSetOrgRequired(ctx context.Context, tx *gorm.DB) error {
	for _, s := range variableSetOrgRequiredStatements() {
		if err := tx.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("variable_set org required: %w", err)
		}
	}
	return nil
}
