package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/domain/valueobject"
)

// PermissionRepositoryImpl 权限仓储GORM实现
type PermissionRepositoryImpl struct {
	db *gorm.DB
}

// NewPermissionRepository 创建权限仓储实例
func NewPermissionRepository(db *gorm.DB) repository.PermissionRepository {
	return &PermissionRepositoryImpl{db: db}
}

// QueryOrgPermissions 查询组织级权限
func (r *PermissionRepositoryImpl) QueryOrgPermissions(
	ctx context.Context,
	orgID uint,
	principalType valueobject.PrincipalType,
	principalIDs []string,
	resourceType valueobject.ResourceType,
) ([]*entity.OrgPermission, error) {
	var permissions []*entity.OrgPermission

	query := r.db.WithContext(ctx).
		Preload("Permission").
		Where("org_id = ? AND principal_type = ?", orgID, principalType)

	if len(principalIDs) > 0 {
		query = query.Where("principal_id IN ?", principalIDs)
	}

	// 如果指定了资源类型，通过join过滤
	if resourceType != "" {
		query = query.Joins("JOIN permission_definitions ON permission_definitions.id = org_permissions.permission_id").
			Where("permission_definitions.resource_type = ?", resourceType)
	}

	if err := query.Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("failed to query org permissions: %w", err)
	}

	return permissions, nil
}

// QueryProjectPermissions 查询项目级权限
func (r *PermissionRepositoryImpl) QueryProjectPermissions(
	ctx context.Context,
	projectID uint,
	principalType valueobject.PrincipalType,
	principalIDs []string,
	resourceType valueobject.ResourceType,
) ([]*entity.ProjectPermission, error) {
	var permissions []*entity.ProjectPermission

	query := r.db.WithContext(ctx).
		Preload("Permission").
		Where("project_id = ? AND principal_type = ?", projectID, principalType)

	if len(principalIDs) > 0 {
		query = query.Where("principal_id IN ?", principalIDs)
	}

	if resourceType != "" {
		query = query.Joins("JOIN permission_definitions ON permission_definitions.id = project_permissions.permission_id").
			Where("permission_definitions.resource_type = ?", resourceType)
	}

	if err := query.Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("failed to query project permissions: %w", err)
	}

	return permissions, nil
}

// QueryWorkspacePermissions 查询工作空间级权限
func (r *PermissionRepositoryImpl) QueryWorkspacePermissions(
	ctx context.Context,
	workspaceID string,
	principalType valueobject.PrincipalType,
	principalIDs []string,
	resourceType valueobject.ResourceType,
) ([]*entity.WorkspacePermission, error) {
	var permissions []*entity.WorkspacePermission

	query := r.db.WithContext(ctx).
		Preload("Permission").
		Where("workspace_id = ? AND principal_type = ?", workspaceID, principalType)

	if len(principalIDs) > 0 {
		query = query.Where("principal_id IN ?", principalIDs)
	}

	if resourceType != "" {
		query = query.Joins("JOIN permission_definitions ON permission_definitions.id = workspace_permissions.permission_id").
			Where("permission_definitions.resource_type = ?", resourceType)
	}

	if err := query.Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("failed to query workspace permissions: %w", err)
	}

	return permissions, nil
}

// GrantOrgPermission 授予组织级权限
func (r *PermissionRepositoryImpl) GrantOrgPermission(ctx context.Context, permission *entity.OrgPermission) error {
	// 检查是否已存在相同的权限授予
	var existing entity.OrgPermission
	err := r.db.WithContext(ctx).
		Where("org_id = ? AND principal_type = ? AND principal_id = ? AND permission_id = ?",
			permission.OrgID, permission.PrincipalType, permission.PrincipalID, permission.PermissionID).
		First(&existing).Error

	if err == nil {
		// 已存在权限,返回冲突错误
		return fmt.Errorf("permission already exists: principal %s already has permission %s (level: %s) on org %d",
			permission.PrincipalID, permission.PermissionID, existing.PermissionLevel, permission.OrgID)
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing org permission: %w", err)
	}

	// 不存在,创建新权限
	if err := r.db.WithContext(ctx).Create(permission).Error; err != nil {
		return fmt.Errorf("failed to grant org permission: %w", err)
	}
	return nil
}

// GrantProjectPermission 授予项目级权限
func (r *PermissionRepositoryImpl) GrantProjectPermission(ctx context.Context, permission *entity.ProjectPermission) error {
	// 检查是否已存在相同的权限授予
	var existing entity.ProjectPermission
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND principal_type = ? AND principal_id = ? AND permission_id = ?",
			permission.ProjectID, permission.PrincipalType, permission.PrincipalID, permission.PermissionID).
		First(&existing).Error

	if err == nil {
		// 已存在权限,返回冲突错误
		return fmt.Errorf("permission already exists: principal %s already has permission %s (level: %s) on project %d",
			permission.PrincipalID, permission.PermissionID, existing.PermissionLevel, permission.ProjectID)
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing project permission: %w", err)
	}

	// 不存在,创建新权限
	if err := r.db.WithContext(ctx).Create(permission).Error; err != nil {
		return fmt.Errorf("failed to grant project permission: %w", err)
	}
	return nil
}

