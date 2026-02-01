package router

import (
	"iac-platform/internal/application/service"
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupUserRoutes sets up user routes
func setupUserRoutes(protected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	user := protected.Group("/user")
	{
		authHandler := handlers.NewAuthHandler(db)

		// 管理员重置用户密码
		user.POST("/reset-password", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				authHandler.ResetPassword(c)
				return
			}
			iamMiddleware.RequirePermission("USER_MANAGEMENT", "USER", "WRITE")(c)
			if !c.IsAborted() {
				authHandler.ResetPassword(c)
			}
		})

		// 用户个人设置相关路由
		// 使用统一的JWT密钥配置
		userTokenService := service.NewUserTokenService(db, "")
		userTokenHandler := handlers.NewUserTokenHandler(userTokenService, db)

		// 用户修改自己的密码
		user.POST("/change-password", userTokenHandler.ChangePassword)

		// 用户Token管理
		user.POST("/tokens", userTokenHandler.CreateUserToken)
		user.GET("/tokens", userTokenHandler.ListUserTokens)
		user.DELETE("/tokens/:token_name", userTokenHandler.RevokeUserToken)
	}
}
