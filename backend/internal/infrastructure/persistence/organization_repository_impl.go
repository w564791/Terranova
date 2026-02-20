package persistence

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
)

// OrganizationRepositoryImpl 组织仓储GORM实现
type OrganizationRepositoryImpl struct {
	db *gorm.DB
}

// NewOrganizationRepository 创建组织仓储实例
func NewOrganizationRepository(db *gorm.DB) repository.OrganizationRepository {
	return &OrganizationRepositoryImpl{db: db}
}

// CreateOrganization 创建组织
func (r *OrganizationRepositoryImpl) CreateOrganization(ctx context.Context, org *entity.Organization) error {
	if err := r.db.WithContext(ctx).Create(org).Error; err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}
	return nil
}

// GetOrganizationByID 根据ID获取组织
func (r *OrganizationRepositoryImpl) GetOrganizationByID(ctx context.Context, id uint) (*entity.Organization, error) {
	var org entity.Organization
	if err := r.db.WithContext(ctx).First(&org, id).Error; err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}
	return &org, nil
}

// GetOrganizationByName 根据名称获取组织
func (r *OrganizationRepositoryImpl) GetOrganizationByName(ctx context.Context, name string) (*entity.Organization, error) {
	var org entity.Organization
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&org).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get organization by name: %w", err)
	}
	return &org, nil
}

// ListOrganizations 列出所有组织
func (r *OrganizationRepositoryImpl) ListOrganizations(ctx context.Context, isActive *bool) ([]*entity.Organization, error) {
	var orgs []*entity.Organization
	query := r.db.WithContext(ctx)

	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Order("created_at DESC").Find(&orgs).Error; err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}
	return orgs, nil
}

// UpdateOrganization 更新组织信息
func (r *OrganizationRepositoryImpl) UpdateOrganization(ctx context.Context, org *entity.Organization) error {
	if err := r.db.WithContext(ctx).Save(org).Error; err != nil {
		return fmt.Errorf("failed to update organization: %w", err)
	}
	return nil
}

// DeleteOrganization 删除组织
func (r *OrganizationRepositoryImpl) DeleteOrganization(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&entity.Organization{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	return nil
}

// AddUserToOrg 添加用户到组织
func (r *OrganizationRepositoryImpl) AddUserToOrg(ctx context.Context, userOrg *entity.UserOrganization) error {
	if err := r.db.WithContext(ctx).Create(userOrg).Error; err != nil {
		return fmt.Errorf("failed to add user to organization: %w", err)
	}
	return nil
}

// RemoveUserFromOrg 从组织移除用户
func (r *OrganizationRepositoryImpl) RemoveUserFromOrg(ctx context.Context, userID string, orgID uint) error {
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND org_id = ?", userID, orgID).
		Delete(&entity.UserOrganization{}).Error; err != nil {
		return fmt.Errorf("failed to remove user from organization: %w", err)
	}
	return nil
}

// ListOrgUsers 列出组织的所有用户
func (r *OrganizationRepositoryImpl) ListOrgUsers(ctx context.Context, orgID uint) ([]string, error) {
	var userIDs []string
	if err := r.db.WithContext(ctx).
		Model(&entity.UserOrganization{}).
		Where("org_id = ?", orgID).
		Pluck("user_id", &userIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to list org users: %w", err)
	}
	return userIDs, nil
}

// GetUserOrganizations 获取用户所属的所有组织
func (r *OrganizationRepositoryImpl) GetUserOrganizations(ctx context.Context, userID string) ([]*entity.Organization, error) {
	var orgs []*entity.Organization
	if err := r.db.WithContext(ctx).
		Joins("JOIN user_organizations ON user_organizations.org_id = organizations.id").
		Where("user_organizations.user_id = ?", userID).
		Find(&orgs).Error; err != nil {
		return nil, fmt.Errorf("failed to get user organizations: %w", err)
	}
	return orgs, nil
}

// IsUserInOrg 判断用户是否属于组织
func (r *OrganizationRepositoryImpl) IsUserInOrg(ctx context.Context, userID string, orgID uint) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&entity.UserOrganization{}).
		Where("user_id = ? AND org_id = ?", userID, orgID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check user in org: %w", err)
	}
	return count > 0, nil
}

// ProjectRepositoryImpl 项目仓储GORM实现
type ProjectRepositoryImpl struct {
	db *gorm.DB
}

// NewProjectRepository 创建项目仓储实例
func NewProjectRepository(db *gorm.DB) repository.ProjectRepository {
	return &ProjectRepositoryImpl{db: db}
}

// CreateProject 创建项目
func (r *ProjectRepositoryImpl) CreateProject(ctx context.Context, project *entity.Project) error {
	if err := r.db.WithContext(ctx).Create(project).Error; err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	return nil
}

// GetProjectByID 根据ID获取项目
func (r *ProjectRepositoryImpl) GetProjectByID(ctx context.Context, id uint) (*entity.Project, error) {
	var project entity.Project
	if err := r.db.WithContext(ctx).Preload("Organization").First(&project, id).Error; err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}
	return &project, nil
}

// GetProjectByName 根据组织ID和名称获取项目
func (r *ProjectRepositoryImpl) GetProjectByName(ctx context.Context, orgID uint, name string) (*entity.Project, error) {
	var project entity.Project
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND name = ?", orgID, name).
		First(&project).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get project by name: %w", err)
	}
	return &project, nil
}