// GrantWorkspacePermission 授予工作空间级权限
func (r *PermissionRepositoryImpl) GrantWorkspacePermission(ctx context.Context, permission *entity.WorkspacePermission) error {
	// 检查是否已存在相同的权限授予
	var existing entity.WorkspacePermission
	err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND principal_type = ? AND principal_id = ? AND permission_id = ?",
			permission.WorkspaceID, permission.PrincipalType, permission.PrincipalID, permission.PermissionID).
		First(&existing).Error

	if err == nil {
		// 已存在权限,返回冲突错误
		return fmt.Errorf("permission already exists: principal %s already has permission %s (level: %s) on workspace %s",
			permission.PrincipalID, permission.PermissionID, existing.PermissionLevel, permission.WorkspaceID)
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check existing workspace permission: %w", err)
	}

	// 不存在,创建新权限
	if err := r.db.WithContext(ctx).Create(permission).Error; err != nil {
		return fmt.Errorf("failed to grant workspace permission: %w", err)
	}
	return nil
}

// RevokeOrgPermission 撤销组织级权限
func (r *PermissionRepositoryImpl) RevokeOrgPermission(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&entity.OrgPermission{}, id).Error; err != nil {
		return fmt.Errorf("failed to revoke org permission: %w", err)
	}
	return nil
}

// RevokeProjectPermission 撤销项目级权限
func (r *PermissionRepositoryImpl) RevokeProjectPermission(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&entity.ProjectPermission{}, id).Error; err != nil {
		return fmt.Errorf("failed to revoke project permission: %w", err)
	}
	return nil
}

// RevokeWorkspacePermission 撤销工作空间级权限
func (r *PermissionRepositoryImpl) RevokeWorkspacePermission(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&entity.WorkspacePermission{}, id).Error; err != nil {
		return fmt.Errorf("failed to revoke workspace permission: %w", err)
	}
	return nil
}

// UpdateOrgPermission 更新组织级权限
func (r *PermissionRepositoryImpl) UpdateOrgPermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error {
	if err := r.db.WithContext(ctx).Model(&entity.OrgPermission{}).
		Where("id = ?", id).
		Update("permission_level", level).Error; err != nil {
		return fmt.Errorf("failed to update org permission: %w", err)
	}
	return nil
}

// UpdateProjectPermission 更新项目级权限
func (r *PermissionRepositoryImpl) UpdateProjectPermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error {
	if err := r.db.WithContext(ctx).Model(&entity.ProjectPermission{}).
		Where("id = ?", id).
		Update("permission_level", level).Error; err != nil {
		return fmt.Errorf("failed to update project permission: %w", err)
	}
	return nil
}

// UpdateWorkspacePermission 更新工作空间级权限
func (r *PermissionRepositoryImpl) UpdateWorkspacePermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error {
	if err := r.db.WithContext(ctx).Model(&entity.WorkspacePermission{}).
		Where("id = ?", id).
		Update("permission_level", level).Error; err != nil {
		return fmt.Errorf("failed to update workspace permission: %w", err)
	}
	return nil
}

// ListPermissionsByScope 列出指定作用域的所有权限分配
func (r *PermissionRepositoryImpl) ListPermissionsByScope(
	ctx context.Context,
	scopeType valueobject.ScopeType,
	scopeID uint,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	switch scopeType {
	case valueobject.ScopeTypeOrganization:
		var orgPerms []*entity.OrgPermission
		if err := r.db.WithContext(ctx).
			Preload("Permission").
			Where("org_id = ?", scopeID).
			Find(&orgPerms).Error; err != nil {
			return nil, err
		}
		for _, perm := range orgPerms {
			grants = append(grants, perm.ToPermissionGrant())
		}

	case valueobject.ScopeTypeProject:
		var projPerms []*entity.ProjectPermission
		if err := r.db.WithContext(ctx).
			Preload("Permission").
			Where("project_id = ?", scopeID).
			Find(&projPerms).Error; err != nil {
			return nil, err
		}
		for _, perm := range projPerms {
			grants = append(grants, perm.ToPermissionGrant())
		}

	case valueobject.ScopeTypeWorkspace:
		var wsPerms []*entity.WorkspacePermission
		if err := r.db.WithContext(ctx).
			Preload("Permission").
			Where("workspace_id = ?", scopeID).
			Find(&wsPerms).Error; err != nil {
			return nil, err
		}
		for _, perm := range wsPerms {
			grants = append(grants, perm.ToPermissionGrant())
		}
	}

	return grants, nil
}

