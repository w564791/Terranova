package controllers

import (
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ModuleVersionSkillController Module 版本 Skill 控制器
type ModuleVersionSkillController struct {
	db      *gorm.DB
	service *services.ModuleVersionSkillService
}

// NewModuleVersionSkillController 创建控制器实例
func NewModuleVersionSkillController(db *gorm.DB) *ModuleVersionSkillController {
	return &ModuleVersionSkillController{
		db:      db,
		service: services.NewModuleVersionSkillService(db),
	}
}

// GetSkill 获取版本的 Skill
// GET /api/v1/module-versions/:id/skill
func (c *ModuleVersionSkillController) GetSkill(ctx *gin.Context) {
	versionID := ctx.Param("id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "版本 ID 不能为空"})
		return
	}

	skill, err := c.service.GetByVersionID(versionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if skill == nil {
		// 返回空的 Skill 结构
		ctx.JSON(http.StatusOK, gin.H{
			"id":                       "",
			"module_version_id":        versionID,
			"schema_generated_content": "",
			"custom_content":           "",
			"combined_content":         "",
			"is_active":                true,
		})
		return
	}

	ctx.JSON(http.StatusOK, skill.ToResponse())
}

// GenerateFromSchema 根据 Schema 生成 Skill
// POST /api/v1/module-versions/:id/skill/generate
func (c *ModuleVersionSkillController) GenerateFromSchema(ctx *gin.Context) {
	versionID := ctx.Param("id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "版本 ID 不能为空"})
		return
	}

	// 获取当前用户 ID
	userID := getUserIDFromContext(ctx)

	skill, err := c.service.GenerateFromSchema(versionID, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, skill.ToResponse())
}

// UpdateCustomContent 更新自定义内容
// PUT /api/v1/module-versions/:id/skill
func (c *ModuleVersionSkillController) UpdateCustomContent(ctx *gin.Context) {
	versionID := ctx.Param("id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "版本 ID 不能为空"})
		return
	}

	var req models.UpdateCustomContentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取当前用户 ID
	userID := getUserIDFromContext(ctx)

	skill, err := c.service.UpdateCustomContent(versionID, req.CustomContent, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, skill.ToResponse())
}

// InheritFromVersion 从其他版本继承 Skill
// POST /api/v1/module-versions/:id/skill/inherit
func (c *ModuleVersionSkillController) InheritFromVersion(ctx *gin.Context) {
	versionID := ctx.Param("id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "版本 ID 不能为空"})
		return
	}

	var req models.InheritSkillRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取当前用户 ID
	userID := getUserIDFromContext(ctx)

	skill, err := c.service.InheritFromVersion(versionID, req.FromVersionID, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, skill.ToResponse())
}

// DeleteSkill 删除 Skill
// DELETE /api/v1/module-versions/:id/skill
func (c *ModuleVersionSkillController) DeleteSkill(ctx *gin.Context) {
	versionID := ctx.Param("id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "版本 ID 不能为空"})
		return
	}

	if err := c.service.Delete(versionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// getUserIDFromContext 从上下文获取用户 ID
func getUserIDFromContext(ctx *gin.Context) string {
	// 尝试从上下文获取用户 ID
	if userID, exists := ctx.Get("user_id"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	return ""
}
