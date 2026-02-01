package repository

import (
	"context"

	"iac-platform/internal/domain/entity"

	"gorm.io/gorm"
)

// OrganizationRepository 组织仓储接口
type OrganizationRepository interface {
	// CreateOrganization 创建组织
	CreateOrganization(ctx context.Context, org *entity.Organization) error

	// GetOrganizationByID 根据ID获取组织
	GetOrganizationByID(ctx context.Context, id uint) (*entity.Organization, error)

	// GetOrganizationByName 根据名称获取组织
	GetOrganizationByName(ctx context.Context, name string) (*entity.Organization, error)

	// ListOrganizations 列出所有组织
	ListOrganizations(ctx context.Context, isActive *bool) ([]*entity.Organization, error)

	// UpdateOrganization 更新组织信息
	UpdateOrganization(ctx context.Context, org *entity.Organization) error

	// DeleteOrganization 删除组织
	DeleteOrganization(ctx context.Context, id uint) error

	// AddUserToOrg 添加用户到组织
	AddUserToOrg(ctx context.Context, userOrg *entity.UserOrganization) error

	// RemoveUserFromOrg 从组织移除用户
	RemoveUserFromOrg(ctx context.Context, userID string, orgID uint) error

	// ListOrgUsers 列出组织的所有用户
	ListOrgUsers(ctx context.Context, orgID uint) ([]string, error)

	// GetUserOrganizations 获取用户所属的所有组织
	GetUserOrganizations(ctx context.Context, userID string) ([]*entity.Organization, error)

	// IsUserInOrg 判断用户是否属于组织
	IsUserInOrg(ctx context.Context, userID string, orgID uint) (bool, error)
}

// ProjectRepository 项目仓储接口
type ProjectRepository interface {
	// CreateProject 创建项目
	CreateProject(ctx context.Context, project *entity.Project) error

	// GetProjectByID 根据ID获取项目
	GetProjectByID(ctx context.Context, id uint) (*entity.Project, error)

	// GetProjectByName 根据组织ID和名称获取项目
	GetProjectByName(ctx context.Context, orgID uint, name string) (*entity.Project, error)

	// ListProjectsByOrg 列出组织的所有项目
	ListProjectsByOrg(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Project, error)

	// GetDefaultProject 获取组织的默认项目
	GetDefaultProject(ctx context.Context, orgID uint) (*entity.Project, error)

	// UpdateProject 更新项目信息
	UpdateProject(ctx context.Context, project *entity.Project) error

	// DeleteProject 删除项目
	DeleteProject(ctx context.Context, id uint) error

	// GetOrgIDByProjectID 根据项目ID获取组织ID
	GetOrgIDByProjectID(ctx context.Context, projectID uint) (uint, error)

	// ListWorkspacesByProject 列出项目的所有工作空间（返回语义化ID列表）
	ListWorkspacesByProject(ctx context.Context, projectID uint) ([]string, error)

	// GetProjectByWorkspaceID 根据工作空间语义化ID获取项目
	GetProjectByWorkspaceID(ctx context.Context, workspaceID string) (*entity.Project, error)

	// AssignWorkspaceToProject 将工作空间分配到项目（使用语义化ID）
	AssignWorkspaceToProject(ctx context.Context, workspaceID string, projectID uint) error

	// RemoveWorkspaceFromProject 从项目移除工作空间（使用语义化ID）
	RemoveWorkspaceFromProject(ctx context.Context, workspaceID string) error

	// GetWorkspaceProjectRelation 获取工作空间的项目关联
	GetWorkspaceProjectRelation(ctx context.Context, workspaceID string) (*entity.WorkspaceProjectRelation, error)

	// GetDB 获取数据库实例（用于特殊查询）
	GetDB() *gorm.DB

	// GetWorkspaceIDBySemanticID 根据语义化ID获取数字ID
	GetWorkspaceIDBySemanticID(ctx context.Context, semanticID string) (uint, error)

	// GetProjectIDByWorkspaceID 根据工作空间数字ID获取项目ID
	GetProjectIDByWorkspaceID(ctx context.Context, workspaceID uint) (uint, error)
}
