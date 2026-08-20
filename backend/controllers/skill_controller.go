package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// skillUsageSnapshotScope is the tenant-relevant part of a usage-log input
// snapshot. A snapshot is treated as untrusted historical data: when it says
// a log was created for a workspace or task, that relationship must still be
// resolved through the canonical workspace -> project -> organization chain.
type skillUsageSnapshotScope struct {
	workspaceID string
	taskID      *uint
}

// parseSkillUsageSnapshotScope reads only the identity fields used to bind a
// usage log. Invalid or ambiguous values are an authorization failure rather
// than a reason to silently drop the field and treat the log as global.
func parseSkillUsageSnapshotScope(snapshot *json.RawMessage) (skillUsageSnapshotScope, error) {
	if snapshot == nil || len(bytes.TrimSpace(*snapshot)) == 0 || bytes.Equal(bytes.TrimSpace(*snapshot), []byte("null")) {
		return skillUsageSnapshotScope{}, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(*snapshot, &fields); err != nil || fields == nil {
		if err == nil {
			err = fmt.Errorf("input snapshot must be an object")
		}
		return skillUsageSnapshotScope{}, err
	}

	scope := skillUsageSnapshotScope{}
	if rawWorkspaceID, present := fields["workspace_id"]; present {
		if err := json.Unmarshal(rawWorkspaceID, &scope.workspaceID); err != nil || scope.workspaceID == "" {
			if err == nil {
				err = fmt.Errorf("invalid workspace_id in input snapshot")
			}
			return skillUsageSnapshotScope{}, err
		}
	}

	if rawTaskID, present := fields["task_id"]; present {
		var taskIDText string
		if err := json.Unmarshal(rawTaskID, &taskIDText); err != nil {
			var numericTaskID json.Number
			if err := json.Unmarshal(rawTaskID, &numericTaskID); err != nil {
				return skillUsageSnapshotScope{}, fmt.Errorf("invalid task_id in input snapshot")
			}
			taskIDText = numericTaskID.String()
		}
		taskID, err := strconv.ParseUint(taskIDText, 10, 32)
		if err != nil || taskID == 0 {
			return skillUsageSnapshotScope{}, fmt.Errorf("invalid task_id in input snapshot")
		}
		resolvedTaskID := uint(taskID)
		scope.taskID = &resolvedTaskID
	}

	return scope, nil
}

// SkillController Skill 管理控制器
type SkillController struct {
	db               *gorm.DB
	skillAssembler   *services.SkillAssembler
	assessmentWorker *services.AssessmentWorker
}

// NewSkillController 创建控制器实例
func NewSkillController(db *gorm.DB, opts ...interface{}) *SkillController {
	c := &SkillController{
		db:             db,
		skillAssembler: services.NewSkillAssembler(db),
	}
	for _, opt := range opts {
		if w, ok := opt.(*services.AssessmentWorker); ok {
			c.assessmentWorker = w
		}
	}
	return c
}

// ListSkills 获取 Skill 列表
// @Summary List skills
// @Description List skills with pagination, filter by layer, status, source type, and keyword search
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param layer query string false "Layer filter: foundation, domain, task"
// @Param is_active query bool false "Active status filter"
// @Param source_type query string false "Source type: manual, module_auto, hybrid"
// @Param search query string false "Search keyword (matches name, display_name, content)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} models.SkillListResponse
// @Security BearerAuth
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
// @Summary Get a skill
// @Description Get skill detail by ID or name
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID or name"
// @Success 200 {object} models.Skill
// @Failure 404 {object} map[string]string
// @Security BearerAuth
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
// @Summary Create a skill
// @Description Create a new skill
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param request body models.CreateSkillRequest true "Skill information"
// @Success 201 {object} models.Skill
// @Failure 400 {object} map[string]string
// @Security BearerAuth
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
// @Summary Update a skill
// @Description Update skill information
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param request body models.UpdateSkillRequest true "Update information"
// @Success 200 {object} models.Skill
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
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
// @Summary Deactivate a skill
// @Description Deactivate a skill (soft delete)
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/skills/{id}/deactivate [post]
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
// @Summary Delete a skill
// @Description Delete a skill by ID (supports hard delete via ?hard=true)
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param hard query bool false "Hard delete flag"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/skills/{id} [delete]
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
// @Summary Activate a skill
// @Description Activate a previously deactivated skill
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} models.Skill
// @Failure 404 {object} map[string]string
// @Security BearerAuth
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
// @Summary Preview domain skill discovery
// @Description Preview which domain skills would be auto-discovered based on a task skill's domain_tags
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param task_skill query string true "Task skill name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Security BearerAuth
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
// @Summary Get skill usage statistics
// @Description Get usage count, average rating, and other statistics for a skill
// @Tags Skill Admin
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]string
// @Security BearerAuth
// @Router /api/v1/admin/skills/{id}/usage-stats [get]
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

