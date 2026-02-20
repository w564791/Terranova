package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupAuthRoutes sets up authentication routes
func setupAuthRoutes(api *gin.RouterGroup, db *gorm.DB) {
	// TODO: 实现auth路由
	// 参考原router.go中的:
	// - auth := api.Group("/auth")
	// - api.POST("/auth/refresh")
	// - api.GET("/auth/me")
}
