// Package migration provides the database migration entry point used by
// deployment jobs.  Application processes deliberately do not mutate schema
// on startup; a failed migration must block the release instead of leaving a
// mixed-version fleet serving traffic.
package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const advisoryLockName = "iac-platform-schema-migrations-v1"

type appliedMigration struct {
	Version   string    `gorm:"primaryKey;type:varchar(128)"`
	Checksum  string    `gorm:"not null;type:varchar(64)"`
	AppliedAt time.Time `gorm:"not null"`
}

func (appliedMigration) TableName() string { return "schema_migrations" }

type definition struct {
	version  string
	contract string
	apply    func(context.Context, *gorm.DB) error
}

// Run applies every registered migration exactly once. It is safe for many
// rollout jobs to start concurrently: PostgreSQL serializes them with a
// transaction-scoped advisory lock. A changed migration checksum is fatal,
// because silently replaying edited history makes upgrades non-auditable.
func Run(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("migration database is not configured")
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", advisoryLockName).Error; err != nil {
			return fmt.Errorf("acquire migration advisory lock: %w", err)
		}
		if err := tx.AutoMigrate(&appliedMigration{}); err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}

		for _, m := range definitions() {
			checksum := checksumFor(m)
			var applied appliedMigration
			err := tx.Where("version = ?", m.version).First(&applied).Error
			switch {
			case err == nil:
				if applied.Checksum != checksum {
					return fmt.Errorf("migration %s checksum mismatch: recorded=%s current=%s", m.version, applied.Checksum, checksum)
				}
				continue
			case !errors.Is(err, gorm.ErrRecordNotFound):
				return fmt.Errorf("read migration %s: %w", m.version, err)
			}

			if err := m.apply(ctx, tx); err != nil {
				return fmt.Errorf("apply migration %s: %w", m.version, err)
			}
			if err := tx.Create(&appliedMigration{
				Version:   m.version,
				Checksum:  checksum,
				AppliedAt: time.Now().UTC(),
			}).Error; err != nil {
				return fmt.Errorf("record migration %s: %w", m.version, err)
			}
		}
		return nil
	})
}

func definitions() []definition {
	return []definition{
		// Keep semantics for already-recorded versions stable. Retry-safe fixes
		// for a migration that failed before its record was written may remain in
		// that migration; v5 reconciles databases that already recorded v1-v4.
		{version: "20260717_01_iam_multitenant_hardening", contract: "tenant-boundary-v2", apply: applyIAMMultitenantHardening},
		{version: "20260717_02_iam_role_data_transition", contract: "role-principal-transition-v1", apply: applyIAMRoleDataTransition},
		{version: "20260717_03_iam_release_completeness", contract: "builtin-role-and-index-v1", apply: applyIAMReleaseCompleteness},
		{version: "20260717_04_iam_application_role_transition", contract: "application-direct-grant-transition-v1", apply: migrateLegacyApplicationDirectGrantsToRoles},
		// Reconcile installations which recorded the earlier migrations before
		// the tenant-boundary rules were complete.  Keep this separate so their
		// historical checksums remain valid.
		{version: "20260717_05_iam_role_tenant_reconciliation", contract: "role-quarantine-and-application-tenant-v1", apply: applyIAMRoleTenantReconciliation},
	}
}

// checksumFor intentionally derives from the version and implementation
// contract. Behaviour that changes a recorded migration belongs in a new
// version; retry-safe handling for an unrecorded transaction is an exception.
func checksumFor(m definition) string {
	s := sha256.Sum256([]byte(m.version + ":" + m.contract))
	return hex.EncodeToString(s[:])
}