// requireSkillUsageCaller obtains the organization selected and authorized by
// IAM. Skill usage rows intentionally do not carry an org_id, so every access
// path below must bind a row through its workspace/task before mutating or
// returning it. There is no default organization for these endpoints.
func requireSkillUsageCaller(ctx *gin.Context) (string, uint, bool) {
	rawUserID, ok := ctx.Get("user_id")
	userID, _ := rawUserID.(string)
	if !ok || userID == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user authentication is required"})
		return "", 0, false
	}

	orgID, ok := middleware.AuthOrgID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "authenticated organization is required"})
		return "", 0, false
	}
	return userID, orgID, true
}

// resolveSkillUsageWorkspaceOrg follows the one and only canonical ownership
// chain for an AI usage log. A duplicate relation is corrupt data even when
// both projects happen to be in the same organization, so it is rejected.
func (c *SkillController) resolveSkillUsageWorkspaceOrg(ctx *gin.Context, workspaceID string) (uint, bool) {
	if c.db == nil || workspaceID == "" {
		return 0, false
	}

	var binding struct {
		RelationCount int64 `gorm:"column:relation_count"`
		OrgID         uint  `gorm:"column:org_id"`
	}
	err := c.db.WithContext(ctx.Request.Context()).Raw(`
SELECT COUNT(*) AS relation_count, MIN(p.org_id) AS org_id
FROM workspaces AS w
JOIN workspace_project_relations AS wpr ON wpr.workspace_id = w.workspace_id
JOIN projects AS p ON p.id = wpr.project_id
WHERE w.workspace_id = ?`, workspaceID).Scan(&binding).Error
	if err != nil || binding.RelationCount != 1 || binding.OrgID == 0 {
		return 0, false
	}
	return binding.OrgID, true
}

// resolveSkillUsageTaskScope resolves a task ID before it is used as a usage
// log selector. Task IDs are global, therefore a caller-supplied task must
// never be trusted until its workspace has a strict organization binding.
func (c *SkillController) resolveSkillUsageTaskScope(ctx *gin.Context, taskID uint) (string, uint, bool) {
	if c.db == nil || taskID == 0 {
		return "", 0, false
	}

	var task models.WorkspaceTask
	if err := c.db.WithContext(ctx.Request.Context()).Select("workspace_id").First(&task, taskID).Error; err != nil || task.WorkspaceID == "" {
		return "", 0, false
	}
	orgID, ok := c.resolveSkillUsageWorkspaceOrg(ctx, task.WorkspaceID)
	if !ok {
		return "", 0, false
	}
	return task.WorkspaceID, orgID, true
}

// resolveSkillUsageLogWorkspace resolves all workspace-bearing fields of a
// historical usage row. Conflicting workspace_id/task_id fields, malformed
// snapshots, missing tasks, and ambiguous workspace ownership are all denied
// rather than allowing a fallback to a global/system record.
//
// The returned scoped value distinguishes an unscoped user-owned record (which
// is still private to that user) from a system record, which must always have
// a determinable tenant scope.
func (c *SkillController) resolveSkillUsageLogWorkspace(ctx *gin.Context, usageLog *models.SkillUsageLog) (workspaceID string, scoped bool, valid bool) {
	if usageLog == nil {
		return "", true, false
	}

	snapshotScope, err := parseSkillUsageSnapshotScope(usageLog.InputSnapshot)
	if err != nil {
		return "", true, false
	}

	workspaceIDs := make(map[string]struct{}, 3)
	if usageLog.WorkspaceID != "" {
		workspaceIDs[usageLog.WorkspaceID] = struct{}{}
	}
	if snapshotScope.workspaceID != "" {
		workspaceIDs[snapshotScope.workspaceID] = struct{}{}
	}
	if snapshotScope.taskID != nil {
		taskWorkspaceID, _, ok := c.resolveSkillUsageTaskScope(ctx, *snapshotScope.taskID)
		if !ok {
			return "", true, false
		}
		workspaceIDs[taskWorkspaceID] = struct{}{}
	}

	if len(workspaceIDs) == 0 {
		return "", false, true
	}
	if len(workspaceIDs) != 1 {
		return "", true, false
	}
	for id := range workspaceIDs {
		return id, true, true
	}
	return "", true, false
}

