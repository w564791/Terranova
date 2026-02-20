package iam

import (
	"gorm.io/gorm"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/repository"
	"iac-platform/internal/infrastructure/persistence"
)

// ServiceFactory IAM服务工厂
type ServiceFactory struct {
	db *gorm.DB

	// Repositories
	permissionRepo  repository.PermissionRepository
	teamRepo        repository.TeamRepository
	orgRepo         repository.OrganizationRepository
	projectRepo     repository.ProjectRepository
	auditRepo       repository.AuditRepository
	applicationRepo repository.ApplicationRepository

	// Services
	permissionChecker  service.PermissionChecker
	permissionService  service.PermissionService
	teamService        service.TeamService
	orgService         service.OrganizationService
	projectService     service.ProjectService
	applicationService *service.ApplicationService
	auditService       *service.AuditService
}

// NewServiceFactory 创建IAM服务工厂
func NewServiceFactory(db *gorm.DB) *ServiceFactory {
	factory := &ServiceFactory{db: db}

	// 初始化Repositories
	factory.permissionRepo = persistence.NewPermissionRepository(db)
	factory.teamRepo = persistence.NewTeamRepository(db)
	factory.orgRepo = persistence.NewOrganizationRepository(db)
	factory.projectRepo = persistence.NewProjectRepository(db)
	factory.auditRepo = persistence.NewAuditRepository(db)
	factory.applicationRepo = persistence.NewApplicationRepository(db)

	// 初始化Services
	factory.permissionChecker = service.NewPermissionChecker(
		factory.permissionRepo,
		factory.teamRepo,
		factory.orgRepo,
		factory.projectRepo,
		factory.auditRepo,
	)

	factory.permissionService = service.NewPermissionService(
		factory.permissionRepo,
		factory.auditRepo,
		factory.permissionChecker,
		factory.db,
	)

	factory.teamService = service.NewTeamService(
		factory.teamRepo,
		factory.orgRepo,
		factory.auditRepo,
	)

	factory.orgService = service.NewOrganizationService(
		factory.orgRepo,
		factory.teamRepo,
		factory.auditRepo,
	)

	factory.projectService = service.NewProjectService(
		factory.projectRepo,
		factory.orgRepo,
		factory.auditRepo,
	)

	factory.applicationService = service.NewApplicationService(
		factory.applicationRepo,
	)

	factory.auditService = service.NewAuditService(
		factory.auditRepo,
	)

	return factory
}

// GetPermissionChecker 获取权限检查器
func (f *ServiceFactory) GetPermissionChecker() service.PermissionChecker {
	return f.permissionChecker
}

// GetPermissionService 获取权限服务
func (f *ServiceFactory) GetPermissionService() service.PermissionService {
	return f.permissionService
}

// GetTeamService 获取团队服务
func (f *ServiceFactory) GetTeamService() service.TeamService {
	return f.teamService
}

// GetOrganizationService 获取组织服务
func (f *ServiceFactory) GetOrganizationService() service.OrganizationService {
	return f.orgService
}

// GetProjectService 获取项目服务
func (f *ServiceFactory) GetProjectService() service.ProjectService {
	return f.projectService
}

// GetApplicationService 获取应用服务
func (f *ServiceFactory) GetApplicationService() *service.ApplicationService {
	return f.applicationService
}

// GetAuditService 获取审计服务
func (f *ServiceFactory) GetAuditService() *service.AuditService {
	return f.auditService
}
