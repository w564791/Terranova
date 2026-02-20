package controllers

import (
	"iac-platform/services"

	"github.com/gin-gonic/gin"
)

// AIFormController AI 表单控制器
type AIFormController struct {
	service *services.AIFormService
}

// NewAIFormController 创建 AI 表单控制器实例
func NewAIFormController(service *services.AIFormService) *AIFormController {
	return &AIFormController{service: service}
}

// GenerateConfig 生成表单配置
// @Summary 生成表单配置
// @Description 根据用户描述和 Module Schema 生成表单配置
// @Tags AI
// @Accept json
// @Produce json
// @Param request body GenerateConfigRequest true "生成配置请求"
// @Success 200 {object} services.GenerateConfigResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/ai/form/generate [post]
func (c *AIFormController) GenerateConfig(ctx *gin.Context) {
	var req GenerateConfigRequest

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"code": 400, "error": "参数错误", "message": err.Error()})
		return
	}

	userID := ctx.GetString("user_id")
	if userID == "" {
		ctx.JSON(401, gin.H{"code": 401, "error": "未授权"})
		return
	}

	// 调用服务
	response, err := c.service.GenerateConfig(
		userID,
		req.ModuleID,
		req.UserDescription,
		req.ContextIDs.WorkspaceID,
		req.ContextIDs.OrganizationID,
		req.CurrentConfig,
		req.Mode,
	)

	if err != nil {
		ctx.JSON(500, gin.H{"code": 500, "error": err.Error()})
		return
	}

	ctx.JSON(200, gin.H{"code": 200, "data": response, "message": "Success"})
}

// GenerateConfigRequest 生成配置请求
type GenerateConfigRequest struct {
	ModuleID        uint                   `json:"module_id" binding:"required"`
	UserDescription string                 `json:"user_description" binding:"required,max=1000"`
	CurrentConfig   map[string]interface{} `json:"current_config,omitempty"` // 现有配置，用于修复模式
	Mode            string                 `json:"mode,omitempty"`           // 模式：new（新建）或 refine（修复）
	ContextIDs      struct {
		WorkspaceID    string `json:"workspace_id,omitempty"`
		OrganizationID string `json:"organization_id,omitempty"`
		ManifestID     string `json:"manifest_id,omitempty"`
	} `json:"context_ids,omitempty"`
}
