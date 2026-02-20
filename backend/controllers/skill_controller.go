package controllers

import (
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SkillController Skill 管理控制器
type SkillController struct {
	db             *gorm.DB
	skillAssembler *services.SkillAssembler
}

// NewSkillController 创建控制器实例
func NewSkillController(db *gorm.DB) *SkillController {
	return &SkillController{
		db:             db,
		skillAssembler: services.NewSkillAssembler(db),
	}
}

// ListSkills 获取 Skill 列表
// @Summary 获取 Skill 列表
// @Description 分页获取 Skill 列表，支持按层级、状态过滤和关键词搜索
// @Tags Skill
// @Accept json
// @Produce json
// @Param layer query string false "层级过滤: foundation, domain, task"
// @Param is_active query bool false "是否激活"
// @Param source_type query string false "来源类型: manual, module_auto, hybrid"
// @Param search query string false "搜索关键词（匹配名称、显示名称、内容）"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} models.SkillListResponse
// @Router /api/v1/admin/skills [get]
func (c *SkillController) ListSkills(ctx *gin.Context) {
	// 解析查询参数
	layer := ctx.Query("layer")
	isActiveStr := ctx.Query("is_active")
	sourceType := ctx.Query("source_type")
	search := ctx.Query("search")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询
	query := c.db.Model(&models.Skill{})

	if layer != "" {
		query = query.Where("layer = ?", layer)
	}
	if isActiveStr != "" {
		isActive := isActiveStr == "true"
		query = query.Where("is_active = ?", isActive)
	}
	if sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	// 搜索功能：匹配名称、显示名称或内容
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where("name ILIKE ? OR display_name ILIKE ? OR content ILIKE ?", searchPattern, searchPattern, searchPattern)
	}

	// 获取总数
	var total int64
	query.Count(&total)

	// 分页查询
	var skills []models.Skill
	offset := (page - 1) * pageSize
	if err := query.Order("layer ASC, priority ASC, name ASC").
		Offset(offset).Limit(pageSize).
		Find(&skills).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	// 计算总页数
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}

	ctx.JSON(http.StatusOK, models.SkillListResponse{
		Skills:     skills,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// GetSkill 获取单个 Skill
// @Summary 获取单个 Skill
// @Description 根据 ID 或名称获取 Skill 详情
// @Tags Skill
// @Accept json
// @Produce json
// @Param id path string true "Skill ID 或名称"
// @Success 200 {object} models.Skill
// @Failure 404 {object} map[string]string
// @Router /api/v1/admin/skills/{id} [get]
func (c *SkillController) GetSkill(ctx *gin.Context) {
	idOrName := ctx.Param("id")

	var skill models.Skill
	// 先尝试按 ID 查询，再按名称查询
	if err := c.db.Where("id = ? OR name = ?", idOrName, idOrName).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Skill 不存在",
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

// CreateSkill 创建 Skill
// @Summary 创建 Skill
// @Description 创建新的 Skill
// @Tags Skill
// @Accept json
// @Produce json
// @Param request body models.CreateSkillRequest true "Skill 信息"
// @Success 201 {object} models.Skill
// @Failure 400 {object} map[string]string
// @Router /api/v1/admin/skills [post]
func (c *SkillController) CreateSkill(ctx *gin.Context) {
	var req models.CreateSkillRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	// 检查名称是否已存在
	var existing models.Skill
	if err := c.db.Where("name = ?", req.Name).First(&existing).Error; err == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Skill 名称已存在",
		})
		return
	}

	// 获取创建者 ID
	userID, _ := ctx.Get("user_id")

	// 设置默认值
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = models.SkillSourceManual
	}
	version := req.Version
	if version == "" {
		version = "1.0.0"
	}

	// 创建 Skill
	skill := models.Skill{
		ID:          uuid.New().String(),
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description, // 添加 Description 字段
		Layer:       req.Layer,
		Content:     req.Content,
		Version:     version,
		IsActive:    true,
		Priority:    req.Priority,
		SourceType:  sourceType,
		CreatedBy:   userID.(string),
	}

	if req.Metadata != nil {
		skill.Metadata = *req.Metadata
	}

	if err := c.db.Create(&skill).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建失败",
			"details": err.Error(),
		})
		return
	}

	// 清除缓存
	c.skillAssembler.ClearCache()

	ctx.JSON(http.StatusCreated, skill)
}

