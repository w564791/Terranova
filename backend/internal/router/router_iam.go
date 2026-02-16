package router

import (
	"iac-platform/internal/application/service"
	"iac-platform/internal/handlers"
	"iac-platform/internal/iam"
	"iac-platform/internal/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupIAMRoutes sets up IAM (Identity and Access Management) routes
// 包括: permissions, teams, organizations, projects, applications, audit, users, roles
func setupIAMRoutes(adminProtected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// TODO: 实现IAM路由
	// 参考原router.go中的 iamGroup := adminProtected.Group("/iam") 部分
	// 包括以下子模块：
	// - 权限管理 (permissions)
	// - 团队管理 (teams)
	// - 团队Token管理 (team tokens)
	// - 团队角色管理 (team roles)
	// - 组织管理 (organizations)
	// - 项目管理 (projects)
	// - 应用管理 (applications)
	// - 审计日志 (audit)
	// - 用户管理 (users)
	// - 角色管理 (roles)
	iamGroup := adminProtected.Group("/iam")
	{
		// 状态端点
		iamGroup.GET("/status", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status":  "IAM system enabled",
				"version": "v1.0.0",
				"apis":    22,
			})
		})

		// 初始化IAM服务工厂
		iamFactory := iam.NewServiceFactory(db)

		// 初始化IAM handlers
		permissionHandler := handlers.NewPermissionHandler(
			iamFactory.GetPermissionService(),
			iamFactory.GetPermissionChecker(),
			iamFactory.GetTeamService(),
			db,
		)
		teamHandler := handlers.NewTeamHandler(iamFactory.GetTeamService())
		orgHandler := handlers.NewOrganizationHandler(
			iamFactory.GetOrganizationService(),
			iamFactory.GetProjectService(),
		)
		applicationHandler := handlers.NewApplicationHandler(iamFactory.GetApplicationService())
		auditHandler := handlers.NewAuditHandler(iamFactory.GetAuditService())
		auditConfigHandler := handlers.NewAuditConfigHandler(db)
		userHandler := handlers.NewUserHandler(service.NewUserService(db))
		roleHandler := handlers.NewRoleHandler(db)

		// 权限管理 - 其他权限管理API
		iamGroup.POST("/permissions/grant",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN"),
			permissionHandler.GrantPermission,
		)

		iamGroup.POST("/permissions/batch-grant",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN"),
			permissionHandler.BatchGrantPermissions,
		)

		iamGroup.POST("/permissions/grant-preset",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN"),
			permissionHandler.GrantPresetPermissions,
		)

		iamGroup.DELETE("/permissions/:scope_type/:id",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN"),
			permissionHandler.RevokePermission,
		)

		iamGroup.GET("/permissions/:scope_type/:scope_id",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ"),
			permissionHandler.ListPermissions,
		)

		iamGroup.GET("/permissions/definitions",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ"),
			permissionHandler.ListPermissionDefinitions,
		)

		// 新增：按主体查询权限的API
		iamGroup.GET("/users/:id/permissions",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ"),
			permissionHandler.ListUserPermissions,
		)

		iamGroup.GET("/teams/:id/permissions",
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ"),
			permissionHandler.ListTeamPermissions,
		)

		// 团队管理 - 添加IAM权限检查
		iamGroup.POST("/teams",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE"),
			teamHandler.CreateTeam,
		)

		iamGroup.GET("/teams",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ"),
			teamHandler.ListTeamsByOrg,
		)

		iamGroup.GET("/teams/:id",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ"),
			teamHandler.GetTeam,
		)

		iamGroup.DELETE("/teams/:id",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "ADMIN"),
			teamHandler.DeleteTeam,
		)

		iamGroup.POST("/teams/:id/members",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE"),
			teamHandler.AddTeamMember,
		)

		iamGroup.DELETE("/teams/:id/members/:user_id",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE"),
			teamHandler.RemoveTeamMember,
		)

		iamGroup.GET("/teams/:id/members",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ"),
			teamHandler.ListTeamMembers,
		)

		// 团队Token管理 - 使用统一JWT密钥
		teamTokenHandler := handlers.NewTeamTokenHandler(service.NewTeamTokenService(db, ""))

		iamGroup.POST("/teams/:id/tokens",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE"),
			teamTokenHandler.CreateTeamToken,
		)

		iamGroup.GET("/teams/:id/tokens",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ"),
			teamTokenHandler.ListTeamTokens,
		)

		iamGroup.DELETE("/teams/:id/tokens/:token_id",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE"),
			teamTokenHandler.RevokeTeamToken,
		)

		// 团队角色管理 - 添加IAM权限检查
		iamGroup.POST("/teams/:id/roles",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "ADMIN"),
			roleHandler.AssignTeamRole,
		)

		iamGroup.GET("/teams/:id/roles",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ"),
			roleHandler.ListTeamRoles,
		)

		iamGroup.DELETE("/teams/:id/roles/:assignment_id",
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "ADMIN"),
			roleHandler.RevokeTeamRole,
		)

		// 组织管理 - 添加IAM权限检查
		iamGroup.POST("/organizations",
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "ADMIN"),
			orgHandler.CreateOrganization,
		)

		iamGroup.GET("/organizations",
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "READ"),
			orgHandler.ListOrganizations,
		)

		iamGroup.GET("/organizations/:id",
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "READ"),
			orgHandler.GetOrganization,
		)

		iamGroup.PUT("/organizations/:id",
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "WRITE"),
			orgHandler.UpdateOrganization,
		)

		iamGroup.DELETE("/organizations/:id",
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "ADMIN"),
			orgHandler.DeleteOrganization,
		)

		// 项目管理 - 添加IAM权限检查
		iamGroup.POST("/projects",
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "WRITE"),
			orgHandler.CreateProject,
		)

		iamGroup.GET("/projects",
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "READ"),
			orgHandler.ListProjects,
		)

		iamGroup.GET("/projects/:id",
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "READ"),
			orgHandler.GetProject,
		)

		iamGroup.PUT("/projects/:id",
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "WRITE"),
			orgHandler.UpdateProject,
		)

		iamGroup.DELETE("/projects/:id",
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "ADMIN"),
			orgHandler.DeleteProject,
		)

		// 应用管理 - 添加IAM权限检查
		iamGroup.POST("/applications",
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "WRITE"),
			applicationHandler.CreateApplication,
		)

		iamGroup.GET("/applications",
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "READ"),
			applicationHandler.ListApplications,
		)

		iamGroup.GET("/applications/:id",
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "READ"),
			applicationHandler.GetApplication,
		)

		iamGroup.PUT("/applications/:id",
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "WRITE"),
			applicationHandler.UpdateApplication,
		)

		iamGroup.DELETE("/applications/:id",
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "ADMIN"),
			applicationHandler.DeleteApplication,
		)

		iamGroup.POST("/applications/:id/regenerate-secret",
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "ADMIN"),
			applicationHandler.RegenerateSecret,
		)

		// 审计日志 - 添加IAM权限检查
		iamGroup.GET("/audit/config",
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ"),
			auditConfigHandler.GetAuditConfig,
		)

		iamGroup.PUT("/audit/config",
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "ADMIN"),
			auditConfigHandler.UpdateAuditConfig,
		)

		iamGroup.GET("/audit/permission-history",
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ"),
			auditHandler.QueryPermissionHistory,
		)

		iamGroup.GET("/audit/access-history",
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ"),
			auditHandler.QueryAccessHistory,
		)

		iamGroup.GET("/audit/denied-access",
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ"),
			auditHandler.QueryDeniedAccess,
		)

		iamGroup.GET("/audit/permission-changes-by-principal",
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ"),
			auditHandler.QueryPermissionChangesByPrincipal,
		)

		iamGroup.GET("/audit/permission-changes-by-performer",
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ"),
			auditHandler.QueryPermissionChangesByPerformer,
		)

		// 用户管理 - 添加IAM权限检查
		iamGroup.GET("/users/stats",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ"),
			userHandler.GetUserStats,
		)

		iamGroup.GET("/users",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ"),
			userHandler.ListUsers,
		)

		iamGroup.POST("/users",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "WRITE"),
			userHandler.CreateUser,
		)

		// 用户角色分配（使用 /users/:id/roles 路径）
		iamGroup.POST("/users/:id/roles",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN"),
			roleHandler.AssignRole,
		)

		iamGroup.DELETE("/users/:id/roles/:assignment_id",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN"),
			roleHandler.RevokeRole,
		)

		iamGroup.GET("/users/:id/roles",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ"),
			roleHandler.ListUserRoles,
		)

		// 用户基本操作
		iamGroup.GET("/users/:id",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ"),
			userHandler.GetUser,
		)

		iamGroup.PUT("/users/:id",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "WRITE"),
			userHandler.UpdateUser,
		)

		iamGroup.POST("/users/:id/activate",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN"),
			userHandler.ActivateUser,
		)

		iamGroup.POST("/users/:id/deactivate",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN"),
			userHandler.DeactivateUser,
		)

		iamGroup.DELETE("/users/:id",
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN"),
			userHandler.DeleteUser,
		)

		// 角色管理 - 添加IAM权限检查
		iamGroup.GET("/roles",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "READ"),
			roleHandler.ListRoles,
		)

		iamGroup.GET("/roles/:id",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "READ"),
			roleHandler.GetRole,
		)

		iamGroup.POST("/roles",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE"),
			roleHandler.CreateRole,
		)

		iamGroup.PUT("/roles/:id",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE"),
			roleHandler.UpdateRole,
		)

		iamGroup.DELETE("/roles/:id",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "ADMIN"),
			roleHandler.DeleteRole,
		)

		// 角色策略管理
		iamGroup.POST("/roles/:id/policies",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE"),
			roleHandler.AddRolePolicy,
		)

		iamGroup.DELETE("/roles/:id/policies/:policy_id",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE"),
			roleHandler.RemoveRolePolicy,
		)

		// 角色克隆
		iamGroup.POST("/roles/:id/clone",
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE"),
			roleHandler.CloneRole,
		)
	}
}
