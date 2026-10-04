package migration

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// orgCatalogPermission is an organization-scoped permission definition plus
// the built-in Role policies that must accompany it. Without Role policies a
// newly registered resource type is unreachable for every principal (the
// business API has no system-admin bypass), so both are installed together.
type orgCatalogPermission struct {
	id          string
	resource    string
	displayName string
	description string
	// builtin system Role name (org_id = 0) -> permission level @ ORGANIZATION
	grants []builtinRoleGrant
}

type builtinRoleGrant struct {
	role  string
	level string
}

// manifestIAMPermissions is the data contract of migration
// 20261004_01_manifest_iam_resource. It mirrors existing catalog resources:
//   - MANIFESTS mirrors MODULES (admin/org_admin ADMIN; developer, viewer and
//     the default user Role READ).
//   - VARIABLE_SETS was referenced by routes but never registered in
//     permission_definitions; register it mirroring RUN_TASKS (admin/org_admin
//     ADMIN, viewer READ) so manifest deploy's varset step is reachable.
//
// Keep backend/migrations/add_manifest_iam_permissions.sql and the seed block
// in manifests/db/init_seed_data.sql in sync with this list.
func manifestIAMPermissions() []orgCatalogPermission {
	return []orgCatalogPermission{
		{
			id: "orgpm-manifests", resource: "MANIFESTS",
			displayName: "Manifest 管理",
			description: "管理组织下的 Manifest 目录（查看、编辑/发布、删除/归档）；部署另需目标 workspace 的 WORKSPACE_RESOURCES WRITE",
			grants: []builtinRoleGrant{
				{"admin", "ADMIN"}, {"org_admin", "ADMIN"},
				{"developer", "READ"}, {"viewer", "READ"}, {"user", "READ"},
			},
		},
		{
			id: "orgpm-variable-sets", resource: "VARIABLE_SETS",
			displayName: "变量集管理",
			description: "管理组织变量集（查看、编辑、删除、分配）",
			grants: []builtinRoleGrant{
				{"admin", "ADMIN"}, {"org_admin", "ADMIN"}, {"viewer", "READ"},
			},
		},
	}
}

const insertOrgPermissionDefinitionSQL = `INSERT INTO permission_definitions
  (id, name, resource_type, scope_level, display_name, description, is_system, created_at)
SELECT ?, ?, ?, 'ORGANIZATION', ?, ?, true, NOW()
 WHERE NOT EXISTS (
   SELECT 1 FROM permission_definitions
    WHERE id = ? OR name = ? OR (resource_type = ? AND scope_level = 'ORGANIZATION')
 )`

const insertBuiltinRoleOrgPolicySQL = `INSERT INTO iam_role_policies
  (role_id, permission_id, permission_level, scope_type, created_at)
SELECT r.id, pd.id, ?, 'ORGANIZATION', NOW()
  FROM iam_roles r
  JOIN permission_definitions pd
    ON pd.resource_type = ? AND pd.scope_level = 'ORGANIZATION'
 WHERE r.name = ?
   AND r.is_system = true
   AND r.org_id = 0
   AND NOT EXISTS (
     SELECT 1 FROM iam_role_policies rp
      WHERE rp.role_id = r.id
        AND rp.permission_id = pd.id
        AND rp.scope_type = 'ORGANIZATION'
   )`

// applyManifestIAMResource is additive and idempotent: it only inserts rows
// that do not exist and never modifies or removes existing grants.
func applyManifestIAMResource(ctx context.Context, tx *gorm.DB) error {
	for _, p := range manifestIAMPermissions() {
		if err := tx.WithContext(ctx).Exec(insertOrgPermissionDefinitionSQL,
			p.id, p.resource, p.resource, p.displayName, p.description,
			p.id, p.resource, p.resource,
		).Error; err != nil {
			return fmt.Errorf("register permission definition %s: %w", p.resource, err)
		}
		for _, g := range p.grants {
			if err := tx.WithContext(ctx).Exec(insertBuiltinRoleOrgPolicySQL,
				g.level, p.resource, g.role,
			).Error; err != nil {
				return fmt.Errorf("grant %s %s to builtin role %s: %w", p.resource, g.level, g.role, err)
			}
		}
	}
	return nil
}
