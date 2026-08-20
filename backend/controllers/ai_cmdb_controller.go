package controllers

import (
	"iac-platform/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AICMDBController AI + CMDB 集成控制器
type AICMDBController struct {
	db      *gorm.DB
	service *services.AICMDBService
}

// NewAICMDBController 创建 AI + CMDB 控制器实例
func NewAICMDBController(db *gorm.DB) *AICMDBController {
	return &AICMDBController{
		db:      db,
		service: services.NewAICMDBService(db),
	}
}

// GenerateConfigWithCMDB 带 CMDB 查询的配置生成
// @Summary Generate config with CMDB lookup
// @Description Automatically query CMDB resources based on user description and generate Terraform configuration
// @Tags AI Form
// @Accept json
// @Produce json
// @Param request body services.GenerateConfigWithCMDBRequest true "Request parameters"
// @Success 200 {object} services.GenerateConfigWithCMDBResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/ai/form/generate-with-cmdb [post]
func (c *AICMDBController) GenerateConfigWithCMDB(ctx *gin.Context) {
	// 获取用户 ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "未授权",
		})
		return
	}

	// 解析请求
	var req services.GenerateConfigWithCMDBRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	tenantScope, err := resolveAICMDBTenantScope(
		ctx,
		c.db,
		req.ContextIDs.OrganizationID,
		req.ContextIDs.WorkspaceID,
	)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求上下文不属于当前组织",
		})
		return
	}

	// 调用服务
	response, err := c.service.GenerateConfigWithCMDB(
		userID.(string),
		req.ModuleID,
		req.UserDescription,
		tenantScope.workspaceID,
		tenantScope.organizationID,
		tenantScope.cmdbScope,
		req.UserSelections,
		req.CurrentConfig,
		req.Mode,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "配置生成失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": response,
	})
}