// UpdateSkill 更新 Skill
// @Summary 更新 Skill
// @Description 更新 Skill 信息
// @Tags Skill
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param request body models.UpdateSkillRequest true "更新信息"
// @Success 200 {object} models.Skill
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /api/v1/admin/skills/{id} [put]
func (c *SkillController) UpdateSkill(ctx *gin.Context) {
	skillID := ctx.Param("id")

	var skill models.Skill
	if err := c.db.Where("id = ?", skillID).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Skill 不存在",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	var req models.UpdateSkillRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	// 更新字段
	if req.DisplayName != nil {
		skill.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		skill.Description = *req.Description
	}
	if req.Content != nil {
		skill.Content = *req.Content
		// 如果是 Module 自动生成的，更新后变为 hybrid
		if skill.SourceType == models.SkillSourceModuleAuto {
			skill.SourceType = models.SkillSourceHybrid
		}
	}
	if req.Version != nil {
		skill.Version = *req.Version
	}
	if req.IsActive != nil {
		skill.IsActive = *req.IsActive
	}
	if req.Priority != nil {
		skill.Priority = *req.Priority
	}
	if req.Metadata != nil {
		skill.Metadata = *req.Metadata
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

// DeactivateSkill 停用 Skill
// @Summary 停用 Skill
// @Description 停用指定的 Skill（软删除）
// @Tags AI Skills
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/ai/skills/{id}/deactivate [post]
func (c *SkillController) DeactivateSkill(ctx *gin.Context) {
	skillID := ctx.Param("id")

	var skill models.Skill
	if err := c.db.First(&skill, "id = ?", skillID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Skill 不存在"})
		return
	}

	// 更新为非活跃状态
	if err := c.db.Model(&skill).Update("is_active", false).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "停用 Skill 失败"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Skill 已停用",
		"skill":   skill,
	})
}

// DeleteSkill 删除 Skill
// @Summary 删除 Skill
// @Description 删除指定的 Skill
// @Tags AI Skills
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/ai/skills/{id} [delete]
func (c *SkillController) DeleteSkill(ctx *gin.Context) {
	skillID := ctx.Param("id")
	hardDelete := ctx.Query("hard") == "true"

	var skill models.Skill
	if err := c.db.Where("id = ?", skillID).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Skill 不存在",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	if hardDelete {
		// 硬删除
		if err := c.db.Delete(&skill).Error; err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "删除失败",
			})
			return
		}
	} else {
		// 软删除（停用）
		skill.IsActive = false
		if err := c.db.Save(&skill).Error; err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "停用失败",
			})
			return
		}
	}

	// 清除缓存
	c.skillAssembler.ClearCache()

	ctx.JSON(http.StatusOK, gin.H{
		"message": "操作成功",
	})
}

// ActivateSkill 激活 Skill
// @Summary 激活 Skill
// @Description 激活已停用的 Skill
// @Tags Skill
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} models.Skill
// @Failure 404 {object} map[string]string
// @Router /api/v1/admin/skills/{id}/activate [post]
func (c *SkillController) ActivateSkill(ctx *gin.Context) {
	skillID := ctx.Param("id")

	var skill models.Skill
	if err := c.db.Where("id = ?", skillID).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Skill 不存在",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	skill.IsActive = true
	if err := c.db.Save(&skill).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "激活失败",
		})
		return
	}

	// 清除缓存
	c.skillAssembler.ClearCache()

	ctx.JSON(http.StatusOK, skill)
}

