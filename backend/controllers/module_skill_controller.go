package controllers

import (
	"fmt"
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ModuleSkillController Module Skill 控制器
type ModuleSkillController struct {
	db             *gorm.DB
	skillGenerator *services.ModuleSkillGenerator
	skillAssembler *services.SkillAssembler
}

// NewModuleSkillController 创建控制器实例
func NewModuleSkillController(db *gorm.DB) *ModuleSkillController {
	return &ModuleSkillController{
		db:             db,
		skillGenerator: services.NewModuleSkillGenerator(db),
		skillAssembler: services.NewSkillAssembler(db),
	}
}

// GetModuleSkill 获取 Module 的 Skill
// @Summary 获取 Module 的 Skill
// @Description 获取指定 Module 关联的 Skill，如果不存在则返回 404
// @Tags Module Skill
// @Accept json
// @Produce json
// @Param module_id path int true "Module ID"
// @Success 200 {object} models.Skill
// @Failure 404 {object} map[string]string
// @Router /api/v1/modules/{module_id}/skill [get]
func (c *ModuleSkillController) GetModuleSkill(ctx *gin.Context) {
	moduleIDStr := ctx.Param("module_id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 Module ID",
		})
		return
	}

	// 查找 Module 关联的 Skill
	var skill models.Skill
	if err := c.db.Where("source_module_id = ?", moduleID).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error":   "Module 没有关联的 Skill",
				"message": "可以使用 POST /modules/{module_id}/skill/generate 生成",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	ctx.JSON(http.StatusOK, skill)
}

// GenerateModuleSkill 生成 Module Skill
// @Summary 生成 Module Skill
// @Description 根据 Module 的 Schema 和 Demo 自动生成 Skill
// @Tags Module Skill
// @Accept json
// @Produce json
// @Param module_id path int true "Module ID"
// @Param force query bool false "是否强制重新生成"
// @Success 200 {object} models.Skill
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{module_id}/skill/generate [post]
func (c *ModuleSkillController) GenerateModuleSkill(ctx *gin.Context) {
	moduleIDStr := ctx.Param("module_id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 Module ID",
		})
		return
	}

	force := ctx.Query("force") == "true"

	// 检查 Module 是否存在
	var module models.Module
	if err := c.db.First(&module, moduleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Module 不存在",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	// 检查是否已存在 Skill
	var existingSkill models.Skill
	skillExists := c.db.Where("source_module_id = ?", moduleID).First(&existingSkill).Error == nil

	if skillExists && !force {
		ctx.JSON(http.StatusOK, gin.H{
			"message": "Skill 已存在，使用 force=true 强制重新生成",
			"skill":   existingSkill,
		})
		return
	}

	// 生成 Skill
	skill, err := c.skillAssembler.GetOrGenerateModuleSkill(uint(moduleID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "生成 Skill 失败",
			"details": err.Error(),
		})
		return
	}

	if skill == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无法生成 Skill，Module 可能没有活跃的 Schema",
		})
		return
	}

	ctx.JSON(http.StatusOK, skill)
}

// PreviewModuleSkill 预览 Module Skill 内容
// @Summary 预览 Module Skill 内容
// @Description 预览将要生成的 Skill 内容，不保存到数据库
// @Tags Module Skill
// @Accept json
// @Produce json
// @Param module_id path int true "Module ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Router /api/v1/modules/{module_id}/skill/preview [get]
func (c *ModuleSkillController) PreviewModuleSkill(ctx *gin.Context) {
	moduleIDStr := ctx.Param("module_id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 Module ID",
		})
		return
	}

	// 预览 Skill 内容
	content, err := c.skillGenerator.PreviewSkillContent(uint(moduleID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "预览失败",
			"details": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"module_id": moduleID,
		"content":   content,
	})
}

// UpdateModuleSkillRequest 更新 Module Skill 请求
type UpdateModuleSkillRequest struct {
	Content *string `json:"content,omitempty"`
}