func applyIAMMultitenantHardening(ctx context.Context, tx *gorm.DB) error {
	// Application app_key is app_ + 32 hex characters. Legacy permission
	// tables used varchar(20), which made both new grants and the id→key data
	// migration fail at the database boundary.
	for _, statement := range []string{
		"ALTER TABLE org_permissions ALTER COLUMN principal_id TYPE varchar(64)",
		"ALTER TABLE project_permissions ALTER COLUMN principal_id TYPE varchar(64)",
		"ALTER TABLE workspace_permissions ALTER COLUMN principal_id TYPE varchar(64)",
		"ALTER TABLE applications ADD COLUMN IF NOT EXISTS workspace_tag_filter JSONB DEFAULT NULL",
		"ALTER TABLE iam_roles ADD COLUMN IF NOT EXISTS org_id BIGINT NOT NULL DEFAULT 0",
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	// A workspace must have one owning project before its tenant can be used
	// when normalizing an application principal.  Failing before the rewrite
	// avoids selecting an arbitrary organization from malformed legacy data.
	if err := failOnDuplicateWorkspaceRelations(ctx, tx); err != nil {
		return err
	}

	// Normalize historic numeric APPLICATION principal IDs only after widening
	// the target columns. The runtime continues to accept aliases during the
	// transition, but storage has one canonical form: applications.app_key.
	// Every rewrite is bound to the organization that owns the granted resource;
	// a numeric ID must never be resolved through an application in another org.
	for _, statement := range legacyApplicationPrincipalNormalizationStatements() {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}

	if err := tx.WithContext(ctx).Exec("CREATE UNIQUE INDEX IF NOT EXISTS uq_workspace_project_relations_workspace_id ON workspace_project_relations (workspace_id)").Error; err != nil {
		return err
	}

	if err := reconcileIAMRoleTenantBoundary(ctx, tx); err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(`
DO $$
DECLARE r RECORD;
BEGIN
  FOR r IN
    SELECT c.conname
      FROM pg_constraint c
      JOIN pg_class t ON c.conrelid = t.oid
     WHERE t.relname = 'iam_roles'
       AND c.contype = 'u'
       AND pg_get_constraintdef(c.oid) ILIKE '%(name)%'
       AND pg_get_constraintdef(c.oid) NOT ILIKE '%org_id%'
  LOOP
    EXECUTE format('ALTER TABLE iam_roles DROP CONSTRAINT %I', r.conname);
  END LOOP;
END $$`).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec("DROP INDEX IF EXISTS idx_iam_roles_name").Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_role_org_name ON iam_roles (org_id, name)").Error; err != nil {
		return err
	}
	for _, statement := range []string{
		"DROP INDEX IF EXISTS uq_app_roles_active",
		`CREATE TABLE IF NOT EXISTS iam_application_roles (
  id BIGSERIAL PRIMARY KEY,
  application_principal_id VARCHAR(64) NOT NULL,
  role_id INTEGER NOT NULL REFERENCES iam_roles(id),
  scope_type VARCHAR(20) NOT NULL,
  scope_id BIGINT NOT NULL,
  assigned_by VARCHAR(20),
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ,
  reason TEXT
)`,
		"CREATE INDEX IF NOT EXISTS idx_app_roles_principal ON iam_application_roles (application_principal_id)",
		"CREATE INDEX IF NOT EXISTS idx_app_roles_scope ON iam_application_roles (scope_type, scope_id)",
		// A time-dependent partial index is invalid in PostgreSQL because NOW()
		// is not immutable. The handler treats a logical assignment as one row,
		// so an unconditional identity key is both valid and race-safe.
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_app_roles_identity ON iam_application_roles (application_principal_id, role_id, scope_type, scope_id)",
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func legacyApplicationPrincipalNormalizationStatements() []string {
	return []string{
		`UPDATE org_permissions op
   SET principal_id = a.app_key
  FROM applications a
 WHERE op.principal_type = 'APPLICATION'
   AND op.principal_id ~ '^[0-9]+$'
   AND a.id::text = op.principal_id
   AND a.org_id = op.org_id
   AND a.app_key IS NOT NULL AND a.app_key <> ''`,
		`UPDATE project_permissions pp
   SET principal_id = a.app_key
  FROM applications a, projects p
 WHERE p.id = pp.project_id
   AND pp.principal_type = 'APPLICATION'
   AND pp.principal_id ~ '^[0-9]+$'
   AND a.id::text = pp.principal_id
   AND a.org_id = p.org_id
   AND a.app_key IS NOT NULL AND a.app_key <> ''`,
		`UPDATE workspace_permissions wp
   SET principal_id = a.app_key
  FROM applications a,
       workspaces w,
       workspace_project_relations wpr,
       projects p
 WHERE w.workspace_id = wp.workspace_id
   AND wpr.workspace_id = w.workspace_id
   AND p.id = wpr.project_id
   AND wp.principal_type = 'APPLICATION'
   AND wp.principal_id ~ '^[0-9]+$'
   AND a.id::text = wp.principal_id
   AND a.org_id = p.org_id
   AND a.app_key IS NOT NULL AND a.app_key <> ''`,
	}
}

// applyIAMRoleDataTransition makes the authorization changes from the first
// migration usable for existing installations. It deliberately retains the
// legacy direct-grant rows: the running application dual-reads them during
// the staged cutover, while these synthesized Roles make the new model
// observable and give us a safe, auditable removal path later.
func applyIAMRoleDataTransition(ctx context.Context, tx *gorm.DB) error {
	if err := ensureSystemAdminRoleCoverage(ctx, tx); err != nil {
		return fmt.Errorf("restore system administrator role coverage: %w", err)
	}
	if err := migrateLegacyDirectGrantsToRoles(ctx, tx); err != nil {
		return fmt.Errorf("synthesize legacy direct grants as roles: %w", err)
	}
	if err := repairUserOrganizationMemberships(ctx, tx); err != nil {
		return fmt.Errorf("repair user organization memberships: %w", err)
	}
	return nil
}

// applyIAMReleaseCompleteness contains follow-on hardening that must remain
// separately versioned from the data transition. Never edit a migration that
// an early release may have already recorded; add a new version instead.
func applyIAMReleaseCompleteness(ctx context.Context, tx *gorm.DB) error {
	if err := ensureWorkspaceAdminRoleCoverage(ctx, tx); err != nil {
		return fmt.Errorf("ensure workspace administrator role: %w", err)
	}
	if err := installIAMSupportingIndexes(ctx, tx); err != nil {
		return fmt.Errorf("install IAM supporting indexes: %w", err)
	}
	return nil
}

// applyIAMRoleTenantReconciliation is deliberately a follow-on migration for
// databases that had already recorded the earlier IAM migrations.  The same
// reconciliation is also called from v1 so an unrecorded v1 failure can be
// retried safely instead of requiring a manual data edit.
func applyIAMRoleTenantReconciliation(ctx context.Context, tx *gorm.DB) error {
	if err := reconcileIAMRoleTenantBoundary(ctx, tx); err != nil {
		return fmt.Errorf("reconcile IAM role tenant boundary: %w", err)
	}
	return nil
}

func ensureSystemAdminRoleCoverage(ctx context.Context, tx *gorm.DB) error {
	// The business API no longer treats is_system_admin as an IAM bypass. Keep
	// the platform identity for platform-only APIs, but give active platform
	// administrators a real, explicit administrator Role in each tenant.
	for _, statement := range []string{
		`INSERT INTO iam_roles
  (org_id, name, display_name, description, is_system, is_active, created_at, updated_at)
SELECT 0, 'admin', 'Platform Administrator',
       'System role used to grant administrators explicit tenant access',
       true, true, NOW(), NOW()
 WHERE NOT EXISTS (
   SELECT 1 FROM iam_roles
    WHERE name = 'admin' AND is_system = true AND org_id = 0
 )`,
		`UPDATE iam_roles
    SET is_active = true, updated_at = NOW()
  WHERE name = 'admin' AND is_system = true AND org_id = 0 AND is_active = false`,
		// An organization-scoped Role policy legitimately covers resources below
		// it in the hierarchy. Supplying every current definition here prevents
		// the removal of the old bypass from locking platform administrators out
		// of IAM administration or newly introduced resource types.
		`INSERT INTO iam_role_policies
  (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id, pd.id, 'ADMIN', 'ORGANIZATION', NOW()
  FROM iam_roles r
 CROSS JOIN permission_definitions pd
 WHERE r.name = 'admin'
   AND r.is_system = true
   AND r.org_id = 0
   AND pd.scope_level IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')
   AND NOT EXISTS (
     SELECT 1 FROM iam_role_policies rp
      WHERE rp.role_id = r.id
        AND rp.permission_id = pd.id
        AND rp.scope_type = 'ORGANIZATION'
   )`,
		// Repair a pre-existing, expired assignment before inserting missing
		// rows. The unique key in normal installations means an expired record
		// otherwise prevents re-granting the administrator role.
		`UPDATE iam_user_roles ur
    SET expires_at = NULL,
        assigned_by = 'system:migration',
        assigned_at = NOW(),
        reason = 'IAM migration: explicit administrator access'
   FROM users u, organizations o, iam_roles r
  WHERE ur.user_id = u.user_id
    AND ur.role_id = r.id
    AND ur.scope_type = 'ORGANIZATION'
    AND ur.scope_id = o.id
    AND u.is_system_admin = true
    AND COALESCE(u.is_active, true)
    AND COALESCE(o.is_active, true)
    AND r.name = 'admin'
    AND r.is_system = true
    AND r.org_id = 0`,
		`INSERT INTO iam_user_roles
  (user_id, role_id, scope_type, scope_id, assigned_by, assigned_at, expires_at, reason)
SELECT u.user_id, r.id, 'ORGANIZATION', o.id,
       'system:migration', NOW(), NULL,
       'IAM migration: explicit administrator access'
  FROM users u
 CROSS JOIN organizations o
 CROSS JOIN iam_roles r
 WHERE u.is_system_admin = true
   AND COALESCE(u.is_active, true)
   AND COALESCE(o.is_active, true)
   AND r.name = 'admin'
   AND r.is_system = true
   AND r.org_id = 0
   AND NOT EXISTS (
     SELECT 1 FROM iam_user_roles ur
      WHERE ur.user_id = u.user_id
        AND ur.role_id = r.id
        AND ur.scope_type = 'ORGANIZATION'
        AND ur.scope_id = o.id
   )`,
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureWorkspaceAdminRoleCoverage(ctx context.Context, tx *gorm.DB) error {
	// Workspace creation binds this built-in Role to the creator. Ensure the
	// Role and its minimum policy set exist before application instances may
	// serve traffic, instead of relying on an optional manual SQL patch.
	for _, statement := range []string{
		`INSERT INTO iam_roles
  (org_id, name, display_name, description, is_system, is_active, created_at, updated_at)
SELECT 0, 'workspace_admin', 'Workspace Administrator',
       'System role for explicit workspace creator administration',
       true, true, NOW(), NOW()
 WHERE NOT EXISTS (
   SELECT 1 FROM iam_roles
    WHERE name = 'workspace_admin' AND is_system = true AND org_id = 0
 )`,
		`UPDATE iam_roles
    SET is_active = true, updated_at = NOW()
  WHERE name = 'workspace_admin' AND is_system = true AND org_id = 0 AND is_active = false`,
		`INSERT INTO iam_role_policies
  (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id, pd.id,
       CASE WHEN pd.id = 'wspm-workspace-state-sensitive' THEN 'READ' ELSE 'ADMIN' END,
       'WORKSPACE', NOW()
  FROM iam_roles r
 CROSS JOIN permission_definitions pd
 WHERE r.name = 'workspace_admin'
   AND r.is_system = true
   AND r.org_id = 0
   AND pd.scope_level = 'WORKSPACE'
   AND (
     pd.resource_type IN (
       'WORKSPACE_MANAGEMENT',
       'WORKSPACE_EXECUTION',
       'WORKSPACE_STATE',
       'WORKSPACE_VARIABLES',
       'WORKSPACE_RESOURCES',
       'TASK_DATA_ACCESS'
     )
     OR pd.id = 'wspm-workspace-state-sensitive'
   )
   AND NOT EXISTS (
     SELECT 1 FROM iam_role_policies rp
      WHERE rp.role_id = r.id
        AND rp.permission_id = pd.id
        AND rp.scope_type = 'WORKSPACE'
   )`,
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacyDirectGrantsToRoles(ctx context.Context, tx *gorm.DB) error {
	// Only migrate grants whose resource definition can be legally hosted at
	// the assignment layer. This is intentionally narrower than the historic
	// direct-grant evaluator: malformed rows remain as source evidence instead
	// of gaining a new path to authority during the transition.
	if err := tx.WithContext(ctx).Exec(`
CREATE TEMP TABLE _iam_legacy_direct_grant_groups ON COMMIT DROP AS
SELECT
  op.principal_type,
  op.principal_id,
  'ORGANIZATION'::text AS scope_type,
  op.org_id::bigint AS scope_id,
  op.org_id::bigint AS org_id,
  op.expires_at,
  md5(op.principal_type || ':' || op.principal_id || ':ORGANIZATION:' || op.org_id::text || ':' || COALESCE(op.expires_at::text, 'never')) AS ghash
FROM org_permissions op
JOIN permission_definitions pd ON pd.id = op.permission_id
WHERE op.principal_type IN ('USER', 'TEAM')
  AND (op.expires_at IS NULL OR op.expires_at > NOW())
  AND op.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
  AND pd.scope_level IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')
GROUP BY op.principal_type, op.principal_id, op.org_id, op.expires_at

UNION

SELECT
  pp.principal_type,
  pp.principal_id,
  'PROJECT'::text AS scope_type,
  pp.project_id::bigint AS scope_id,
  p.org_id::bigint AS org_id,
  pp.expires_at,
  md5(pp.principal_type || ':' || pp.principal_id || ':PROJECT:' || pp.project_id::text || ':' || COALESCE(pp.expires_at::text, 'never')) AS ghash
FROM project_permissions pp
JOIN projects p ON p.id = pp.project_id
JOIN permission_definitions pd ON pd.id = pp.permission_id
WHERE pp.principal_type IN ('USER', 'TEAM')
  AND (pp.expires_at IS NULL OR pp.expires_at > NOW())
  AND pp.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
  AND pd.scope_level IN ('PROJECT', 'WORKSPACE')
GROUP BY pp.principal_type, pp.principal_id, pp.project_id, p.org_id, pp.expires_at

UNION

SELECT
  wp.principal_type,
  wp.principal_id,
  'WORKSPACE'::text AS scope_type,
  w.id::bigint AS scope_id,
  p.org_id::bigint AS org_id,
  wp.expires_at,
  md5(wp.principal_type || ':' || wp.principal_id || ':WORKSPACE:' || w.id::text || ':' || COALESCE(wp.expires_at::text, 'never')) AS ghash
FROM workspace_permissions wp
JOIN workspaces w ON w.workspace_id = wp.workspace_id
JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
JOIN projects p ON p.id = wpr.project_id
JOIN permission_definitions pd ON pd.id = wp.permission_id
WHERE wp.principal_type IN ('USER', 'TEAM')
  AND (wp.expires_at IS NULL OR wp.expires_at > NOW())
  AND wp.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
  AND pd.scope_level = 'WORKSPACE'
GROUP BY wp.principal_type, wp.principal_id, w.id, p.org_id, wp.expires_at`).Error; err != nil {
		return err
	}

	for _, statement := range []string{
		// Use the full digest in the durable role name. Truncating it made a
		// collision merge unrelated principals' policies into one Role.
		`INSERT INTO iam_roles
  (org_id, name, display_name, description, is_system, is_active, created_at, updated_at)
SELECT DISTINCT g.org_id,
       'legacy_dg_' || g.ghash,
       'Legacy Direct Grant',
       'Auto-synthesized from direct grants; principal=' || g.principal_type || '/' || g.principal_id ||
         ' scope=' || g.scope_type || '/' || g.scope_id::text,
       false, true, NOW(), NOW()
  FROM _iam_legacy_direct_grant_groups g
 WHERE NOT EXISTS (
   SELECT 1 FROM iam_roles r
    WHERE r.org_id = g.org_id AND r.name = 'legacy_dg_' || g.ghash
 )`,
		`INSERT INTO iam_role_policies
  (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id,
       op.permission_id,
       CASE MAX(CASE op.permission_level::text
                    WHEN '3' THEN 3 WHEN 'ADMIN' THEN 3
                    WHEN '2' THEN 2 WHEN 'WRITE' THEN 2
                    WHEN '1' THEN 1 WHEN 'READ' THEN 1
                    ELSE 0 END)
         WHEN 3 THEN 'ADMIN' WHEN 2 THEN 'WRITE' ELSE 'READ' END,
       'ORGANIZATION', NOW()
  FROM org_permissions op
  JOIN permission_definitions pd ON pd.id = op.permission_id
  JOIN _iam_legacy_direct_grant_groups g
    ON g.principal_type = op.principal_type
   AND g.principal_id = op.principal_id
   AND g.scope_type = 'ORGANIZATION'
   AND g.scope_id = op.org_id
   AND g.expires_at IS NOT DISTINCT FROM op.expires_at
  JOIN iam_roles r
    ON r.org_id = g.org_id AND r.name = 'legacy_dg_' || g.ghash
 WHERE op.principal_type IN ('USER', 'TEAM')
   AND (op.expires_at IS NULL OR op.expires_at > NOW())
   AND op.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
   AND pd.scope_level IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')
   AND NOT EXISTS (
     SELECT 1 FROM iam_role_policies rp
      WHERE rp.role_id = r.id
        AND rp.permission_id = op.permission_id
        AND rp.scope_type = 'ORGANIZATION'
   )
 GROUP BY r.id, op.permission_id`,
		`INSERT INTO iam_role_policies
  (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id,
       pp.permission_id,
       CASE MAX(CASE pp.permission_level::text
                    WHEN '3' THEN 3 WHEN 'ADMIN' THEN 3
                    WHEN '2' THEN 2 WHEN 'WRITE' THEN 2
                    WHEN '1' THEN 1 WHEN 'READ' THEN 1
                    ELSE 0 END)
         WHEN 3 THEN 'ADMIN' WHEN 2 THEN 'WRITE' ELSE 'READ' END,
       'PROJECT', NOW()
  FROM project_permissions pp
  JOIN projects p ON p.id = pp.project_id
  JOIN permission_definitions pd ON pd.id = pp.permission_id
  JOIN _iam_legacy_direct_grant_groups g
    ON g.principal_type = pp.principal_type
   AND g.principal_id = pp.principal_id
   AND g.scope_type = 'PROJECT'
   AND g.scope_id = pp.project_id
   AND g.org_id = p.org_id
   AND g.expires_at IS NOT DISTINCT FROM pp.expires_at
  JOIN iam_roles r
    ON r.org_id = g.org_id AND r.name = 'legacy_dg_' || g.ghash
 WHERE pp.principal_type IN ('USER', 'TEAM')
   AND (pp.expires_at IS NULL OR pp.expires_at > NOW())
   AND pp.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
   AND pd.scope_level IN ('PROJECT', 'WORKSPACE')
   AND NOT EXISTS (
     SELECT 1 FROM iam_role_policies rp
      WHERE rp.role_id = r.id
        AND rp.permission_id = pp.permission_id
        AND rp.scope_type = 'PROJECT'
   )
 GROUP BY r.id, pp.permission_id`,
		`INSERT INTO iam_role_policies
  (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id,
       wp.permission_id,
       CASE MAX(CASE wp.permission_level::text
                    WHEN '3' THEN 3 WHEN 'ADMIN' THEN 3
                    WHEN '2' THEN 2 WHEN 'WRITE' THEN 2
                    WHEN '1' THEN 1 WHEN 'READ' THEN 1
                    ELSE 0 END)
         WHEN 3 THEN 'ADMIN' WHEN 2 THEN 'WRITE' ELSE 'READ' END,
       'WORKSPACE', NOW()
  FROM workspace_permissions wp
  JOIN workspaces w ON w.workspace_id = wp.workspace_id
  JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
  JOIN projects p ON p.id = wpr.project_id
  JOIN permission_definitions pd ON pd.id = wp.permission_id
  JOIN _iam_legacy_direct_grant_groups g
    ON g.principal_type = wp.principal_type
   AND g.principal_id = wp.principal_id
   AND g.scope_type = 'WORKSPACE'
   AND g.scope_id = w.id
   AND g.org_id = p.org_id
   AND g.expires_at IS NOT DISTINCT FROM wp.expires_at
  JOIN iam_roles r
    ON r.org_id = g.org_id AND r.name = 'legacy_dg_' || g.ghash
 WHERE wp.principal_type IN ('USER', 'TEAM')
   AND (wp.expires_at IS NULL OR wp.expires_at > NOW())
   AND wp.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
   AND pd.scope_level = 'WORKSPACE'
   AND NOT EXISTS (
     SELECT 1 FROM iam_role_policies rp
      WHERE rp.role_id = r.id
        AND rp.permission_id = wp.permission_id
        AND rp.scope_type = 'WORKSPACE'
   )
 GROUP BY r.id, wp.permission_id`,
		// A USER assignment must refer to an existing user record. Invalid
		// orphan grants are retained in the source table and are not converted
		// into authority for a possibly reused identifier in the future.
		`INSERT INTO iam_user_roles
  (user_id, role_id, scope_type, scope_id, assigned_by, assigned_at, expires_at, reason)
SELECT g.principal_id, r.id, g.scope_type, g.scope_id,
       'system:legacy_dg', NOW(), g.expires_at,
       'IAM migration: synthesized from direct grant'
  FROM _iam_legacy_direct_grant_groups g
  JOIN users u ON u.user_id = g.principal_id
  JOIN iam_roles r ON r.org_id = g.org_id AND r.name = 'legacy_dg_' || g.ghash
 WHERE g.principal_type = 'USER'
   AND NOT EXISTS (
     SELECT 1 FROM iam_user_roles ur
      WHERE ur.user_id = g.principal_id
        AND ur.role_id = r.id
        AND ur.scope_type = g.scope_type
        AND ur.scope_id = g.scope_id
   )`,
		// Team assignments additionally prove that the team belongs to the same
		// tenant as the scoped resource. This closes a historic cross-tenant
		// principal-reference ambiguity instead of copying it into Roles.
		`INSERT INTO iam_team_roles
  (team_id, role_id, scope_type, scope_id, assigned_by, assigned_at, expires_at, reason)
SELECT g.principal_id, r.id, g.scope_type, g.scope_id,
       'system:legacy_dg', NOW(), g.expires_at,
       'IAM migration: synthesized from direct grant'
  FROM _iam_legacy_direct_grant_groups g
  JOIN teams t ON t.team_id = g.principal_id AND t.org_id = g.org_id
  JOIN iam_roles r ON r.org_id = g.org_id AND r.name = 'legacy_dg_' || g.ghash
 WHERE g.principal_type = 'TEAM'
   AND NOT EXISTS (
     SELECT 1 FROM iam_team_roles tr
      WHERE tr.team_id = g.principal_id
        AND tr.role_id = r.id
        AND tr.scope_type = g.scope_type
        AND tr.scope_id = g.scope_id
   )`,
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateLegacyApplicationDirectGrantsToRoles applies the same staged
// transition to APPLICATION grants. Application permissions are intentionally
// organization-scoped in the new model: project/workspace application grants
// were never evaluated by the Application principal path and are left as
// legacy evidence rather than being widened during migration.
func migrateLegacyApplicationDirectGrantsToRoles(ctx context.Context, tx *gorm.DB) error {
	if err := tx.WithContext(ctx).Exec(`
CREATE TEMP TABLE _iam_legacy_application_grant_groups ON COMMIT DROP AS
SELECT
  op.principal_id AS application_principal_id,
  op.org_id::bigint AS org_id,
  op.expires_at,
  md5('APPLICATION:' || op.principal_id || ':ORGANIZATION:' || op.org_id::text || ':' || COALESCE(op.expires_at::text, 'never')) AS ghash
FROM org_permissions op
JOIN applications a ON a.app_key = op.principal_id AND a.org_id = op.org_id
JOIN permission_definitions pd ON pd.id = op.permission_id
WHERE op.principal_type = 'APPLICATION'
  AND (op.expires_at IS NULL OR op.expires_at > NOW())
  AND op.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
  AND pd.scope_level IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')
GROUP BY op.principal_id, op.org_id, op.expires_at`).Error; err != nil {
		return err
	}

	for _, statement := range []string{
		`INSERT INTO iam_roles
  (org_id, name, display_name, description, is_system, is_active, created_at, updated_at)
SELECT DISTINCT g.org_id,
       'legacy_app_dg_' || g.ghash,
       'Legacy Application Direct Grant',
       'Auto-synthesized from application direct grants; principal=' || g.application_principal_id ||
         ' scope=ORGANIZATION/' || g.org_id::text,
       false, true, NOW(), NOW()
  FROM _iam_legacy_application_grant_groups g
 WHERE NOT EXISTS (
   SELECT 1 FROM iam_roles r
    WHERE r.org_id = g.org_id AND r.name = 'legacy_app_dg_' || g.ghash
 )`,
		`INSERT INTO iam_role_policies
  (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id,
       op.permission_id,
       CASE MAX(CASE op.permission_level::text
                    WHEN '3' THEN 3 WHEN 'ADMIN' THEN 3
                    WHEN '2' THEN 2 WHEN 'WRITE' THEN 2
                    WHEN '1' THEN 1 WHEN 'READ' THEN 1
                    ELSE 0 END)
         WHEN 3 THEN 'ADMIN' WHEN 2 THEN 'WRITE' ELSE 'READ' END,
       'ORGANIZATION', NOW()
  FROM org_permissions op
  JOIN applications a ON a.app_key = op.principal_id AND a.org_id = op.org_id
  JOIN permission_definitions pd ON pd.id = op.permission_id
  JOIN _iam_legacy_application_grant_groups g
    ON g.application_principal_id = op.principal_id
   AND g.org_id = op.org_id
   AND g.expires_at IS NOT DISTINCT FROM op.expires_at
  JOIN iam_roles r
    ON r.org_id = g.org_id AND r.name = 'legacy_app_dg_' || g.ghash
 WHERE op.principal_type = 'APPLICATION'
   AND (op.expires_at IS NULL OR op.expires_at > NOW())
   AND op.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
   AND pd.scope_level IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')
   AND NOT EXISTS (
     SELECT 1 FROM iam_role_policies rp
      WHERE rp.role_id = r.id
        AND rp.permission_id = op.permission_id
        AND rp.scope_type = 'ORGANIZATION'
   )
 GROUP BY r.id, op.permission_id`,
		`INSERT INTO iam_application_roles
  (application_principal_id, role_id, scope_type, scope_id, assigned_by, assigned_at, expires_at, reason)
SELECT g.application_principal_id, r.id, 'ORGANIZATION', g.org_id,
       'system:legacy_dg', NOW(), g.expires_at,
       'IAM migration: synthesized from application direct grant'
  FROM _iam_legacy_application_grant_groups g
  JOIN applications a ON a.app_key = g.application_principal_id AND a.org_id = g.org_id
  JOIN iam_roles r ON r.org_id = g.org_id AND r.name = 'legacy_app_dg_' || g.ghash
 WHERE NOT EXISTS (
   SELECT 1 FROM iam_application_roles ar
    WHERE ar.application_principal_id = g.application_principal_id
      AND ar.role_id = r.id
      AND ar.scope_type = 'ORGANIZATION'
      AND ar.scope_id = g.org_id
   )`,
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func repairUserOrganizationMemberships(ctx context.Context, tx *gorm.DB) error {
	// user_organizations is the source for the active-organization bootstrap.
	// Derive missing rows only from active, tenant-resolved authorization paths;
	// do not grant a membership merely because a malformed historical row names
	// an organization.
	return tx.WithContext(ctx).Exec(`
INSERT INTO user_organizations (user_id, org_id, joined_at)
SELECT DISTINCT candidates.user_id, candidates.org_id, NOW()
  FROM (
    SELECT ur.user_id, CASE ur.scope_type
      WHEN 'ORGANIZATION' THEN ur.scope_id::bigint
      WHEN 'PROJECT' THEN p.org_id::bigint
      WHEN 'WORKSPACE' THEN wsp.org_id::bigint
    END AS org_id
      FROM iam_user_roles ur
      JOIN iam_roles r ON r.id = ur.role_id AND r.is_active = true
      LEFT JOIN projects p ON ur.scope_type = 'PROJECT' AND p.id = ur.scope_id
      LEFT JOIN workspaces w ON ur.scope_type = 'WORKSPACE' AND w.id = ur.scope_id
      LEFT JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
      LEFT JOIN projects wsp ON wsp.id = wpr.project_id
     WHERE ur.expires_at IS NULL OR ur.expires_at > NOW()

    UNION

    SELECT op.principal_id, op.org_id::bigint
      FROM org_permissions op
      JOIN permission_definitions pd ON pd.id = op.permission_id
     WHERE op.principal_type = 'USER'
       AND (op.expires_at IS NULL OR op.expires_at > NOW())
       AND op.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
       AND pd.scope_level IN ('ORGANIZATION', 'PROJECT', 'WORKSPACE')

    UNION

    SELECT pp.principal_id, p.org_id::bigint
      FROM project_permissions pp
      JOIN projects p ON p.id = pp.project_id
      JOIN permission_definitions pd ON pd.id = pp.permission_id
     WHERE pp.principal_type = 'USER'
       AND (pp.expires_at IS NULL OR pp.expires_at > NOW())
       AND pp.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
       AND pd.scope_level IN ('PROJECT', 'WORKSPACE')

    UNION

    SELECT wp.principal_id, p.org_id::bigint
      FROM workspace_permissions wp
      JOIN workspaces w ON w.workspace_id = wp.workspace_id
      JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
      JOIN projects p ON p.id = wpr.project_id
      JOIN permission_definitions pd ON pd.id = wp.permission_id
     WHERE wp.principal_type = 'USER'
       AND (wp.expires_at IS NULL OR wp.expires_at > NOW())
       AND wp.permission_level::text IN ('1', '2', '3', 'READ', 'WRITE', 'ADMIN')
       AND pd.scope_level = 'WORKSPACE'

    UNION

    SELECT tm.user_id, t.org_id::bigint
      FROM team_members tm
      JOIN teams t ON t.team_id = tm.team_id
  ) candidates
  JOIN users u ON u.user_id = candidates.user_id
  JOIN organizations o ON o.id = candidates.org_id
 WHERE candidates.org_id > 0
   AND NOT EXISTS (
     SELECT 1 FROM user_organizations uo
      WHERE uo.user_id = candidates.user_id AND uo.org_id = candidates.org_id
	   )`).Error
}

func installIAMSupportingIndexes(ctx context.Context, tx *gorm.DB) error {
	var duplicateActiveTokenNames int64
	if err := tx.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM (
  SELECT team_id, token_name
    FROM team_tokens
   WHERE is_active = true
   GROUP BY team_id, token_name
  HAVING COUNT(*) > 1
) duplicates`).Scan(&duplicateActiveTokenNames).Error; err != nil {
		return err
	}
	if duplicateActiveTokenNames > 0 {
		return fmt.Errorf("found %d duplicate active team-token names; revoke or rename duplicates before migration", duplicateActiveTokenNames)
	}

	for _, statement := range []string{
		// Empty string never identifies a real user. Normalizing it before the
		// index prevents an accidental user_id match in temporary approvals.
		"UPDATE task_temporary_permissions SET user_id = NULL WHERE user_id = ''",
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_team_tokens_active_name
  ON team_tokens (team_id, token_name)
  WHERE is_active = true`,
		`CREATE INDEX IF NOT EXISTS idx_temp_perms_task_user_id
  ON task_temporary_permissions (task_id, user_id)
  WHERE user_id IS NOT NULL AND user_id <> ''`,
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func failOnDuplicateWorkspaceRelations(ctx context.Context, tx *gorm.DB) error {
	var duplicateCount int64
	err := tx.WithContext(ctx).Raw(`
SELECT COUNT(*) FROM (
  SELECT workspace_id
    FROM workspace_project_relations
   GROUP BY workspace_id
  HAVING COUNT(*) > 1
) duplicates`).Scan(&duplicateCount).Error
	if err != nil {
		return err
	}
	if duplicateCount > 0 {
		return fmt.Errorf("found %d workspaces attached to multiple projects; resolve ownership before migration", duplicateCount)
	}
	return nil
}

type roleOrgCandidate struct {
	RoleID uint
	OrgID  uint
}

const (
	iamRoleQuarantineTable       = "iam_role_migration_quarantine"
	iamRoleTenantConstraint      = "chk_iam_roles_custom_org"
	iamRoleTenantCheckExpression = "(is_system = true OR is_active = false OR org_id > 0)"
	iamRoleQuarantineReasonNil   = "no tenant ownership evidence"
	iamRoleQuarantineReasonMany  = "ambiguous tenant ownership"
	iamRoleQuarantineReasonName  = "resolved tenant ownership conflicts with another role name"
)

// reconcileIAMRoleTenantBoundary normalizes nullable legacy values, maps
// custom roles when their organization is provable, and quarantines the rest.
// A quarantined role is deliberately retained as inactive evidence at org 0;
// it is never reclassified as a globally visible system role.
func reconcileIAMRoleTenantBoundary(ctx context.Context, tx *gorm.DB) error {
	if err := tx.WithContext(ctx).Exec(`
CREATE TABLE IF NOT EXISTS ` + iamRoleQuarantineTable + ` (
  role_id BIGINT PRIMARY KEY,
  role_name VARCHAR(100) NOT NULL,
  candidate_org_ids TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL,
  quarantined_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`).Error; err != nil {
		return fmt.Errorf("create role quarantine audit table: %w", err)
	}

	// Replace the earlier check before changing nullable org_id values.  The
	// old check rejected every non-system org 0 role, including an inactive
	// quarantine record, and NULL values could bypass it altogether.
	if err := tx.WithContext(ctx).Exec("ALTER TABLE iam_roles DROP CONSTRAINT IF EXISTS " + iamRoleTenantConstraint).Error; err != nil {
		return fmt.Errorf("drop legacy role tenant constraint: %w", err)
	}
	for _, statement := range []string{
		"UPDATE iam_roles SET is_system = false WHERE is_system IS NULL",
		// A missing activation flag must fail closed.  It remains inactive even
		// if the role can otherwise be mapped to one tenant.
		"UPDATE iam_roles SET is_active = false WHERE is_active IS NULL",
		// Do not collapse a nullable custom org_id to 0 in one bulk update: a
		// legacy per-org unique index may contain several NULL-name pairs. The
		// per-role reconciliation below resolves or quarantines them safely.
		"UPDATE iam_roles SET org_id = 0 WHERE is_system = true AND org_id IS DISTINCT FROM 0",
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("normalize legacy IAM role fields: %w", err)
		}
	}

	if err := normalizeLegacyRoleOwnership(ctx, tx); err != nil {
		return err
	}

	for _, statement := range []string{
		"ALTER TABLE iam_roles ALTER COLUMN org_id SET DEFAULT 0",
		"ALTER TABLE iam_roles ALTER COLUMN org_id SET NOT NULL",
		"ALTER TABLE iam_roles ALTER COLUMN is_system SET DEFAULT false",
		"ALTER TABLE iam_roles ALTER COLUMN is_system SET NOT NULL",
		"ALTER TABLE iam_roles ALTER COLUMN is_active SET DEFAULT true",
		"ALTER TABLE iam_roles ALTER COLUMN is_active SET NOT NULL",
		"ALTER TABLE iam_roles ADD CONSTRAINT " + iamRoleTenantConstraint + " CHECK " + iamRoleTenantCheckExpression,
	} {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("enforce IAM role tenant boundary: %w", err)
		}
	}
	return nil
}

func normalizeLegacyRoleOwnership(ctx context.Context, tx *gorm.DB) error {
	var candidates []roleOrgCandidate
	err := tx.WithContext(ctx).Raw(`
SELECT DISTINCT role_id, org_id FROM (
  SELECT role_id, scope_id::bigint AS org_id
    FROM iam_user_roles WHERE scope_type = 'ORGANIZATION'
  UNION
  SELECT ur.role_id, p.org_id::bigint
    FROM iam_user_roles ur JOIN projects p ON ur.scope_type = 'PROJECT' AND ur.scope_id = p.id
  UNION
  SELECT ur.role_id, p.org_id::bigint
    FROM iam_user_roles ur
    JOIN workspaces w ON ur.scope_type = 'WORKSPACE' AND ur.scope_id = w.id
    JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
    JOIN projects p ON p.id = wpr.project_id
  UNION
  SELECT role_id, scope_id::bigint AS org_id
    FROM iam_team_roles WHERE scope_type = 'ORGANIZATION'
  UNION
  SELECT tr.role_id, p.org_id::bigint
    FROM iam_team_roles tr JOIN projects p ON tr.scope_type = 'PROJECT' AND tr.scope_id = p.id
  UNION
  SELECT tr.role_id, p.org_id::bigint
    FROM iam_team_roles tr
    JOIN workspaces w ON tr.scope_type = 'WORKSPACE' AND tr.scope_id = w.id
    JOIN workspace_project_relations wpr ON wpr.workspace_id = w.workspace_id
    JOIN projects p ON p.id = wpr.project_id
) scoped_roles
WHERE org_id > 0`).Scan(&candidates).Error
	if err != nil {
		return err
	}

	byRole := make(map[uint]map[uint]struct{})
	for _, c := range candidates {
		if byRole[c.RoleID] == nil {
			byRole[c.RoleID] = make(map[uint]struct{})
		}
		byRole[c.RoleID][c.OrgID] = struct{}{}
	}

	type legacyRole struct {
		ID        uint
		Name      string
		CreatedBy *string
	}
	var roles []legacyRole
	if err := tx.WithContext(ctx).Table("iam_roles").
		Select("id, name, created_by").
		Where(`is_system IS NOT TRUE
  AND (org_id IS NULL OR org_id <= 0)
  AND NOT EXISTS (
    SELECT 1 FROM ` + iamRoleQuarantineTable + ` q WHERE q.role_id = iam_roles.id
  )`).Find(&roles).Error; err != nil {
		return err
	}

	for _, role := range roles {
		orgs := byRole[role.ID]
		if len(orgs) == 0 && role.CreatedBy != nil && *role.CreatedBy != "" {
			var creatorOrgs []uint
			if err := tx.WithContext(ctx).Table("user_organizations").
				Where("user_id = ?", *role.CreatedBy).Pluck("org_id", &creatorOrgs).Error; err != nil {
				return err
			}
			for _, orgID := range creatorOrgs {
				if orgID > 0 {
					if orgs == nil {
						orgs = make(map[uint]struct{})
					}
					orgs[orgID] = struct{}{}
				}
			}
		}
		if len(orgs) != 1 {
			reason := iamRoleQuarantineReasonNil
			if len(orgs) > 1 {
				reason = iamRoleQuarantineReasonMany
			}
			if err := quarantineLegacyRole(ctx, tx, role.ID, role.Name, orgs, reason); err != nil {
				return err
			}
			continue
		}
		var orgID uint
		for orgID = range orgs {
		}

		var nameConflictCount int64
		if err := tx.WithContext(ctx).Table("iam_roles").
			Where("id <> ? AND org_id = ? AND name = ?", role.ID, orgID, role.Name).
			Count(&nameConflictCount).Error; err != nil {
			return err
		}
		if nameConflictCount > 0 {
			if err := quarantineLegacyRole(ctx, tx, role.ID, role.Name, orgs, iamRoleQuarantineReasonName); err != nil {
				return err
			}
			continue
		}

		if err := tx.WithContext(ctx).Table("iam_roles").Where("id = ?", role.ID).
			Updates(map[string]interface{}{"org_id": orgID, "is_system": false}).Error; err != nil {
			return err
		}
	}
	return nil
}

func quarantineLegacyRole(ctx context.Context, tx *gorm.DB, roleID uint, roleName string, orgs map[uint]struct{}, reason string) error {
	ids := make([]string, 0, len(orgs))
	for orgID := range orgs {
		ids = append(ids, fmt.Sprint(orgID))
	}
	sort.Strings(ids)

	// Preserve the original role name whenever it is safe.  A quarantine row
	// shares org 0 with system Roles, however, so a collision (or a future
	// built-in name) must be moved aside before the per-org unique index and
	// subsequent built-in-role migrations run.  The audit row always keeps the
	// original name for operator recovery.
	storedName := roleName
	var orgZeroNameConflicts int64
	if err := tx.WithContext(ctx).Table("iam_roles").
		Where("id <> ? AND org_id = 0 AND name = ?", roleID, roleName).
		Count(&orgZeroNameConflicts).Error; err != nil {
		return fmt.Errorf("check quarantine role name %d: %w", roleID, err)
	}
	if orgZeroNameConflicts > 0 || isReservedSystemRoleName(roleName) {
		storedName = fmt.Sprintf("quarantined_role_%d", roleID)
	}

	if err := tx.WithContext(ctx).Table("iam_roles").Where("id = ?", roleID).
		Updates(map[string]interface{}{
			"org_id":    0,
			"is_system": false,
			"is_active": false,
			"name":      storedName,
		}).Error; err != nil {
		return fmt.Errorf("quarantine legacy role %d: %w", roleID, err)
	}
	if err := tx.WithContext(ctx).Exec(`
INSERT INTO `+iamRoleQuarantineTable+`
  (role_id, role_name, candidate_org_ids, reason, quarantined_at)
VALUES (?, ?, ?, ?, NOW())
ON CONFLICT (role_id) DO UPDATE
   SET role_name = EXCLUDED.role_name,
       candidate_org_ids = EXCLUDED.candidate_org_ids,
       reason = EXCLUDED.reason,
       quarantined_at = EXCLUDED.quarantined_at`,
		roleID, roleName, strings.Join(ids, ","), reason).Error; err != nil {
		return fmt.Errorf("audit quarantined legacy role %d: %w", roleID, err)
	}
	return nil
}

func isReservedSystemRoleName(name string) bool {
	switch name {
	case "admin", "workspace_admin":
		return true
	default:
		return false
	}
}
