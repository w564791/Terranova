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
		iamGroup.POST("/permissions/grant", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.GrantPermission(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				permissionHandler.GrantPermission(c)
			}
		})

		iamGroup.POST("/permissions/batch-grant", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.BatchGrantPermissions(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				permissionHandler.BatchGrantPermissions(c)
			}
		})

		iamGroup.POST("/permissions/grant-preset", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.GrantPresetPermissions(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				permissionHandler.GrantPresetPermissions(c)
			}
		})

		iamGroup.DELETE("/permissions/:scope_type/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.RevokePermission(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				permissionHandler.RevokePermission(c)
			}
		})

		iamGroup.GET("/permissions/:scope_type/:scope_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.ListPermissions(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				permissionHandler.ListPermissions(c)
			}
		})

		iamGroup.GET("/permissions/definitions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.ListPermissionDefinitions(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				permissionHandler.ListPermissionDefinitions(c)
			}
		})

		// 新增：按主体查询权限的API
		iamGroup.GET("/users/:id/permissions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.ListUserPermissions(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				permissionHandler.ListUserPermissions(c)
			}
		})

		iamGroup.GET("/teams/:id/permissions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				permissionHandler.ListTeamPermissions(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PERMISSIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				permissionHandler.ListTeamPermissions(c)
			}
		})

		// 团队管理 - 添加IAM权限检查
		iamGroup.POST("/teams", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamHandler.CreateTeam(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				teamHandler.CreateTeam(c)
			}
		})

		iamGroup.GET("/teams", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamHandler.ListTeamsByOrg(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				teamHandler.ListTeamsByOrg(c)
			}
		})

		iamGroup.GET("/teams/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamHandler.GetTeam(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				teamHandler.GetTeam(c)
			}
		})

		iamGroup.DELETE("/teams/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamHandler.DeleteTeam(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				teamHandler.DeleteTeam(c)
			}
		})

		iamGroup.POST("/teams/:id/members", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamHandler.AddTeamMember(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				teamHandler.AddTeamMember(c)
			}
		})

		iamGroup.DELETE("/teams/:id/members/:user_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamHandler.RemoveTeamMember(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				teamHandler.RemoveTeamMember(c)
			}
		})

		iamGroup.GET("/teams/:id/members", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamHandler.ListTeamMembers(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				teamHandler.ListTeamMembers(c)
			}
		})

		// 团队Token管理 - 使用统一JWT密钥
		teamTokenHandler := handlers.NewTeamTokenHandler(service.NewTeamTokenService(db, ""))

		iamGroup.POST("/teams/:id/tokens", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamTokenHandler.CreateTeamToken(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				teamTokenHandler.CreateTeamToken(c)
			}
		})

		iamGroup.GET("/teams/:id/tokens", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamTokenHandler.ListTeamTokens(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				teamTokenHandler.ListTeamTokens(c)
			}
		})

		iamGroup.DELETE("/teams/:id/tokens/:token_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				teamTokenHandler.RevokeTeamToken(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				teamTokenHandler.RevokeTeamToken(c)
			}
		})

		// 团队角色管理 - 添加IAM权限检查
		iamGroup.POST("/teams/:id/roles", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.AssignTeamRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				roleHandler.AssignTeamRole(c)
			}
		})

		iamGroup.GET("/teams/:id/roles", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.ListTeamRoles(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				roleHandler.ListTeamRoles(c)
			}
		})

		iamGroup.DELETE("/teams/:id/roles/:assignment_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.RevokeTeamRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_TEAMS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				roleHandler.RevokeTeamRole(c)
			}
		})

		// 组织管理 - 添加IAM权限检查
		iamGroup.POST("/organizations", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.CreateOrganization(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				orgHandler.CreateOrganization(c)
			}
		})

		iamGroup.GET("/organizations", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.ListOrganizations(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				orgHandler.ListOrganizations(c)
			}
		})

		iamGroup.GET("/organizations/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.GetOrganization(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				orgHandler.GetOrganization(c)
			}
		})

		iamGroup.PUT("/organizations/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.UpdateOrganization(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				orgHandler.UpdateOrganization(c)
			}
		})

		iamGroup.DELETE("/organizations/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.DeleteOrganization(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ORGANIZATIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				orgHandler.DeleteOrganization(c)
			}
		})

		// 项目管理 - 添加IAM权限检查
		iamGroup.POST("/projects", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.CreateProject(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				orgHandler.CreateProject(c)
			}
		})

		iamGroup.GET("/projects", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.ListProjects(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				orgHandler.ListProjects(c)
			}
		})

		iamGroup.GET("/projects/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.GetProject(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				orgHandler.GetProject(c)
			}
		})

		iamGroup.PUT("/projects/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.UpdateProject(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				orgHandler.UpdateProject(c)
			}
		})

		iamGroup.DELETE("/projects/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				orgHandler.DeleteProject(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_PROJECTS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				orgHandler.DeleteProject(c)
			}
		})

		// 应用管理 - 添加IAM权限检查
		iamGroup.POST("/applications", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				applicationHandler.CreateApplication(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				applicationHandler.CreateApplication(c)
			}
		})

		iamGroup.GET("/applications", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				applicationHandler.ListApplications(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				applicationHandler.ListApplications(c)
			}
		})

		iamGroup.GET("/applications/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				applicationHandler.GetApplication(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				applicationHandler.GetApplication(c)
			}
		})

		iamGroup.PUT("/applications/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				applicationHandler.UpdateApplication(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				applicationHandler.UpdateApplication(c)
			}
		})

		iamGroup.DELETE("/applications/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				applicationHandler.DeleteApplication(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				applicationHandler.DeleteApplication(c)
			}
		})

		iamGroup.POST("/applications/:id/regenerate-secret", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				applicationHandler.RegenerateSecret(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_APPLICATIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				applicationHandler.RegenerateSecret(c)
			}
		})

		// 审计日志 - 添加IAM权限检查
		iamGroup.GET("/audit/config", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				auditConfigHandler.GetAuditConfig(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				auditConfigHandler.GetAuditConfig(c)
			}
		})

		iamGroup.PUT("/audit/config", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				auditConfigHandler.UpdateAuditConfig(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				auditConfigHandler.UpdateAuditConfig(c)
			}
		})

		iamGroup.GET("/audit/permission-history", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				auditHandler.QueryPermissionHistory(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				auditHandler.QueryPermissionHistory(c)
			}
		})

		iamGroup.GET("/audit/access-history", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				auditHandler.QueryAccessHistory(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				auditHandler.QueryAccessHistory(c)
			}
		})

		iamGroup.GET("/audit/denied-access", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				auditHandler.QueryDeniedAccess(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				auditHandler.QueryDeniedAccess(c)
			}
		})

		iamGroup.GET("/audit/permission-changes-by-principal", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				auditHandler.QueryPermissionChangesByPrincipal(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				auditHandler.QueryPermissionChangesByPrincipal(c)
			}
		})

		iamGroup.GET("/audit/permission-changes-by-performer", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				auditHandler.QueryPermissionChangesByPerformer(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_AUDIT", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				auditHandler.QueryPermissionChangesByPerformer(c)
			}
		})

		// 用户管理 - 添加IAM权限检查
		iamGroup.GET("/users/stats", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.GetUserStats(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				userHandler.GetUserStats(c)
			}
		})

		iamGroup.GET("/users", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.ListUsers(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				userHandler.ListUsers(c)
			}
		})

		iamGroup.POST("/users", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.CreateUser(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				userHandler.CreateUser(c)
			}
		})

		// 用户角色分配（使用 /users/:id/roles 路径）
		iamGroup.POST("/users/:id/roles", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.AssignRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				roleHandler.AssignRole(c)
			}
		})

		iamGroup.DELETE("/users/:id/roles/:assignment_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.RevokeRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				roleHandler.RevokeRole(c)
			}
		})

		iamGroup.GET("/users/:id/roles", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.ListUserRoles(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				roleHandler.ListUserRoles(c)
			}
		})

		// 用户基本操作
		iamGroup.GET("/users/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.GetUser(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				userHandler.GetUser(c)
			}
		})

		iamGroup.PUT("/users/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.UpdateUser(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				userHandler.UpdateUser(c)
			}
		})

		iamGroup.POST("/users/:id/activate", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.ActivateUser(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				userHandler.ActivateUser(c)
			}
		})

		iamGroup.POST("/users/:id/deactivate", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.DeactivateUser(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				userHandler.DeactivateUser(c)
			}
		})

		iamGroup.DELETE("/users/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				userHandler.DeleteUser(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_USERS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				userHandler.DeleteUser(c)
			}
		})

		// 角色管理 - 添加IAM权限检查
		iamGroup.GET("/roles", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.ListRoles(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				roleHandler.ListRoles(c)
			}
		})

		iamGroup.GET("/roles/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.GetRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				roleHandler.GetRole(c)
			}
		})

		iamGroup.POST("/roles", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.CreateRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				roleHandler.CreateRole(c)
			}
		})

		iamGroup.PUT("/roles/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.UpdateRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				roleHandler.UpdateRole(c)
			}
		})

		iamGroup.DELETE("/roles/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.DeleteRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				roleHandler.DeleteRole(c)
			}
		})

		// 角色策略管理
		iamGroup.POST("/roles/:id/policies", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.AddRolePolicy(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				roleHandler.AddRolePolicy(c)
			}
		})

		iamGroup.DELETE("/roles/:id/policies/:policy_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.RemoveRolePolicy(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				roleHandler.RemoveRolePolicy(c)
			}
		})

		// 角色克隆
		iamGroup.POST("/roles/:id/clone", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				roleHandler.CloneRole(c)
				return
			}
			iamMiddleware.RequirePermission("IAM_ROLES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				roleHandler.CloneRole(c)
			}
		})
	}
}