// GetPermissionDefinitionByName 根据ID或名称获取权限定义
func (r *PermissionRepositoryImpl) GetPermissionDefinitionByName(ctx context.Context, nameOrID string) (*entity.PermissionDefinition, error) {
	var permission entity.PermissionDefinition
	// 先尝试用id查询（语义ID如wspm-000000000024）
	if err := r.db.WithContext(ctx).Where("id = ?", nameOrID).First(&permission).Error; err == nil {
		return &permission, nil
	}
	// 如果id查询失败，再尝试用name查询
	if err := r.db.WithContext(ctx).Where("name = ?", nameOrID).First(&permission).Error; err != nil {
		return nil, fmt.Errorf("permission definition not found: %w", err)
	}
	return &permission, nil
}

// ListPermissionDefinitions 列出所有权限定义
func (r *PermissionRepositoryImpl) ListPermissionDefinitions(ctx context.Context) ([]*entity.PermissionDefinition, error) {
	var permissions []*entity.PermissionDefinition
	if err := r.db.WithContext(ctx).Find(&permissions).Error; err != nil {
		return nil, fmt.Errorf("failed to list permission definitions: %w", err)
	}
	return permissions, nil
}

// GetPresetPermissions 获取预设权限集包含的权限列表
func (r *PermissionRepositoryImpl) GetPresetPermissions(
	ctx context.Context,
	presetName string,
	scopeLevel valueobject.ScopeType,
) ([]*entity.PresetPermission, error) {
	var presetPerms []*entity.PresetPermission

	if err := r.db.WithContext(ctx).
		Preload("Permission").
		Joins("JOIN permission_presets ON permission_presets.id = preset_permissions.preset_id").
		Where("permission_presets.name = ? AND permission_presets.scope_level = ?", presetName, scopeLevel).
		Find(&presetPerms).Error; err != nil {
		return nil, fmt.Errorf("failed to get preset permissions: %w", err)
	}

	return presetPerms, nil
}

// CheckTemporaryPermission 检查临时权限
func (r *PermissionRepositoryImpl) CheckTemporaryPermission(
	ctx context.Context,
	taskID uint,
	userEmail string,
	permissionType string,
) (*entity.TaskTemporaryPermission, error) {
	var tempPerm entity.TaskTemporaryPermission

	err := r.db.WithContext(ctx).
		Where("task_id = ? AND user_email = ? AND permission_type = ? AND expires_at > ? AND is_used = ?",
			taskID, userEmail, permissionType, gorm.Expr("NOW()"), false).
		First(&tempPerm).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to check temporary permission: %w", err)
	}

	return &tempPerm, nil
}

// CreateTemporaryPermission 创建临时权限
func (r *PermissionRepositoryImpl) CreateTemporaryPermission(ctx context.Context, permission *entity.TaskTemporaryPermission) error {
	if err := r.db.WithContext(ctx).Create(permission).Error; err != nil {
		return fmt.Errorf("failed to create temporary permission: %w", err)
	}
	return nil
}

// MarkTemporaryPermissionUsed 标记临时权限为已使用
func (r *PermissionRepositoryImpl) MarkTemporaryPermissionUsed(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Model(&entity.TaskTemporaryPermission{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"is_used": true,
			"used_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
		return fmt.Errorf("failed to mark temporary permission as used: %w", err)
	}
	return nil
}

// QueryUserRoles 查询用户的角色分配
func (r *PermissionRepositoryImpl) QueryUserRoles(
	ctx context.Context,
	userID string,
	scopeType valueobject.ScopeType,
	scopeID uint,
) ([]*entity.UserRole, error) {
	var roles []*entity.UserRole

	err := r.db.WithContext(ctx).
		Table("iam_user_roles").
		Select("iam_user_roles.*, iam_roles.name as role_name, iam_roles.display_name as role_display_name").
		Joins("JOIN iam_roles ON iam_roles.id = iam_user_roles.role_id").
		Where("iam_user_roles.user_id = ? AND iam_user_roles.scope_type = ? AND iam_user_roles.scope_id = ?", userID, string(scopeType), scopeID).
		Where("iam_roles.is_active = ?", true).
		Where("iam_user_roles.expires_at IS NULL OR iam_user_roles.expires_at > ?", gorm.Expr("NOW()")).
		Find(&roles).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query user roles: %w", err)
	}

	return roles, nil
}

