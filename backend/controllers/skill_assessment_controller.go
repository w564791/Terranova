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
// @Summary Get skill assessment overview
// @Description Get aggregated assessment KPI data including pass rate, per-capability stats, recent failures, and daily trend
// @Tags Skill Assessment
// @Produce json
// @Param days query int false "Number of days" default(7)
// @Param fail_page query int false "Failure list page number" default(1)
// @Param fail_page_size query int false "Failure list page size" default(10)
// @Success 200 {object} services.AssessmentOverview
// @Security BearerAuth
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
// @Summary Get capability assessment detail
// @Description Get detailed assessment data for a specific capability
// @Tags Skill Assessment
// @Produce json
// @Param capability query string true "Capability name"
// @Param days query int false "Number of days" default(7)
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/skill-assessment/detail [get]
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
// @Summary Compare skill assessment versions
// @Description Compare assessment data between two content_hash versions
// @Tags Skill Assessment
// @Produce json
// @Param capability query string true "Capability name"
// @Param hash_a query string true "First content hash"
// @Param hash_b query string true "Second content hash"
// @Param days query int false "Number of days" default(7)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/skill-assessment/compare [get]
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
// @Summary Get top rule violations
// @Description Get the most frequently violated rules across skill assessments
// @Tags Skill Assessment
// @Produce json
// @Param capability query string false "Capability name (optional)"
// @Param days query int false "Number of days" default(7)
// @Param limit query int false "Max results" default(10)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/skill-assessment/top-violations [get]
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
