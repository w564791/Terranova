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
	failPage, _ := strconv.Atoi(ctx.DefaultQuery("fail_page", "1"))
	failPageSize, _ := strconv.Atoi(ctx.DefaultQuery("fail_page_size", "10"))

	overview, err := c.service.GetOverview(days, failPage, failPageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch assessment overview",
			"details": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, overview)
}

// GetCapabilityDetail returns detail data for a single capability
// GET /api/v1/admin/skill-assessment/detail?capability=plan_summary&days=7
func (c *SkillAssessmentController) GetCapabilityDetail(ctx *gin.Context) {
	capability := ctx.Query("capability")
	if capability == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "capability is required"})
		return
	}
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days <= 0 || days > 365 {
		days = 7
	}
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	detail, err := c.service.GetCapabilityDetail(capability, days, page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to fetch capability detail",
			"details": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, detail)
}

// CompareVersions returns comparison data between two content_hash versions
// GET /api/v1/admin/skill-assessment/compare?capability=xxx&hash_a=xxx&hash_b=xxx&days=7
func (c *SkillAssessmentController) CompareVersions(ctx *gin.Context) {
	capability := ctx.Query("capability")
	if capability == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "capability is required"})
		return
	}
	hashA := ctx.Query("hash_a")
	hashB := ctx.Query("hash_b")
	if hashA == "" || hashB == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "hash_a and hash_b are required"})
		return
	}
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days <= 0 || days > 365 {
		days = 7
	}

	compare, err := c.service.CompareVersions(capability, hashA, hashB, days)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to compare versions",
			"details": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, compare)
}

// GetTopViolations returns the most frequent rule violations
// GET /api/v1/admin/skill-assessment/top-violations?capability=xxx&days=7&limit=10
func (c *SkillAssessmentController) GetTopViolations(ctx *gin.Context) {
	capability := ctx.Query("capability") // optional
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "10"))

	violations, err := c.service.GetTopViolations(capability, days, limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"violations": violations})
}