// authorizeSkillUsageLog is the single access rule for usage-log feedback:
// a caller may update/read their own unscoped row, or their/system row only
// after a strict workspace -> project -> org binding proves it is in the
// authenticated organization. The local return value is used only to retain a
// useful 403 for another user's same-tenant row; all cross-tenant and
// indeterminate rows are hidden as 404.
func (c *SkillController) authorizeSkillUsageLog(ctx *gin.Context, usageLog *models.SkillUsageLog, userID string, authOrgID uint) (allowed bool, local bool) {
	if usageLog == nil || userID == "" || authOrgID == 0 {
		return false, false
	}

	workspaceID, scoped, valid := c.resolveSkillUsageLogWorkspace(ctx, usageLog)
	if !valid {
		return false, false
	}
	if !scoped {
		// No tenant can be proven for an unscoped system record. A user-owned
		// legacy/global log remains private to that exact user only.
		return usageLog.UserID == userID, usageLog.UserID == userID
	}

	orgID, ok := c.resolveSkillUsageWorkspaceOrg(ctx, workspaceID)
	if !ok || orgID != authOrgID {
		return false, false
	}
	if usageLog.UserID != userID && usageLog.UserID != "system" {
		return false, true
	}
	return true, true
}

// UpdateSkillUsageAction 更新用户对 Skill 输出的操作
// @Summary Update skill usage action
// @Description Update user action (accepted/modified/aborted) or feedback for a skill usage log
// @Tags Skill Usage
// @Accept json
// @Produce json
// @Param id path string true "Usage log ID"
// @Param request body object true "Action and/or feedback"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/ai/skill-usage/{id}/action [put]
func (c *SkillController) UpdateSkillUsageAction(ctx *gin.Context) {
	logID := ctx.Param("id")
	var req struct {
		Action           string  `json:"action,omitempty" binding:"omitempty,oneof=accepted modified aborted"`
		ModificationDiff *string `json:"modification_diff,omitempty"`
		Feedback         *int    `json:"feedback,omitempty" binding:"omitempty,min=1,max=5"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Action == "" && req.Feedback == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "action or feedback is required"})
		return
	}

	uid, authOrgID, ok := requireSkillUsageCaller(ctx)
	if !ok {
		return
	}

	var usageLog models.SkillUsageLog
	if err := c.db.First(&usageLog, "id = ?", logID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "usage log not found"})
		return
	}
	allowed, local := c.authorizeSkillUsageLog(ctx, &usageLog, uid, authOrgID)
	if !allowed && local {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "cannot update other user's action"})
		return
	}
	if !allowed {
		// Do not disclose a cross-tenant/system row just because its UUID was
		// guessed. This also fails closed for corrupt historical bindings.
		ctx.JSON(http.StatusNotFound, gin.H{"error": "usage log not found"})
		return
	}

	updates := map[string]interface{}{}
	if req.Action != "" {
		updates["user_action"] = req.Action
	}
	if req.ModificationDiff != nil && req.Action == "modified" {
		updates["user_modification_diff"] = *req.ModificationDiff
	}
	if req.Feedback != nil {
		updates["user_feedback"] = *req.Feedback
	}

	if err := c.db.Model(&usageLog).Updates(updates).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update usage log"})
		return
	}

	// 延迟补评触发：差评（<= 2）且尚未完成 Layer 2+3 评估时，提交补评
	if req.Feedback != nil && *req.Feedback <= 2 && c.assessmentWorker != nil {
		var ruleCount int64
		c.db.Model(&models.SkillAssessmentResult{}).
			Where("usage_log_id = ? AND assessment_layer = ?", logID, "rule").
			Count(&ruleCount)
		if ruleCount == 0 {
			c.assessmentWorker.Submit(logID, "")
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// UpdateSkillUsageByCapability 按 capability + 关联 ID 更新 user_action / feedback
// @Summary Update skill usage by capability
// @Description Update user action or feedback by capability and associated ID (for scenarios where usage_log_id is not available in frontend)
// @Tags Skill Usage
// @Accept json
// @Produce json
// @Param request body object true "Capability, task_id, action, feedback"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/ai/skill-usage/by-capability [put]
func (c *SkillController) UpdateSkillUsageByCapability(ctx *gin.Context) {
	var req struct {
		Capability string `json:"capability" binding:"required"`
		TaskID     *uint  `json:"task_id,omitempty"`
		Action     string `json:"action,omitempty" binding:"omitempty,oneof=accepted modified aborted"`
		Feedback   *int   `json:"feedback,omitempty" binding:"omitempty,min=1,max=5"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Action == "" && req.Feedback == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "action or feedback is required"})
		return
	}

	uid, authOrgID, ok := requireSkillUsageCaller(ctx)
	if !ok {
		return
	}

	if req.TaskID != nil {
		// A task ID in the request is a global identifier. Bind it first, before
		// even looking for a matching usage log, so it cannot select another
		// tenant's system record.
		_, taskOrgID, taskOK := c.resolveSkillUsageTaskScope(ctx, *req.TaskID)
		if !taskOK || taskOrgID != authOrgID {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "usage log not found"})
			return
		}
	}

	// Restrict the candidate set to the caller's own rows and system rows. The
	// latter are authorized one-by-one below, because the table has no org_id.
	var usageLogs []models.SkillUsageLog
	if err := c.db.WithContext(ctx.Request.Context()).
		Where("capability = ? AND user_id IN ?", req.Capability, []string{uid, "system"}).
		Order("created_at DESC").
		Find(&usageLogs).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query usage logs"})
		return
	}

	var usageLog *models.SkillUsageLog
	for i := range usageLogs {
		candidate := &usageLogs[i]
		if req.TaskID != nil {
			snapshotScope, err := parseSkillUsageSnapshotScope(candidate.InputSnapshot)
			if err != nil || snapshotScope.taskID == nil || *snapshotScope.taskID != *req.TaskID {
				continue
			}
		}
		if allowed, _ := c.authorizeSkillUsageLog(ctx, candidate, uid, authOrgID); allowed {
			usageLog = candidate
			break
		}
	}
	if usageLog == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "usage log not found"})
		return
	}

	updates := map[string]interface{}{}
	if req.Action != "" {
		updates["user_action"] = req.Action
	}
	if req.Feedback != nil {
		updates["user_feedback"] = *req.Feedback
	}

	if err := c.db.Model(usageLog).Updates(updates).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update usage log"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "ok", "usage_log_id": usageLog.ID})
}