// PreviewDomainSkillDiscovery 预览 Domain Skill 自动发现结果
// @Summary 预览 Domain Skill 自动发现
// @Description 根据 Task Skill 的 domain_tags 预览将会自动发现的 Domain Skills
// @Tags Skill
// @Accept json
// @Produce json
// @Param task_skill query string true "Task Skill 名称"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Router /api/v1/admin/skills/preview-discovery [get]
func (c *SkillController) PreviewDomainSkillDiscovery(ctx *gin.Context) {
	taskSkillName := ctx.Query("task_skill")
	if taskSkillName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "请提供 task_skill 参数",
		})
		return
	}

	// 加载 Task Skill
	var taskSkill models.Skill
	if err := c.db.Where("name = ? AND is_active = ?", taskSkillName, true).First(&taskSkill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error":      "Task Skill 不存在或未激活",
				"task_skill": taskSkillName,
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	// 获取 Task Skill 的 domain_tags
	domainTags := taskSkill.Metadata.DomainTags
	if len(domainTags) == 0 {
		ctx.JSON(http.StatusOK, gin.H{
			"task_skill":        taskSkillName,
			"domain_tags":       []string{},
			"discovered_skills": []models.Skill{},
			"message":           "Task Skill 没有定义 domain_tags，不会自动发现 Domain Skills",
		})
		return
	}

	// 查询匹配的 Domain Skills
	var discoveredSkills []models.Skill
	var conditions []string
	var args []interface{}
	for _, tag := range domainTags {
		conditions = append(conditions, "metadata->>'tags' LIKE ?")
		args = append(args, "%"+tag+"%")
	}

	query := c.db.Where("layer = ? AND is_active = ?", models.SkillLayerDomain, true)
	if len(conditions) > 0 {
		conditionStr := "(" + conditions[0]
		for i := 1; i < len(conditions); i++ {
			conditionStr += " OR " + conditions[i]
		}
		conditionStr += ")"
		query = query.Where(conditionStr, args...)
	}

	if err := query.Order("priority ASC").Find(&discoveredSkills).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询 Domain Skills 失败",
		})
		return
	}

	// 构建响应
	skillSummaries := make([]gin.H, len(discoveredSkills))
	for i, skill := range discoveredSkills {
		skillSummaries[i] = gin.H{
			"name":         skill.Name,
			"display_name": skill.DisplayName,
			"tags":         skill.Metadata.Tags,
			"priority":     skill.Priority,
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task_skill":        taskSkillName,
		"domain_tags":       domainTags,
		"discovered_skills": skillSummaries,
		"discovered_count":  len(discoveredSkills),
	})
}

// GetSkillUsageStats 获取 Skill 使用统计
// @Summary 获取 Skill 使用统计
// @Description 获取 Skill 的使用次数、平均评分等统计信息
// @Tags Skill
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} models.SkillUsageStats
// @Failure 404 {object} map[string]string
// @Router /api/v1/admin/skills/{id}/stats [get]
func (c *SkillController) GetSkillUsageStats(ctx *gin.Context) {
	skillID := ctx.Param("id")

	// 检查 Skill 是否存在
	var skill models.Skill
	if err := c.db.Where("id = ?", skillID).First(&skill).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{
				"error": "Skill 不存在",
			})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "查询失败",
		})
		return
	}

	// 统计使用次数
	var usageCount int64
	c.db.Model(&models.SkillUsageLog{}).
		Where("skill_ids @> ?", `["`+skillID+`"]`).
		Count(&usageCount)

	// 统计平均评分
	var avgRating float64
	c.db.Model(&models.SkillUsageLog{}).
		Where("skill_ids @> ? AND user_feedback IS NOT NULL", `["`+skillID+`"]`).
		Select("COALESCE(AVG(user_feedback), 0)").
		Scan(&avgRating)

	// 统计平均执行时间
	var avgExecTime float64
	c.db.Model(&models.SkillUsageLog{}).
		Where("skill_ids @> ?", `["`+skillID+`"]`).
		Select("COALESCE(AVG(execution_time_ms), 0)").
		Scan(&avgExecTime)

	// 获取最后使用时间
	var lastLog models.SkillUsageLog
	var lastUsedAt *string
	if err := c.db.Where("skill_ids @> ?", `["`+skillID+`"]`).
		Order("created_at DESC").
		First(&lastLog).Error; err == nil {
		t := lastLog.CreatedAt.Format("2006-01-02 15:04:05")
		lastUsedAt = &t
	}

	ctx.JSON(http.StatusOK, gin.H{
		"skill_id":         skillID,
		"skill_name":       skill.Name,
		"usage_count":      usageCount,
		"avg_rating":       avgRating,
		"avg_exec_time_ms": avgExecTime,
		"last_used_at":     lastUsedAt,
	})
}
