# IAM权限系统集成指南

> 如何将IAM权限系统集成到IaC Platform

---

## 📋 当前状态

###  已完成
- [x] 数据库表结构 (20个表)
- [x] Domain层 (值对象、实体、Repository接口)
- [x] Service层 (4个核心服务)
- [x] Repository实现 (4个GORM实现)
- [x] HTTP Handlers (3个文件, 22个API)
- [x] 路由配置 (已添加到router.go)

### ⏸️ 待完成
- [ ] 服务初始化和依赖注入
- [ ] 启用路由
- [ ] API测试

---

## 🚀 集成步骤

### 步骤1: 初始化IAM服务

在 `backend/main.go` 或创建新的 `backend/internal/iam/factory.go` 文件：

```go
package iam

import (
	"gorm.io/gorm"
	
	"iac-platform/backend/internal/application/service"
	"iac-platform/backend/internal/infrastructure/persistence"
)

// ServiceFactory IAM服务工厂
type ServiceFactory struct {
	db *gorm.DB
	
	// Repositories
	permissionRepo *persistence.PermissionRepositoryImpl
	teamRepo       *persistence.TeamRepositoryImpl
	orgRepo        *persistence.OrganizationRepositoryImpl
	projectRepo    *persistence.ProjectRepositoryImpl
	auditRepo      *persistence.AuditRepositoryImpl
	
	// Services
	permissionChecker service.PermissionChecker
	permissionService service.PermissionService
	teamService       service.TeamService
	orgService        service.OrganizationService
	projectService    service.ProjectService
}

// NewServiceFactory 创建服务工厂
func NewServiceFactory(db *gorm.DB) *ServiceFactory {
	factory := &ServiceFactory{db: db}
	
	// 初始化Repositories
	factory.permissionRepo = persistence.NewPermissionRepository(db).(*persistence.PermissionRepositoryImpl)
	factory.teamRepo = persistence.NewTeamRepository(db).(*persistence.TeamRepositoryImpl)
	factory.orgRepo = persistence.NewOrganizationRepository(db).(*persistence.OrganizationRepositoryImpl)
	factory.projectRepo = persistence.NewProjectRepository(db).(*persistence.ProjectRepositoryImpl)
	factory.auditRepo = persistence.NewAuditRepository(db).(*persistence.AuditRepositoryImpl)
	
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
```

### 步骤2: 修改router.go启用IAM路由

在 `backend/internal/router/router.go` 的 `Setup` 函数中：

```go
func Setup(db *gorm.DB, streamManager *services.OutputStreamManager) *gin.Engine {
	// ... 现有代码 ...
	
	// 初始化IAM服务工厂
	iamFactory := iam.NewServiceFactory(db)
	
	// 在protected路由组中添加：
	protected := api.Group("")
	protected.Use(middleware.JWTAuth())
	{
		// ... 现有路由 ...
		
		// IAM权限系统
		iamGroup := protected.Group("/iam")
		{
			// 初始化handlers
			permissionHandler := handlers.NewPermissionHandler(
				iamFactory.GetPermissionService(),
				iamFactory.GetPermissionChecker(),
			)
			teamHandler := handlers.NewTeamHandler(iamFactory.GetTeamService())
			orgHandler := handlers.NewOrganizationHandler(
				iamFactory.GetOrganizationService(),
				iamFactory.GetProjectService(),
			)
			
			// 权限管理
			iamGroup.POST("/permissions/check", permissionHandler.CheckPermission)
			iamGroup.POST("/permissions/grant", permissionHandler.GrantPermission)
			iamGroup.POST("/permissions/grant-preset", permissionHandler.GrantPresetPermissions)
			iamGroup.DELETE("/permissions/:scope_type/:id", permissionHandler.RevokePermission)
			iamGroup.GET("/permissions/:scope_type/:scope_id", permissionHandler.ListPermissions)
			iamGroup.GET("/permissions/definitions", permissionHandler.ListPermissionDefinitions)
			
			// 团队管理
			iamGroup.POST("/teams", teamHandler.CreateTeam)
			iamGroup.GET("/teams", teamHandler.ListTeamsByOrg)
			iamGroup.GET("/teams/:id", teamHandler.GetTeam)
			iamGroup.DELETE("/teams/:id", teamHandler.DeleteTeam)
			iamGroup.POST("/teams/:id/members", teamHandler.AddTeamMember)
			iamGroup.DELETE("/teams/:id/members/:user_id", teamHandler.RemoveTeamMember)
			iamGroup.GET("/teams/:id/members", teamHandler.ListTeamMembers)
			
			// 组织管理
			iamGroup.POST("/organizations", orgHandler.CreateOrganization)
			iamGroup.GET("/organizations", orgHandler.ListOrganizations)
			iamGroup.GET("/organizations/:id", orgHandler.GetOrganization)
			iamGroup.PUT("/organizations/:id", orgHandler.UpdateOrganization)
			
			// 项目管理
			iamGroup.POST("/projects", orgHandler.CreateProject)
			iamGroup.GET("/projects", orgHandler.ListProjects)
			iamGroup.GET("/projects/:id", orgHandler.GetProject)
			iamGroup.PUT("/projects/:id", orgHandler.UpdateProject)
			iamGroup.DELETE("/projects/:id", orgHandler.DeleteProject)
		}
	}
	
	return r
}
```

