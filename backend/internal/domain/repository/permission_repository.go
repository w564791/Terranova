package repository

import (
	"context"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// PermissionRepository 权限仓储接口
type PermissionRepository interface {
	// QueryOrgPermissions 查询组织级权限
	QueryOrgPermissions(
		ctx context.Context,
		orgID uint,
		principalType valueobject.PrincipalType,
		principalIDs []string,
		resourceType valueobject.ResourceType,
	) ([]*entity.OrgPermission, error)

	// QueryProjectPermissions 查询项目级权限
	QueryProjectPermissions(
		ctx context.Context,
		projectID uint,
		principalType valueobject.PrincipalType,
		principalIDs []string,
		resourceType valueobject.ResourceType,
	) ([]*entity.ProjectPermission, error)

	// QueryWorkspacePermissions 查询工作空间级权限
	QueryWorkspacePermissions(
		ctx context.Context,
		workspaceID string,
		principalType valueobject.PrincipalType,
		principalIDs []string,
		resourceType valueobject.ResourceType,
	) ([]*entity.WorkspacePermission, error)

	// GrantOrgPermission 授予组织级权限
	GrantOrgPermission(ctx context.Context, permission *entity.OrgPermission) error

	// GrantProjectPermission 授予项目级权限
	GrantProjectPermission(ctx context.Context, permission *entity.ProjectPermission) error

	// GrantWorkspacePermission 授予工作空间级权限
	GrantWorkspacePermission(ctx context.Context, permission *entity.WorkspacePermission) error

	// RevokeOrgPermission 撤销组织级权限
	RevokeOrgPermission(ctx context.Context, id uint) error

	// RevokeProjectPermission 撤销项目级权限
	RevokeProjectPermission(ctx context.Context, id uint) error

	// RevokeWorkspacePermission 撤销工作空间级权限
	RevokeWorkspacePermission(ctx context.Context, id uint) error

	// UpdateOrgPermission 更新组织级权限
	UpdateOrgPermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error

	// UpdateProjectPermission 更新项目级权限
	UpdateProjectPermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error

	// UpdateWorkspacePermission 更新工作空间级权限
	UpdateWorkspacePermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error

	// ListPermissionsByScope 列出指定作用域的所有权限分配
	ListPermissionsByScope(
		ctx context.Context,
		scopeType valueobject.ScopeType,
		scopeID uint,
	) ([]*entity.PermissionGrant, error)

	// GetPermissionDefinitionByName 根据名称获取权限定义
	GetPermissionDefinitionByName(ctx context.Context, name string) (*entity.PermissionDefinition, error)

	// ListPermissionDefinitions 列出所有权限定义
	ListPermissionDefinitions(ctx context.Context) ([]*entity.PermissionDefinition, error)

	// GetPresetPermissions 获取预设权限集包含的权限列表
	GetPresetPermissions(
		ctx context.Context,
		presetName string,
		scopeLevel valueobject.ScopeType,
	) ([]*entity.PresetPermission, error)

	// CheckTemporaryPermission 检查临时权限
	CheckTemporaryPermission(
		ctx context.Context,
		taskID uint,
		userEmail string,
		permissionType string,
	) (*entity.TaskTemporaryPermission, error)

	// CreateTemporaryPermission 创建临时权限
	CreateTemporaryPermission(ctx context.Context, permission *entity.TaskTemporaryPermission) error

	// MarkTemporaryPermissionUsed 标记临时权限为已使用
	MarkTemporaryPermissionUsed(ctx context.Context, id uint) error

	// QueryUserRoles 查询用户的角色分配
	QueryUserRoles(
		ctx context.Context,
		userID string,
		scopeType valueobject.ScopeType,
		scopeID uint,
	) ([]*entity.UserRole, error)

	// QueryTeamRoles 查询团队的角色分配
	QueryTeamRoles(
		ctx context.Context,
		teamIDs []string,
		scopeType valueobject.ScopeType,
		scopeID uint,
	) ([]*entity.UserRole, error)

	// QueryRolePolicies 查询角色包含的权限策略
	QueryRolePolicies(
		ctx context.Context,
		roleID uint,
		scopeType valueobject.ScopeType,
	) ([]*entity.RolePolicy, error)

	// GetPermissionDefinition 获取权限定义
	GetPermissionDefinition(ctx context.Context, permissionID uint) (*entity.PermissionDefinition, error)

	// ListPermissionsByPrincipal 列出指定主体的所有权限（跨所有作用域）
	ListPermissionsByPrincipal(
		ctx context.Context,
		principalType valueobject.PrincipalType,
		principalID string,
	) ([]*entity.PermissionGrant, error)
}
