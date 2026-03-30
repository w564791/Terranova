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
// @Summary Get module version skill
// @Description Get the skill associated with a module version
// @Tags Module Version Skill
// @Accept json
// @Produce json
// @Param id path string true "Module Version ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/module-versions/{id}/skill [get]
// @Security BearerAuth
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
// @Summary Generate skill from schema
// @Description Generate a skill from the module version's schema
// @Tags Module Version Skill
// @Accept json
// @Produce json
// @Param id path string true "Module Version ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/module-versions/{id}/skill/generate [post]
// @Security BearerAuth
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
// @Summary Update skill custom content
// @Description Update the custom content of a module version skill
// @Tags Module Version Skill
// @Accept json
// @Produce json
// @Param id path string true "Module Version ID"
// @Param request body models.UpdateCustomContentRequest true "Custom content update"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/module-versions/{id}/skill [put]
// @Security BearerAuth
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
// @Summary Inherit skill from another version
// @Description Inherit the skill from another module version
// @Tags Module Version Skill
// @Accept json
// @Produce json
// @Param id path string true "Target Module Version ID"
// @Param request body models.InheritSkillRequest true "Inherit request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/module-versions/{id}/skill/inherit [post]
// @Security BearerAuth
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
// @Summary Delete module version skill
// @Description Delete the skill associated with a module version
// @Tags Module Version Skill
// @Accept json
// @Produce json
// @Param id path string true "Module Version ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/module-versions/{id}/skill [delete]
// @Security BearerAuth
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