// QueryTeamRoles 查询团队的角色分配
func (r *PermissionRepositoryImpl) QueryTeamRoles(
	ctx context.Context,
	teamIDs []string,
	scopeType valueobject.ScopeType,
	scopeID uint,
) ([]*entity.UserRole, error) {
	var roles []*entity.UserRole

	if len(teamIDs) == 0 {
		return roles, nil
	}

	err := r.db.WithContext(ctx).
		Table("iam_team_roles").
		Select("iam_team_roles.id, iam_team_roles.role_id, iam_team_roles.scope_type, iam_team_roles.scope_id, iam_team_roles.assigned_at, iam_team_roles.expires_at, iam_roles.name as role_name, iam_roles.display_name as role_display_name").
		Joins("JOIN iam_roles ON iam_roles.id = iam_team_roles.role_id").
		Where("iam_team_roles.team_id IN ? AND iam_team_roles.scope_type = ? AND iam_team_roles.scope_id = ?", teamIDs, string(scopeType), scopeID).
		Where("iam_roles.is_active = ?", true).
		Where("iam_team_roles.expires_at IS NULL OR iam_team_roles.expires_at > ?", gorm.Expr("NOW()")).
		Find(&roles).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query team roles: %w", err)
	}

	return roles, nil
}

// QueryRolePolicies 查询角色包含的权限策略
func (r *PermissionRepositoryImpl) QueryRolePolicies(
	ctx context.Context,
	roleID uint,
	scopeType valueobject.ScopeType,
) ([]*entity.RolePolicy, error) {
	var policies []*entity.RolePolicy

	// 先查询角色策略
	err := r.db.WithContext(ctx).
		Where("role_id = ? AND scope_type = ?", roleID, string(scopeType)).
		Find(&policies).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query role policies: %w", err)
	}

	// 手动加载权限定义信息
	for _, policy := range policies {
		var permDef entity.PermissionDefinition
		if err := r.db.WithContext(ctx).Where("id = ?", policy.PermissionID).First(&permDef).Error; err == nil {
			policy.PermissionName = permDef.Name
			policy.PermissionDisplayName = permDef.DisplayName
			policy.ResourceType = string(permDef.ResourceType)
		}
	}

	return policies, nil
}

// GetPermissionDefinition 获取权限定义
func (r *PermissionRepositoryImpl) GetPermissionDefinition(ctx context.Context, permissionID uint) (*entity.PermissionDefinition, error) {
	var permission entity.PermissionDefinition
	if err := r.db.WithContext(ctx).First(&permission, permissionID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("permission definition not found: %d", permissionID)
		}
		return nil, fmt.Errorf("failed to get permission definition: %w", err)
	}
	return &permission, nil
}

// ListPermissionsByPrincipal 列出指定主体的所有权限（跨所有作用域）
func (r *PermissionRepositoryImpl) ListPermissionsByPrincipal(
	ctx context.Context,
	principalType valueobject.PrincipalType,
	principalID string,
) ([]*entity.PermissionGrant, error) {
	var grants []*entity.PermissionGrant

	// 查询组织级权限
	var orgPerms []*entity.OrgPermission
	if err := r.db.WithContext(ctx).
		Preload("Permission").
		Where("principal_type = ? AND principal_id = ?", principalType, principalID).
		Find(&orgPerms).Error; err != nil {
		return nil, fmt.Errorf("failed to query org permissions: %w", err)
	}
	for _, perm := range orgPerms {
		grants = append(grants, perm.ToPermissionGrant())
	}

	// 查询项目级权限
	var projPerms []*entity.ProjectPermission
	if err := r.db.WithContext(ctx).
		Preload("Permission").
		Where("principal_type = ? AND principal_id = ?", principalType, principalID).
		Find(&projPerms).Error; err != nil {
		return nil, fmt.Errorf("failed to query project permissions: %w", err)
	}
	for _, perm := range projPerms {
		grants = append(grants, perm.ToPermissionGrant())
	}

	// 查询工作空间级权限
	var wsPerms []*entity.WorkspacePermission
	if err := r.db.WithContext(ctx).
		Preload("Permission").
		Where("principal_type = ? AND principal_id = ?", principalType, principalID).
		Find(&wsPerms).Error; err != nil {
		return nil, fmt.Errorf("failed to query workspace permissions: %w", err)
	}
	for _, perm := range wsPerms {
		grant := perm.ToPermissionGrant()
		// 查询workspace的数字ID，同时保留语义化ID
		var workspace struct {
			ID uint `gorm:"column:id"`
		}
		if err := r.db.WithContext(ctx).Table("workspaces").
			Select("id").
			Where("workspace_id = ?", perm.WorkspaceID).
			First(&workspace).Error; err == nil {
			grant.ScopeID = workspace.ID
		}
		// 保留语义化ID供前端使用
		grant.ScopeIDStr = perm.WorkspaceID
		grants = append(grants, grant)
	}

	return grants, nil
}
