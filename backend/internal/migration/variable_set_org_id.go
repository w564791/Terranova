package migration

import (
	"context"
	"fmt"
	"log"

	"gorm.io/gorm"
)

// variableSetOrgIDStatements is the DDL contract of migration
// 20261016_01_variable_set_org_id. Additive and idempotent.
//
// org_id is nullable. CHECK: scope=global ⇒ org_id IS NULL; scope=specific
// may temporarily keep org_id NULL only for legacy unassigned orphans (set on
// next mount). New specific creates always set org_id.
//
// Name uniqueness: (org_id, name) for non-deleted specific rows; partial
// unique on name where org_id IS NULL for global (and residual orphans).
//
// Keep backend/migrations/add_variable_set_org_id.sql in sync.
func variableSetOrgIDStatements() []string {
	return []string{
		`ALTER TABLE public.variable_sets ADD COLUMN IF NOT EXISTS org_id bigint`,
		`DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
     WHERE conname = 'chk_variable_sets_org_scope'
       AND conrelid = 'public.variable_sets'::regclass
  ) THEN
    ALTER TABLE public.variable_sets
      ADD CONSTRAINT chk_variable_sets_org_scope CHECK (
        (scope = 'global' AND org_id IS NULL)
        OR (scope = 'specific')
      );
  END IF;
END $$`,
		`DROP INDEX IF EXISTS public.idx_variable_sets_name`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_variable_sets_org_name
		   ON public.variable_sets (org_id, name)
		   WHERE is_deleted = false AND org_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_variable_sets_global_name
		   ON public.variable_sets (name)
		   WHERE is_deleted = false AND org_id IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_variable_sets_org_id
		   ON public.variable_sets (org_id) WHERE org_id IS NOT NULL`,
		`COMMENT ON COLUMN public.variable_sets.org_id IS 'Owning organization for specific varsets; NULL for global (and legacy unassigned orphans pending first mount)'`,
	}
}

func applyVariableSetOrgID(ctx context.Context, tx *gorm.DB) error {
	for _, s := range variableSetOrgIDStatements() {
		if err := tx.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("variable_set org_id schema: %w", err)
		}
	}
	if err := backfillVariableSetOrgID(ctx, tx); err != nil {
		return fmt.Errorf("variable_set org_id backfill: %w", err)
	}
	return nil
}

// backfillVariableSetOrgID is idempotent (only touches rows still NULL org_id,
// non-global, not deleted).
//
//  1. Assigned, all mounts in one org → that org_id.
//  2. Cross-org mounts: org with most mounts; delete other-org mounts after WARN.
//  3. Unassigned + created_by: creator's earliest-joined org with VARIABLE_SETS
//     WRITE (level WRITE or ADMIN); else leave NULL.
//  4. global: keep NULL (already).
func backfillVariableSetOrgID(ctx context.Context, tx *gorm.DB) error {
	type vsRow struct {
		VarsetID  string
		Scope     string
		CreatedBy *string
	}
	var rows []vsRow
	if err := tx.WithContext(ctx).Raw(`
SELECT varset_id, scope, created_by
  FROM variable_sets
 WHERE is_deleted = false
   AND scope = 'specific'
   AND org_id IS NULL
`).Scan(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		orgCounts, err := mountOrgCounts(ctx, tx, row.VarsetID)
		if err != nil {
			return err
		}
		if len(orgCounts) == 1 {
			if err := tx.WithContext(ctx).Exec(
				`UPDATE variable_sets SET org_id = ? WHERE varset_id = ? AND org_id IS NULL`,
				orgCounts[0].OrgID, row.VarsetID,
			).Error; err != nil {
				return err
			}
			continue
		}
		if len(orgCounts) > 1 {
			winner := orgCounts[0]
			for _, c := range orgCounts[1:] {
				if c.Cnt > winner.Cnt || (c.Cnt == winner.Cnt && c.OrgID < winner.OrgID) {
					winner = c
				}
			}
			log.Printf("WARN audit: varset %s had cross-org mounts; keeping org_id=%d (most mounts=%d); dropping mounts in other orgs",
				row.VarsetID, winner.OrgID, winner.Cnt)
			if err := deleteMountsOutsideOrg(ctx, tx, row.VarsetID, winner.OrgID); err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Exec(
				`UPDATE variable_sets SET org_id = ? WHERE varset_id = ? AND org_id IS NULL`,
				winner.OrgID, row.VarsetID,
			).Error; err != nil {
				return err
			}
			continue
		}
		// Unassigned.
		if row.CreatedBy == nil || *row.CreatedBy == "" {
			continue
		}
		orgID, ok, err := creatorOrgWithVarsetWrite(ctx, tx, *row.CreatedBy)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := tx.WithContext(ctx).Exec(
			`UPDATE variable_sets SET org_id = ? WHERE varset_id = ? AND org_id IS NULL`,
			orgID, row.VarsetID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

type orgCount struct {
	OrgID uint
	Cnt   int64
}

func mountOrgCounts(ctx context.Context, tx *gorm.DB, varsetID string) ([]orgCount, error) {
	var out []orgCount
	err := tx.WithContext(ctx).Raw(`
SELECT org_id, COUNT(*) AS cnt FROM (
  SELECT p.org_id AS org_id
    FROM varset_assignments a
    JOIN projects p ON p.id = a.project_id
   WHERE a.varset_id = ? AND a.scope_type = 'project'
  UNION ALL
  SELECT p.org_id AS org_id
    FROM varset_assignments a
    JOIN workspace_project_relations wpr ON wpr.workspace_id = a.workspace_id
    JOIN projects p ON p.id = wpr.project_id
   WHERE a.varset_id = ? AND a.scope_type = 'workspace'
) mounts
 GROUP BY org_id
 ORDER BY cnt DESC, org_id ASC
`, varsetID, varsetID).Scan(&out).Error
	return out, err
}

func deleteMountsOutsideOrg(ctx context.Context, tx *gorm.DB, varsetID string, keepOrg uint) error {
	if err := tx.WithContext(ctx).Exec(`
DELETE FROM varset_assignments
 WHERE varset_id = ?
   AND scope_type = 'project'
   AND project_id IN (SELECT id FROM projects WHERE org_id <> ?)
`, varsetID, keepOrg).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(`
DELETE FROM varset_assignments
 WHERE varset_id = ?
   AND scope_type = 'workspace'
   AND workspace_id IN (
     SELECT wpr.workspace_id
       FROM workspace_project_relations wpr
       JOIN projects p ON p.id = wpr.project_id
      WHERE p.org_id <> ?
   )
`, varsetID, keepOrg).Error
}

// creatorOrgWithVarsetWrite picks the creator's earliest-joined org where they
// hold VARIABLE_SETS at WRITE or ADMIN (via iam_user_roles + iam_role_policies
// or iam_team_roles through team membership is out of scope for this backfill;
// user_organizations + ORGANIZATION-scoped role assignment only).
func creatorOrgWithVarsetWrite(ctx context.Context, tx *gorm.DB, userID string) (uint, bool, error) {
	var orgID *uint
	err := tx.WithContext(ctx).Raw(`
SELECT uo.org_id
  FROM user_organizations uo
 WHERE uo.user_id = ?
   AND EXISTS (
     SELECT 1
       FROM iam_user_roles ur
       JOIN iam_role_policies rp ON rp.role_id = ur.role_id
       JOIN permission_definitions pd ON pd.id = rp.permission_id
      WHERE ur.user_id = uo.user_id
        AND ur.scope_type = 'ORGANIZATION'
        AND ur.scope_id = uo.org_id
        AND (ur.expires_at IS NULL OR ur.expires_at > CURRENT_TIMESTAMP)
        AND pd.resource_type = 'VARIABLE_SETS'
        AND pd.scope_level = 'ORGANIZATION'
        AND rp.scope_type = 'ORGANIZATION'
        AND rp.permission_level IN ('WRITE', 'ADMIN')
   )
 ORDER BY uo.joined_at ASC, uo.org_id ASC
 LIMIT 1
`, userID).Scan(&orgID).Error
	if err != nil {
		return 0, false, err
	}
	if orgID == nil {
		return 0, false, nil
	}
	return *orgID, true, nil
}
