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
		modules.GET("",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleController.GetModules,
		)

		modules.GET("/:id",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleController.GetModule,
		)

		modules.GET("/:id/files",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleController.GetModuleFiles,
		)

		modules.GET("/:id/schemas",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			schemaController.GetSchemas,
		)

		modules.GET("/:id/demos",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			demoController.GetDemosByModuleID,
		)

		modules.GET("/:id/prompts",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleController.GetModulePrompts,
		)

		// POST/PUT请求 - Admin绕过检查，非admin需要MODULES WRITE
		modules.POST("",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleController.CreateModule,
		)

		modules.PUT("/:id",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleController.UpdateModule,
		)

		modules.PATCH("/:id",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleController.UpdateModule,
		)

		modules.POST("/:id/sync",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleController.SyncModuleFiles,
		)

		modules.POST("/parse-tf",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleController.ParseTFFile,
		)

		modules.POST("/:id/schemas",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaController.CreateSchema,
		)

		modules.POST("/:id/schemas/generate",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaController.GenerateSchemaFromModule,
		)

		modules.POST("/:id/demos",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			demoController.CreateDemo,
		)

		// DELETE请求 - Admin绕过检查，非admin需要MODULES ADMIN
		modules.DELETE("/:id",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "ADMIN"),
			moduleController.DeleteModule,
		)

		// ========== V2 Schema API (OpenAPI 格式) ==========
		schemaV2Handler := handlers.NewModuleSchemaV2Handler(db)

		// 解析 Terraform variables.tf 并生成 OpenAPI Schema
		modules.POST("/parse-tf-v2",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaV2Handler.ParseTF,
		)

		// 获取模块的 V2 Schema
		modules.GET("/:id/schemas/v2",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			schemaV2Handler.GetSchemaV2,
		)

		// 获取模块的所有 Schema（包括 v1 和 v2）
		modules.GET("/:id/schemas/all",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			schemaV2Handler.GetAllSchemas,
		)

		// 创建 V2 Schema
		modules.POST("/:id/schemas/v2",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaV2Handler.CreateSchemaV2,
		)

		// 更新 V2 Schema
		modules.PUT("/:id/schemas/v2/:schemaId",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaV2Handler.UpdateSchemaV2,
		)

		// 更新单个字段的 UI 配置
		modules.PATCH("/:id/schemas/v2/:schemaId/fields",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaV2Handler.UpdateSchemaField,
		)

		// 将 v1 Schema 迁移到 v2
		modules.POST("/:id/schemas/:schemaId/migrate-v2",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaV2Handler.MigrateToV2,
		)

		// 对比两个 Schema 版本
		modules.GET("/:id/schemas/compare",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			schemaV2Handler.CompareSchemas,
		)

		// 设置活跃版本
		modules.POST("/:id/schemas/:schemaId/activate",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			schemaV2Handler.SetActiveSchema,
		)

		// 验证模块输入
		modules.POST("/:id/validate",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			schemaV2Handler.ValidateModuleInput,
		)

		// ========== Module Version API (多版本管理) ==========
		moduleVersionController := controllers.NewModuleVersionController(db)

		// 获取模块的所有版本
		modules.GET("/:id/versions",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleVersionController.ListVersions,
		)

		// 获取版本详情
		modules.GET("/:id/versions/:version_id",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleVersionController.GetVersion,
		)

		// 获取默认版本
		modules.GET("/:id/default-version",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleVersionController.GetDefaultVersion,
		)

		// 比较两个版本的 Schema 差异
		modules.GET("/:id/versions/compare",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleVersionController.CompareVersions,
		)

		// 获取版本的所有 Schema
		modules.GET("/:id/versions/:version_id/schemas",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleVersionController.GetVersionSchemas,
		)

		// 获取版本的所有 Demo
		modules.GET("/:id/versions/:version_id/demos",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			moduleVersionController.GetVersionDemos,
		)

		// 创建新版本
		modules.POST("/:id/versions",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleVersionController.CreateVersion,
		)

		// 更新版本信息
		modules.PUT("/:id/versions/:version_id",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleVersionController.UpdateVersion,
		)

		// 设置默认版本
		modules.PUT("/:id/default-version",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleVersionController.SetDefaultVersion,
		)

		// 继承 Demos（创建版本时使用）
		modules.POST("/:id/versions/:version_id/inherit-demos",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			moduleVersionController.InheritDemos,
		)

		// 导入 Demos（从其他版本导入）
		modules.POST("/:id/versions/:version_id/import-demos",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "WRITE"),
			demoController.ImportDemos,
		)

		// 删除版本
		modules.DELETE("/:id/versions/:version_id",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "ADMIN"),
			moduleVersionController.DeleteVersion,
		)

		// 迁移现有模块数据到多版本结构（管理员操作）
		modules.POST("/migrate-versions",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "ADMIN"),
			moduleVersionController.MigrateExistingModules,
		)
	}

}
