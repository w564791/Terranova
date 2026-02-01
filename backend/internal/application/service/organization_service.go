package service

import (
	"context"
	"fmt"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/repository"
)

// CreateOrganizationRequest 创建组织请求
type CreateOrganizationRequest struct {
	Name        string                 `json:"name"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	Settings    map[string]interface{} `json:"settings"`
	CreatedBy   string                 `json:"created_by"` // user_id
}

// UpdateOrganizationRequest 更新组织请求
type UpdateOrganizationRequest struct {
	ID          uint                   `json:"id"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	IsActive    bool                   `json:"is_active"`
	Settings    map[string]interface{} `json:"settings"`
}

// OrganizationService 组织管理服务接口
type OrganizationService interface {
	// CreateOrganization 创建组织
	CreateOrganization(ctx context.Context, req *CreateOrganizationRequest) (*entity.Organization, error)

	// GetOrganization 获取组织详情
	GetOrganization(ctx context.Context, orgID uint) (*entity.Organization, error)

	// ListOrganizations 列出所有组织
	ListOrganizations(ctx context.Context, isActive *bool) ([]*entity.Organization, error)

	// UpdateOrganization 更新组织信息
	UpdateOrganization(ctx context.Context, req *UpdateOrganizationRequest) error

	// DeleteOrganization 删除组织
	DeleteOrganization(ctx context.Context, orgID uint) error

	// AddUserToOrg 添加用户到组织
	AddUserToOrg(ctx context.Context, userID string, orgID uint) error

	// RemoveUserFromOrg 从组织移除用户
	RemoveUserFromOrg(ctx context.Context, userID string, orgID uint) error

	// ListOrgUsers 列出组织的所有用户
	ListOrgUsers(ctx context.Context, orgID uint) ([]string, error)
}

// OrganizationServiceImpl 组织管理服务实现
type OrganizationServiceImpl struct {
	orgRepo   repository.OrganizationRepository
	teamRepo  repository.TeamRepository
	auditRepo repository.AuditRepository
}

// NewOrganizationService 创建组织管理服务实例
func NewOrganizationService(
	orgRepo repository.OrganizationRepository,
	teamRepo repository.TeamRepository,
	auditRepo repository.AuditRepository,
) OrganizationService {
	return &OrganizationServiceImpl{
		orgRepo:   orgRepo,
		teamRepo:  teamRepo,
		auditRepo: auditRepo,
	}
}

