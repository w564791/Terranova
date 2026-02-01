package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupSecretRoutes 设置密文管理路由
// 通用的secrets API，支持多种资源类型（agent_pool, workspace, module等）
func setupSecretRoutes(r *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	secretHandler := handlers.NewSecretHandler(db)

	// 通用密文路由：/:resourceType/:resourceId/secrets
	// 例如：/agent_pool/pool-xyz123/secrets
	//      /workspace/ws-abc456/secrets
	secrets := r.Group("/:resourceType/:resourceId/secrets")
	{
		// 创建密文
		secrets.POST("", secretHandler.CreateSecret)

		// 列出密文
		secrets.GET("", secretHandler.ListSecrets)

		// 获取密文详情
		secrets.GET("/:secretId", secretHandler.GetSecret)

		// 更新密文（仅metadata）
		secrets.PUT("/:secretId", secretHandler.UpdateSecret)

		// 删除密文
		secrets.DELETE("/:secretId", secretHandler.DeleteSecret)
	}
}
