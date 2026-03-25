package controllers

import (
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SkillAssessmentController handles skill assessment dashboard endpoints
type SkillAssessmentController struct {
	service *services.SkillAssessmentService
}

// NewSkillAssessmentController creates a new controller instance
func NewSkillAssessmentController(db *gorm.DB) *SkillAssessmentController {
	return &SkillAssessmentController{
		service: services.NewSkillAssessmentService(db),
	}
}

// GetOverview returns aggregated assessment dashboard data
// @Summary 获取 Skill 质量评估概览
// @Description 返回指定天数内的评估 KPI 数据，包括通过率、按能力分组统计、近期失败和每日趋势
// @Tags SkillAssessment
// @Produce json
// @Param days query int false "统计天数" default(7)
// @Success 200 {object} services.AssessmentOverview
// @Router /api/v1/admin/skill-assessment/overview [get]
func (c *SkillAssessmentController) GetOverview(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days <= 0 || days > 365 {
		days = 7
	}

	overview, err := c.service.GetOverview(days)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch assessment overview",
			"details": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, overview)
}