### 步骤3: 运行数据库迁移

如果还没有运行迁移脚本：

```bash
psql postgresql://postgres:postgres123@localhost:5432/iac_platform -f scripts/migrate_iam_system.sql
```

验证表创建：

```bash
psql postgresql://postgres:postgres123@localhost:5432/iac_platform -c "\dt" | grep -E "(organizations|projects|teams|permissions)"
```

### 步骤4: 测试API

启动服务器后，测试IAM状态端点：

```bash
curl -X GET http://localhost:8080/api/v1/iam/status \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

---

## 📝 API端点列表

### 权限管理 (6个)
```
POST   /api/v1/iam/permissions/check
POST   /api/v1/iam/permissions/grant
POST   /api/v1/iam/permissions/grant-preset
DELETE /api/v1/iam/permissions/{scope_type}/{id}
GET    /api/v1/iam/permissions/{scope_type}/{scope_id}
GET    /api/v1/iam/permissions/definitions
```

### 团队管理 (7个)
```
POST   /api/v1/iam/teams
GET    /api/v1/iam/teams
GET    /api/v1/iam/teams/{id}
DELETE /api/v1/iam/teams/{id}
POST   /api/v1/iam/teams/{id}/members
DELETE /api/v1/iam/teams/{id}/members/{user_id}
GET    /api/v1/iam/teams/{id}/members
```

### 组织管理 (4个)
```
POST   /api/v1/iam/organizations
GET    /api/v1/iam/organizations
GET    /api/v1/iam/organizations/{id}
PUT    /api/v1/iam/organizations/{id}
```

### 项目管理 (5个)
```
POST   /api/v1/iam/projects
GET    /api/v1/iam/projects
GET    /api/v1/iam/projects/{id}
PUT    /api/v1/iam/projects/{id}
DELETE /api/v1/iam/projects/{id}
```

---

## 🔧 使用示例

### 1. 检查权限

```bash
curl -X POST http://localhost:8080/api/v1/iam/permissions/check \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resource_type": "TASK_DATA_ACCESS",
    "scope_type": "WORKSPACE",
    "scope_id": 1,
    "required_level": "READ"
  }'
```

### 2. 授予权限

```bash
curl -X POST http://localhost:8080/api/v1/iam/permissions/grant \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "scope_type": "WORKSPACE",
    "scope_id": 1,
    "principal_type": "USER",
    "principal_id": 2,
    "permission_id": 8,
    "permission_level": "WRITE",
    "reason": "Grant workspace access"
  }'
```

### 3. 创建团队

```bash
curl -X POST http://localhost:8080/api/v1/iam/teams \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "org_id": 1,
    "name": "developers",
    "display_name": "Development Team",
    "description": "Core development team"
  }'
```

---

## 📚 相关文档

- [设计文档](./iac-platform-permission-system-design-v2.md)
- [实施进度](./implementation-progress.md)
- [任务清单](./TASKS.md)
- [UI原型](./admin-ui-prototype.md)

---

*最后更新: 2025-10-21*