// GetPendingFeedback 获取当前用户待评分的 usage logs
// @Summary Get pending feedback items
// @Description Get skill usage logs that are pending user feedback (last 24 hours, max 10)
// @Tags Skill Usage
// @Produce json
// @Success 200 {object} map[string]interface{} "Pending feedback items"
// @Security BearerAuth
// @Router /api/v1/ai/skill-usage/pending-feedback [get]
func (c *SkillController) GetPendingFeedback(ctx *gin.Context) {
	uid, authOrgID, ok := requireSkillUsageCaller(ctx)
	if !ok {
		return
	}

	type pendingItem struct {
		ID         string  `json:"id"`
		Capability string  `json:"capability"`
		UserAction string  `json:"user_action"`
		TaskID     *string `json:"task_id"`
		CreatedAt  string  `json:"created_at"`
	}

	var usageLogs []models.SkillUsageLog
	if err := c.db.WithContext(ctx.Request.Context()).
		Where("user_action IS NOT NULL AND user_feedback IS NULL AND created_at > ?", time.Now().Add(-24*time.Hour)).
		Where("user_id IN ?", []string{uid, "system"}).
		Order("created_at DESC").
		Find(&usageLogs).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query usage logs"})
		return
	}

	items := make([]pendingItem, 0, 10)
	for i := range usageLogs {
		if len(items) == 10 {
			break
		}
		usageLog := &usageLogs[i]
		if allowed, _ := c.authorizeSkillUsageLog(ctx, usageLog, uid, authOrgID); !allowed {
			continue
		}

		var taskID *string
		if snapshotScope, err := parseSkillUsageSnapshotScope(usageLog.InputSnapshot); err == nil && snapshotScope.taskID != nil {
			value := strconv.FormatUint(uint64(*snapshotScope.taskID), 10)
			taskID = &value
		}
		action := ""
		if usageLog.UserAction != nil {
			action = *usageLog.UserAction
		}
		items = append(items, pendingItem{
			ID:         usageLog.ID,
			Capability: usageLog.Capability,
			UserAction: action,
			TaskID:     taskID,
			CreatedAt:  usageLog.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

// SubmitFeedback 提交评分（通过 usage_log_id）
// @Summary Submit feedback for skill usage
// @Description Submit a rating (1-5) for a skill usage log
// @Tags Skill Usage
// @Accept json
// @Produce json
// @Param id path string true "Usage log ID"
// @Param request body object true "Feedback rating (1-5)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/ai/skill-usage/{id}/feedback [put]
func (c *SkillController) SubmitFeedback(ctx *gin.Context) {
	logID := ctx.Param("id")
	var req struct {
		Feedback int `json:"feedback" binding:"required,min=1,max=5"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, authOrgID, ok := requireSkillUsageCaller(ctx)
	if !ok {
		return
	}

	var usageLog models.SkillUsageLog
	if err := c.db.WithContext(ctx.Request.Context()).First(&usageLog, "id = ?", logID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "usage log not found or not owned by you"})
		return
	}
	allowed, local := c.authorizeSkillUsageLog(ctx, &usageLog, uid, authOrgID)
	if !allowed && local {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "cannot update other user's record"})
		return
	}
	if !allowed {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "usage log not found or not owned by you"})
		return
	}
	if err := c.db.Model(&usageLog).Update("user_feedback", req.Feedback).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update usage log"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}