// UpdateModuleSkill 更新 Module Skill
// @Summary 更新 Module Skill
// @Description 更新 Module 关联的 Skill 内容，更新后 source_type 变为 hybrid
// @Tags Module Skill
// @Accept json
// @Produce json
// @Param module_id path int true "Module ID"
// @Param request body UpdateModuleSkillRequest true "更新内容"
// @Success 200 {object} models.Skill
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/v1/modules/{module_id}/skill [put]
func (c *ModuleSkillController) UpdateModuleSkill(ctx *gin.Context) {
	moduleIDStr := ctx.Param("module_id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 Module ID",
		})
		return
	}

	var req UpdateModuleSkillRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	// 查找 Module 关联的 Skill
	var skill models.Skill
	if err := c.db.Where("source_module_id = ?", moduleID).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Module 没有关联的 Skill，请先生成",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	// 更新内容
	if req.Content != nil {
		skill.Content = *req.Content
		// 标记为 hybrid（自动生成后手动修改）
		if skill.SourceType == models.SkillSourceModuleAuto {
			skill.SourceType = models.SkillSourceHybrid
		}
	}

	if err := c.db.Save(&skill).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "更新失败",
			"details": err.Error(),
		})
		return
	}

	// 清除缓存
	c.skillAssembler.ClearCache()

	ctx.JSON(http.StatusOK, skill)
}

// DeleteModuleSkill 删除 Module Skill
// @Summary 删除 Module Skill
// @Description 删除 Module 关联的 Skill
// @Tags Module Skill
// @Accept json
// @Produce json
// @Param module_id path int true "Module ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/v1/modules/{module_id}/skill [delete]
func (c *ModuleSkillController) DeleteModuleSkill(ctx *gin.Context) {
	moduleIDStr := ctx.Param("module_id")
	moduleID, err := strconv.ParseUint(moduleIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的 Module ID",
		})
		return
	}

	// 查找并删除 Module 关联的 Skill
	result := c.db.Where("source_module_id = ?", moduleID).Delete(&models.Skill{})
	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "删除失败",
		})
		return
	}

	if result.RowsAffected == 0 {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "Module 没有关联的 Skill",
		})
		return
	}

	// 清除缓存
	c.skillAssembler.ClearCache()

	ctx.JSON(http.StatusOK, gin.H{
		"message": "删除成功",
	})
}

// BatchGenerateModuleSkills 批量生成 Module Skills
// @Summary 批量生成 Module Skills
// @Description 为所有有活跃 Schema 的 Module 生成 Skill
// @Tags Module Skill
// @Accept json
// @Produce json
// @Param force query bool false "是否强制重新生成已存在的"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/admin/modules/skills/batch-generate [post]
func (c *ModuleSkillController) BatchGenerateModuleSkills(ctx *gin.Context) {
	force := ctx.Query("force") == "true"

	// 获取所有有活跃 Schema 的 Module
	var modules []models.Module
	if err := c.db.Joins("JOIN schemas ON schemas.module_id = modules.id AND schemas.status = 'active'").
		Group("modules.id").
		Find(&modules).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询 Module 失败",
		})
		return
	}

	var generated, skipped, failed int
	var errors []string

	for _, module := range modules {
		// 检查是否已存在 Skill
		var existingSkill models.Skill
		skillExists := c.db.Where("source_module_id = ?", module.ID).First(&existingSkill).Error == nil

		if skillExists && !force {
			skipped++
			continue
		}

		// 生成 Skill
		skill, err := c.skillGenerator.GenerateSkillFromModule(module.ID)
		if err != nil {
			failed++
			errors = append(errors, module.Name+": "+err.Error())
			continue
		}

		if skill == nil {
			skipped++
			continue
		}

		// 保存或更新
		if skillExists {
			existingSkill.Content = skill.Content
			existingSkill.Version = incrementSkillVersion(existingSkill.Version)
			if err := c.db.Save(&existingSkill).Error; err != nil {
				failed++
				errors = append(errors, module.Name+": 保存失败")
				continue
			}
		} else {
			skill.ID = uuid.New().String()
			if err := c.db.Create(skill).Error; err != nil {
				failed++
				errors = append(errors, module.Name+": 创建失败")
				continue
			}
		}
		generated++
	}

	// 清除缓存
	c.skillAssembler.ClearCache()

	ctx.JSON(http.StatusOK, gin.H{
		"total_modules": len(modules),
		"generated":     generated,
		"skipped":       skipped,
		"failed":        failed,
		"errors":        errors,
	})
}

// incrementSkillVersion 递增版本号
func incrementSkillVersion(version string) string {
	// 简单实现：1.0.0 -> 1.0.1
	parts := []int{1, 0, 0}
	n, _ := fmt.Sscanf(version, "%d.%d.%d", &parts[0], &parts[1], &parts[2])
	if n == 3 {
		parts[2]++
		return strconv.Itoa(parts[0]) + "." + strconv.Itoa(parts[1]) + "." + strconv.Itoa(parts[2])
	}
	return "1.0.1"
}