// CreateOrganization 创建组织
func (s *OrganizationServiceImpl) CreateOrganization(
	ctx context.Context,
	req *CreateOrganizationRequest,
) (*entity.Organization, error) {
	// 1. 验证请求参数
	if err := s.validateCreateOrgRequest(req); err != nil {
		return nil, err
	}

	// 2. 检查组织名称是否已存在
	existing, _ := s.orgRepo.GetOrganizationByName(ctx, req.Name)
	if existing != nil {
		return nil, fmt.Errorf("organization name already exists")
	}

	// 3. 创建组织
	org := &entity.Organization{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsActive:    true,
		Settings:    req.Settings,
		CreatedBy:   &req.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.orgRepo.CreateOrganization(ctx, org); err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// 4. 创建默认团队（owners, admins）
	s.createDefaultTeams(ctx, org.ID, req.CreatedBy)

	return org, nil
}

// GetOrganization 获取组织详情
func (s *OrganizationServiceImpl) GetOrganization(ctx context.Context, orgID uint) (*entity.Organization, error) {
	return s.orgRepo.GetOrganizationByID(ctx, orgID)
}

// ListOrganizations 列出所有组织
func (s *OrganizationServiceImpl) ListOrganizations(ctx context.Context, isActive *bool) ([]*entity.Organization, error) {
	return s.orgRepo.ListOrganizations(ctx, isActive)
}

// UpdateOrganization 更新组织信息
func (s *OrganizationServiceImpl) UpdateOrganization(
	ctx context.Context,
	req *UpdateOrganizationRequest,
) error {
	// 1. 验证组织是否存在
	org, err := s.orgRepo.GetOrganizationByID(ctx, req.ID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	// 2. 更新字段
	org.DisplayName = req.DisplayName
	org.Description = req.Description
	org.IsActive = req.IsActive
	org.Settings = req.Settings
	org.UpdatedAt = time.Now()

	// 3. 保存更新
	if err := s.orgRepo.UpdateOrganization(ctx, org); err != nil {
		return fmt.Errorf("failed to update organization: %w", err)
	}

	return nil
}

// DeleteOrganization 删除组织
func (s *OrganizationServiceImpl) DeleteOrganization(ctx context.Context, orgID uint) error {
	// 验证组织是否存在
	_, err := s.orgRepo.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	// 删除组织（会级联删除项目、团队、权限等）
	if err := s.orgRepo.DeleteOrganization(ctx, orgID); err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	return nil
}

// AddUserToOrg 添加用户到组织
func (s *OrganizationServiceImpl) AddUserToOrg(ctx context.Context, userID string, orgID uint) error {
	// 验证组织是否存在
	_, err := s.orgRepo.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	// 检查用户是否已在组织中
	isIn, err := s.orgRepo.IsUserInOrg(ctx, userID, orgID)
	if err != nil {
		return fmt.Errorf("failed to check user membership: %w", err)
	}
	if isIn {
		return fmt.Errorf("user is already in organization")
	}

	// 添加用户到组织
	userOrg := &entity.UserOrganization{
		UserID:   userID,
		OrgID:    orgID,
		JoinedAt: time.Now(),
	}

	return s.orgRepo.AddUserToOrg(ctx, userOrg)
}

// RemoveUserFromOrg 从组织移除用户
func (s *OrganizationServiceImpl) RemoveUserFromOrg(ctx context.Context, userID string, orgID uint) error {
	return s.orgRepo.RemoveUserFromOrg(ctx, userID, orgID)
}

// ListOrgUsers 列出组织的所有用户
func (s *OrganizationServiceImpl) ListOrgUsers(ctx context.Context, orgID uint) ([]string, error) {
	return s.orgRepo.ListOrgUsers(ctx, orgID)
}

// createDefaultTeams 创建默认团队
func (s *OrganizationServiceImpl) createDefaultTeams(ctx context.Context, orgID uint, createdBy string) {
	// 创建owners团队
	ownersTeam := &entity.Team{
		OrgID:       orgID,
		Name:        "owners",
		DisplayName: "Organization Owners",
		IsSystem:    true,
		CreatedBy:   &createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_ = s.teamRepo.CreateTeam(ctx, ownersTeam)

	// 创建admins团队
	adminsTeam := &entity.Team{
		OrgID:       orgID,
		Name:        "admins",
		DisplayName: "Organization Admins",
		IsSystem:    true,
		CreatedBy:   &createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	_ = s.teamRepo.CreateTeam(ctx, adminsTeam)
}

// ProjectService 项目管理服务接口
type ProjectService interface {
	// CreateProject 创建项目
	CreateProject(ctx context.Context, req *CreateProjectRequest) (*entity.Project, error)

	// GetProject 获取项目详情
	GetProject(ctx context.Context, projectID uint) (*entity.Project, error)

	// ListProjectsByOrg 列出组织的所有项目
	ListProjectsByOrg(ctx context.Context, orgID uint) ([]*entity.Project, error)

	// UpdateProject 更新项目信息
	UpdateProject(ctx context.Context, req *UpdateProjectRequest) error

	// DeleteProject 删除项目
	DeleteProject(ctx context.Context, projectID uint) error

	// AssignWorkspace 将工作空间分配到项目
	AssignWorkspace(ctx context.Context, workspaceID string, projectID uint) error
}

// CreateProjectRequest 创建项目请求
type CreateProjectRequest struct {
	OrgID       uint                   `json:"org_id"`
	Name        string                 `json:"name"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	Settings    map[string]interface{} `json:"settings"`
	CreatedBy   string                 `json:"created_by"` // user_id
}

// UpdateProjectRequest 更新项目请求
type UpdateProjectRequest struct {
	ID          uint                   `json:"id"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	IsActive    bool                   `json:"is_active"`
	Settings    map[string]interface{} `json:"settings"`
}

// ProjectServiceImpl 项目管理服务实现
type ProjectServiceImpl struct {
	projectRepo repository.ProjectRepository
	orgRepo     repository.OrganizationRepository
	auditRepo   repository.AuditRepository
}

// NewProjectService 创建项目管理服务实例
func NewProjectService(
	projectRepo repository.ProjectRepository,
	orgRepo repository.OrganizationRepository,
	auditRepo repository.AuditRepository,
) ProjectService {
	return &ProjectServiceImpl{
		projectRepo: projectRepo,
		orgRepo:     orgRepo,
		auditRepo:   auditRepo,
	}
}

// CreateProject 创建项目
func (s *ProjectServiceImpl) CreateProject(
	ctx context.Context,
	req *CreateProjectRequest,
) (*entity.Project, error) {
	// 1. 验证请求参数
	if err := s.validateCreateProjectRequest(req); err != nil {
		return nil, err
	}

	// 2. 验证组织是否存在
	_, err := s.orgRepo.GetOrganizationByID(ctx, req.OrgID)
	if err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	// 3. 检查项目名称是否已存在
	existing, _ := s.projectRepo.GetProjectByName(ctx, req.OrgID, req.Name)
	if existing != nil {
		return nil, fmt.Errorf("project name already exists in organization")
	}

	// 4. 创建项目
	project := &entity.Project{
		OrgID:       req.OrgID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsDefault:   false,
		IsActive:    true,
		Settings:    req.Settings,
		CreatedBy:   &req.CreatedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.projectRepo.CreateProject(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return project, nil
}

// GetProject 获取项目详情
func (s *ProjectServiceImpl) GetProject(ctx context.Context, projectID uint) (*entity.Project, error) {
	return s.projectRepo.GetProjectByID(ctx, projectID)
}

// ListProjectsByOrg 列出组织的所有项目
func (s *ProjectServiceImpl) ListProjectsByOrg(ctx context.Context, orgID uint) ([]*entity.Project, error) {
	return s.projectRepo.ListProjectsByOrg(ctx, orgID, nil)
}

// UpdateProject 更新项目信息
func (s *ProjectServiceImpl) UpdateProject(
	ctx context.Context,
	req *UpdateProjectRequest,
) error {
	// 1. 验证项目是否存在
	project, err := s.projectRepo.GetProjectByID(ctx, req.ID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// 2. 更新字段
	project.DisplayName = req.DisplayName
	project.Description = req.Description
	project.IsActive = req.IsActive
	project.Settings = req.Settings
	project.UpdatedAt = time.Now()

	// 3. 保存更新
	if err := s.projectRepo.UpdateProject(ctx, project); err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	return nil
}

// DeleteProject 删除项目
func (s *ProjectServiceImpl) DeleteProject(ctx context.Context, projectID uint) error {
	// 1. 验证项目是否存在
	project, err := s.projectRepo.GetProjectByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// 2. 默认项目不允许删除
	if project.IsDefault {
		return fmt.Errorf("cannot delete default project")
	}

	// 3. 删除项目（会级联删除权限等）
	if err := s.projectRepo.DeleteProject(ctx, projectID); err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	return nil
}

// AssignWorkspace 将工作空间分配到项目
func (s *ProjectServiceImpl) AssignWorkspace(ctx context.Context, workspaceID string, projectID uint) error {
	// 验证项目是否存在
	_, err := s.projectRepo.GetProjectByID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("project not found: %w", err)
	}

	// 分配工作空间
	return s.projectRepo.AssignWorkspaceToProject(ctx, workspaceID, projectID)
}

// validateCreateOrgRequest 验证创建组织请求
func (s *OrganizationServiceImpl) validateCreateOrgRequest(req *CreateOrganizationRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(req.Name) > 100 {
		return fmt.Errorf("name too long (max 100 characters)")
	}
	if req.CreatedBy == "" {
		return fmt.Errorf("created_by is required")
	}
	return nil
}

// validateCreateProjectRequest 验证创建项目请求
func (s *ProjectServiceImpl) validateCreateProjectRequest(req *CreateProjectRequest) error {
	if req.OrgID == 0 {
		return fmt.Errorf("org_id is required")
	}
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(req.Name) > 100 {
		return fmt.Errorf("name too long (max 100 characters)")
	}
	if req.CreatedBy == "" {
		return fmt.Errorf("created_by is required")
	}
	return nil
}
