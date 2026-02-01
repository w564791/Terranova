package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupModuleRoutes sets up module routes
// 包括: modules, schemas, demos
func setupModuleRoutes(api *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// TODO: 实现module路由
	// 参考原router.go中的 modules := api.Group("/modules") 部分
	// 模块管理 - 使用IAM权限控制
	modules := api.Group("/modules")
	modules.Use(middleware.JWTAuth())
	modules.Use(middleware.AuditLogger(db))
	{
		moduleController := controllers.NewModuleController(services.NewModuleService(db))
		schemaController := controllers.NewSchemaController(services.NewSchemaService(db))
		demoController := controllers.NewModuleDemoController(db)

		// GET请求 - Admin绕过检查，非admin需要MODULES READ
		modules.GET("", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.GetModules(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleController.GetModules(c)
			}
		})

		modules.GET("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.GetModule(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleController.GetModule(c)
			}
		})

		modules.GET("/:id/files", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.GetModuleFiles(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleController.GetModuleFiles(c)
			}
		})

		modules.GET("/:id/schemas", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaController.GetSchemas(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				schemaController.GetSchemas(c)
			}
		})

		modules.GET("/:id/demos", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.GetDemosByModuleID(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				demoController.GetDemosByModuleID(c)
			}
		})

		modules.GET("/:id/prompts", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.GetModulePrompts(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleController.GetModulePrompts(c)
			}
		})

		// POST/PUT请求 - Admin绕过检查，非admin需要MODULES WRITE
		modules.POST("", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.CreateModule(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleController.CreateModule(c)
			}
		})

		modules.PUT("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.UpdateModule(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleController.UpdateModule(c)
			}
		})

		modules.PATCH("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.UpdateModule(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleController.UpdateModule(c)
			}
		})

		modules.POST("/:id/sync", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.SyncModuleFiles(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleController.SyncModuleFiles(c)
			}
		})

		modules.POST("/parse-tf", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.ParseTFFile(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleController.ParseTFFile(c)
			}
		})

		modules.POST("/:id/schemas", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaController.CreateSchema(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaController.CreateSchema(c)
			}
		})

		modules.POST("/:id/schemas/generate", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaController.GenerateSchemaFromModule(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaController.GenerateSchemaFromModule(c)
			}
		})

		modules.POST("/:id/demos", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.CreateDemo(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				demoController.CreateDemo(c)
			}
		})

		// DELETE请求 - Admin绕过检查，非admin需要MODULES ADMIN
		modules.DELETE("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleController.DeleteModule(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				moduleController.DeleteModule(c)
			}
		})

		// ========== V2 Schema API (OpenAPI 格式) ==========
		schemaV2Handler := handlers.NewModuleSchemaV2Handler(db)

		// 解析 Terraform variables.tf 并生成 OpenAPI Schema
		modules.POST("/parse-tf-v2", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.ParseTF(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaV2Handler.ParseTF(c)
			}
		})

		// 获取模块的 V2 Schema
		modules.GET("/:id/schemas/v2", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.GetSchemaV2(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				schemaV2Handler.GetSchemaV2(c)
			}
		})

		// 获取模块的所有 Schema（包括 v1 和 v2）
		modules.GET("/:id/schemas/all", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.GetAllSchemas(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				schemaV2Handler.GetAllSchemas(c)
			}
		})

		// 创建 V2 Schema
		modules.POST("/:id/schemas/v2", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.CreateSchemaV2(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaV2Handler.CreateSchemaV2(c)
			}
		})

		// 更新 V2 Schema
		modules.PUT("/:id/schemas/v2/:schemaId", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.UpdateSchemaV2(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaV2Handler.UpdateSchemaV2(c)
			}
		})

		// 更新单个字段的 UI 配置
		modules.PATCH("/:id/schemas/v2/:schemaId/fields", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.UpdateSchemaField(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaV2Handler.UpdateSchemaField(c)
			}
		})

		// 将 v1 Schema 迁移到 v2
		modules.POST("/:id/schemas/:schemaId/migrate-v2", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.MigrateToV2(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaV2Handler.MigrateToV2(c)
			}
		})

		// 对比两个 Schema 版本
		modules.GET("/:id/schemas/compare", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.CompareSchemas(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				schemaV2Handler.CompareSchemas(c)
			}
		})

		// 设置活跃版本
		modules.POST("/:id/schemas/:schemaId/activate", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.SetActiveSchema(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaV2Handler.SetActiveSchema(c)
			}
		})

		// 验证模块输入
		modules.POST("/:id/validate", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaV2Handler.ValidateModuleInput(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				schemaV2Handler.ValidateModuleInput(c)
			}
		})

		// ========== Module Version API (多版本管理) ==========
		moduleVersionController := controllers.NewModuleVersionController(db)

		// 获取模块的所有版本
		modules.GET("/:id/versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.ListVersions(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleVersionController.ListVersions(c)
			}
		})

		// 获取版本详情
		modules.GET("/:id/versions/:version_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.GetVersion(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleVersionController.GetVersion(c)
			}
		})

		// 获取默认版本
		modules.GET("/:id/default-version", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.GetDefaultVersion(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleVersionController.GetDefaultVersion(c)
			}
		})

		// 比较两个版本的 Schema 差异
		modules.GET("/:id/versions/compare", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.CompareVersions(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleVersionController.CompareVersions(c)
			}
		})

		// 获取版本的所有 Schema
		modules.GET("/:id/versions/:version_id/schemas", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.GetVersionSchemas(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleVersionController.GetVersionSchemas(c)
			}
		})

		// 获取版本的所有 Demo
		modules.GET("/:id/versions/:version_id/demos", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.GetVersionDemos(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				moduleVersionController.GetVersionDemos(c)
			}
		})

		// 创建新版本
		modules.POST("/:id/versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.CreateVersion(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleVersionController.CreateVersion(c)
			}
		})

		// 更新版本信息
		modules.PUT("/:id/versions/:version_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.UpdateVersion(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleVersionController.UpdateVersion(c)
			}
		})

		// 设置默认版本
		modules.PUT("/:id/default-version", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.SetDefaultVersion(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleVersionController.SetDefaultVersion(c)
			}
		})

		// 继承 Demos（创建版本时使用）
		modules.POST("/:id/versions/:version_id/inherit-demos", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.InheritDemos(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				moduleVersionController.InheritDemos(c)
			}
		})

		// 导入 Demos（从其他版本导入）
		modules.POST("/:id/versions/:version_id/import-demos", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.ImportDemos(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				demoController.ImportDemos(c)
			}
		})

		// 删除版本
		modules.DELETE("/:id/versions/:version_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.DeleteVersion(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				moduleVersionController.DeleteVersion(c)
			}
		})

		// 迁移现有模块数据到多版本结构（管理员操作）
		modules.POST("/migrate-versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				moduleVersionController.MigrateExistingModules(c)
				return
			}
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				moduleVersionController.MigrateExistingModules(c)
			}
		})
	}

}