// ListProjectsByOrg 列出组织的所有项目
func (r *ProjectRepositoryImpl) ListProjectsByOrg(ctx context.Context, orgID uint, isActive *bool) ([]*entity.Project, error) {
	var projects []*entity.Project
	query := r.db.WithContext(ctx).Where("org_id = ?", orgID)

	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	if err := query.Order("is_default DESC, created_at DESC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	return projects, nil
}

// GetDefaultProject 获取组织的默认项目
func (r *ProjectRepositoryImpl) GetDefaultProject(ctx context.Context, orgID uint) (*entity.Project, error) {
	var project entity.Project
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND is_default = ?", orgID, true).
		First(&project).Error; err != nil {
		return nil, fmt.Errorf("default project not found: %w", err)
	}
	return &project, nil
}

// UpdateProject 更新项目信息
func (r *ProjectRepositoryImpl) UpdateProject(ctx context.Context, project *entity.Project) error {
	if err := r.db.WithContext(ctx).Save(project).Error; err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	return nil
}

// DeleteProject 删除项目
func (r *ProjectRepositoryImpl) DeleteProject(ctx context.Context, id uint) error {
	if err := r.db.WithContext(ctx).Delete(&entity.Project{}, id).Error; err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}

// GetOrgIDByProjectID 根据项目ID获取组织ID
func (r *ProjectRepositoryImpl) GetOrgIDByProjectID(ctx context.Context, projectID uint) (uint, error) {
	var orgID uint
	if err := r.db.WithContext(ctx).
		Model(&entity.Project{}).
		Where("id = ?", projectID).
		Pluck("org_id", &orgID).Error; err != nil {
		return 0, fmt.Errorf("failed to get org_id: %w", err)
	}
	return orgID, nil
}

// ListWorkspacesByProject 列出项目的所有工作空间（返回语义化ID列表）
func (r *ProjectRepositoryImpl) ListWorkspacesByProject(ctx context.Context, projectID uint) ([]string, error) {
	var workspaceIDs []string
	if err := r.db.WithContext(ctx).
		Model(&entity.WorkspaceProjectRelation{}).
		Where("project_id = ?", projectID).
		Pluck("workspace_id", &workspaceIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}
	return workspaceIDs, nil
}

// GetProjectByWorkspaceID 根据工作空间语义化ID获取项目
func (r *ProjectRepositoryImpl) GetProjectByWorkspaceID(ctx context.Context, workspaceID string) (*entity.Project, error) {
	var relation entity.WorkspaceProjectRelation
	if err := r.db.WithContext(ctx).
		Preload("Project").
		Where("workspace_id = ?", workspaceID).
		First(&relation).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 没有关联项目
		}
		return nil, fmt.Errorf("failed to get project by workspace_id: %w", err)
	}
	return relation.Project, nil
}

// GetDB 获取数据库实例
func (r *ProjectRepositoryImpl) GetDB() *gorm.DB {
	return r.db
}

// GetWorkspaceIDBySemanticID 根据语义化ID获取数字ID
func (r *ProjectRepositoryImpl) GetWorkspaceIDBySemanticID(ctx context.Context, semanticID string) (uint, error) {
	var workspace struct {
		ID uint `gorm:"column:id"`
	}
	if err := r.db.WithContext(ctx).Table("workspaces").
		Select("id").
		Where("workspace_id = ?", semanticID).
		First(&workspace).Error; err != nil {
		return 0, err
	}
	return workspace.ID, nil
}

// GetProjectIDByWorkspaceID 根据工作空间数字ID获取项目ID
func (r *ProjectRepositoryImpl) GetProjectIDByWorkspaceID(ctx context.Context, workspaceID uint) (uint, error) {
	// 首先获取工作空间的语义化ID
	var workspace struct {
		WorkspaceID string `gorm:"column:workspace_id"`
	}
	if err := r.db.WithContext(ctx).Table("workspaces").
		Select("workspace_id").
		Where("id = ?", workspaceID).
		First(&workspace).Error; err != nil {
		return 0, err
	}

	// 然后查询项目关联
	var relation entity.WorkspaceProjectRelation
	if err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspace.WorkspaceID).
		First(&relation).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			// 如果没有关联，返回默认项目ID（0表示未分配）
			return 0, nil
		}
		return 0, err
	}
	return relation.ProjectID, nil
}

// AssignWorkspaceToProject 将工作空间分配到项目（使用语义化ID）
func (r *ProjectRepositoryImpl) AssignWorkspaceToProject(ctx context.Context, workspaceID string, projectID uint) error {
	// 先删除已有的关联
	r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Delete(&entity.WorkspaceProjectRelation{})

	// 创建新关联
	relation := &entity.WorkspaceProjectRelation{
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
	}

	if err := r.db.WithContext(ctx).Create(relation).Error; err != nil {
		return fmt.Errorf("failed to assign workspace to project: %w", err)
	}
	return nil
}

// RemoveWorkspaceFromProject 从项目移除工作空间（使用语义化ID）
func (r *ProjectRepositoryImpl) RemoveWorkspaceFromProject(ctx context.Context, workspaceID string) error {
	if err := r.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Delete(&entity.WorkspaceProjectRelation{}).Error; err != nil {
		return fmt.Errorf("failed to remove workspace from project: %w", err)
	}
	return nil
}

// GetWorkspaceProjectRelation 获取工作空间的项目关联
func (r *ProjectRepositoryImpl) GetWorkspaceProjectRelation(ctx context.Context, workspaceID string) (*entity.WorkspaceProjectRelation, error) {
	var relation entity.WorkspaceProjectRelation
	if err := r.db.WithContext(ctx).
		Preload("Project").
		Where("workspace_id = ?", workspaceID).
		First(&relation).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get workspace project relation: %w", err)
	}
	return &relation, nil
}
