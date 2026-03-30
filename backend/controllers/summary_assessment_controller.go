package controllers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SummaryAssessmentController struct {
	db *gorm.DB
}

func NewSummaryAssessmentController(db *gorm.DB) *SummaryAssessmentController {
	return &SummaryAssessmentController{db: db}
}

type SummaryAssessmentOverview struct {
	SummaryCoverage   SummaryCoverage     `json:"summary_coverage"`
	Assessment        AssessmentStats     `json:"assessment"`
	SecurityTagStats  SecurityTagStatsDTO `json:"security_tag_stats"`
	IssueDistribution IssueDistribution   `json:"issue_distribution"`
	DailyTrend        []SummaryDailyTrend `json:"daily_trend"`
	ByResourceType    []ResourceTypeStats `json:"by_resource_type"`
}

type SummaryCoverage struct {
	TotalResources int64   `json:"total_resources"`
	WithSummary    int64   `json:"with_summary"`
	CoveragePct    float64 `json:"coverage_pct"`
}

type AssessmentStats struct {
	TotalAssessed int64   `json:"total_assessed"`
	L1PassRate    float64 `json:"l1_pass_rate"`
	L2AvgScore    float64 `json:"l2_avg_score"`
	L3AvgScore    float64 `json:"l3_avg_score"`
	L1Pass        int64   `json:"l1_pass"`
	L1Warn        int64   `json:"l1_warn"`
	L1Fail        int64   `json:"l1_fail"`
}

type SecurityTagStatsDTO struct {
	TotalExpected int64          `json:"total_expected"`
	TotalHit      int64          `json:"total_hit"`
	HitRate       float64        `json:"hit_rate"`
	MissesByRule  []RuleMissCount `json:"misses_by_rule"`
}

type RuleMissCount struct {
	Rule      string `json:"rule"`
	MissCount int64  `json:"miss_count"`
}

type IssueDistribution struct {
	FormatViolations      []IssueCount `json:"format_violations"`
	HallucinationSuspects int64        `json:"hallucination_suspects"`
	SecurityTagMisses     int64        `json:"security_tag_misses"`
}

type IssueCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type SummaryDailyTrend struct {
	Date     string  `json:"date"`
	Total    int64   `json:"total"`
	Pass     int64   `json:"pass"`
	Warn     int64   `json:"warn"`
	Fail     int64   `json:"fail"`
	PassRate float64 `json:"pass_rate"`
}

type ResourceTypeStats struct {
	ResourceType string  `json:"resource_type"`
	Count        int64   `json:"count"`
	PassRate     float64 `json:"pass_rate"`
	AvgScore     float64 `json:"avg_score"`
}

// GetOverview returns summary assessment dashboard data
// @Summary Get summary assessment overview
// @Description Get dashboard data for summary assessment including coverage, pass rates, security tag stats, issue distribution, daily trend, and per-resource-type stats
// @Tags Summary Assessment
// @Produce json
// @Param days query int false "Number of days" default(7)
// @Success 200 {object} SummaryAssessmentOverview
// @Security BearerAuth
// @Router /api/v1/admin/summary-assessment/overview [get]
func (c *SummaryAssessmentController) GetOverview(ctx *gin.Context) {
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days < 1 || days > 365 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	overview := SummaryAssessmentOverview{}

	// Summary Coverage
	var totalResources, withSummary int64
	c.db.Model(&models.ResourceIndex{}).
		Where("resource_mode = 'managed'").
		Count(&totalResources)
	c.db.Model(&models.ResourceIndex{}).
		Where("resource_mode = 'managed' AND resource_summary IS NOT NULL AND resource_summary != ''").
		Count(&withSummary)

	overview.SummaryCoverage = SummaryCoverage{
		TotalResources: totalResources,
		WithSummary:    withSummary,
	}
	if totalResources > 0 {
		overview.SummaryCoverage.CoveragePct = float64(withSummary) / float64(totalResources) * 100
	}

	// Assessment Stats
	var totalAssessed, l1Pass, l1Warn, l1Fail int64
	baseWhere := "source_type = ? AND assessment_layer = ? AND assessed_at >= ?"
	c.db.Model(&models.SkillAssessmentResult{}).
		Where(baseWhere, "summary", "schema", since).Count(&totalAssessed)
	c.db.Model(&models.SkillAssessmentResult{}).
		Where(baseWhere+" AND verdict = ?", "summary", "schema", since, "pass").Count(&l1Pass)
	c.db.Model(&models.SkillAssessmentResult{}).
		Where(baseWhere+" AND verdict = ?", "summary", "schema", since, "warn").Count(&l1Warn)
	c.db.Model(&models.SkillAssessmentResult{}).
		Where(baseWhere+" AND verdict = ?", "summary", "schema", since, "fail").Count(&l1Fail)

	var l2Avg, l3Avg struct{ Avg float64 }
	c.db.Model(&models.SkillAssessmentResult{}).
		Select("COALESCE(AVG(score), 0) as avg").
		Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ?", "summary", "rule", since).Scan(&l2Avg)
	c.db.Model(&models.SkillAssessmentResult{}).
		Select("COALESCE(AVG(score), 0) as avg").
		Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ?", "summary", "semantic", since).Scan(&l3Avg)

	overview.Assessment = AssessmentStats{
		TotalAssessed: totalAssessed, L1Pass: l1Pass, L1Warn: l1Warn, L1Fail: l1Fail,
		L2AvgScore: l2Avg.Avg, L3AvgScore: l3Avg.Avg,
	}
	if totalAssessed > 0 {
		overview.Assessment.L1PassRate = float64(l1Pass) / float64(totalAssessed) * 100
	}

	// Security Tag Stats
	// total_expected = resources where at least one security rule's AttrPattern matched
	// We approximate by counting: resources with misses + resources that passed all security checks
	// Since we only record misses, we count total misses and report that directly
	var withMisses int64
	c.db.Model(&models.SkillAssessmentResult{}).
		Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ? AND security_tag_misses IS NOT NULL AND security_tag_misses != 'null' AND security_tag_misses != '[]'",
			"summary", "schema", since).Count(&withMisses)

	missesByRule := c.getMissesByRule(since)
	// Sum individual miss counts for total
	var totalMissCount int64
	for _, m := range missesByRule {
		totalMissCount += m.MissCount
	}

	overview.SecurityTagStats = SecurityTagStatsDTO{
		TotalExpected: totalAssessed,
		TotalHit:      totalAssessed - withMisses,
		MissesByRule:  missesByRule,
	}
	if totalAssessed > 0 {
		overview.SecurityTagStats.HitRate = float64(totalAssessed-withMisses) / float64(totalAssessed) * 100
	}

	// Issue Distribution
	overview.IssueDistribution = c.getIssueDistribution(since)

	// Daily Trend
	overview.DailyTrend = c.getDailyTrend(since)

	// By Resource Type
	overview.ByResourceType = c.getByResourceType(since)

	ctx.JSON(http.StatusOK, overview)
}

func (c *SummaryAssessmentController) getMissesByRule(since time.Time) []RuleMissCount {
	var results []models.SkillAssessmentResult
	c.db.Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ? AND security_tag_misses IS NOT NULL AND security_tag_misses != 'null'",
		"summary", "schema", since).
		Select("security_tag_misses").Find(&results)

	counts := make(map[string]int64)
	for _, r := range results {
		if r.SecurityTagMisses == nil {
			continue
		}
		var misses []map[string]string
		if err := json.Unmarshal(*r.SecurityTagMisses, &misses); err != nil {
			continue
		}
		for _, m := range misses {
			counts[m["rule"]]++
		}
	}

	var out []RuleMissCount
	for rule, count := range counts {
		out = append(out, RuleMissCount{Rule: rule, MissCount: count})
	}
	return out
}

func (c *SummaryAssessmentController) getIssueDistribution(since time.Time) IssueDistribution {
	dist := IssueDistribution{}

	var results []models.SkillAssessmentResult
	c.db.Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ? AND format_violations IS NOT NULL",
		"summary", "schema", since).
		Select("format_violations").Find(&results)

	typeCounts := make(map[string]int64)
	for _, r := range results {
		for _, v := range r.FormatViolations {
			typeCounts[v]++
		}
	}
	for t, cnt := range typeCounts {
		dist.FormatViolations = append(dist.FormatViolations, IssueCount{Type: t, Count: cnt})
	}

	// Count individual hallucination suspects, not just records
	var hallucinationResults []models.SkillAssessmentResult
	c.db.Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ? AND hallucination_suspects IS NOT NULL",
		"summary", "schema", since).
		Select("hallucination_suspects").Find(&hallucinationResults)
	for _, r := range hallucinationResults {
		dist.HallucinationSuspects += int64(len(r.HallucinationSuspects))
	}

	c.db.Model(&models.SkillAssessmentResult{}).
		Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ? AND security_tag_misses IS NOT NULL AND security_tag_misses != 'null'",
			"summary", "schema", since).Count(&dist.SecurityTagMisses)

	return dist
}

func (c *SummaryAssessmentController) getDailyTrend(since time.Time) []SummaryDailyTrend {
	type row struct {
		Date    string
		Verdict string
		Count   int64
	}
	var rows []row
	c.db.Model(&models.SkillAssessmentResult{}).
		Select("TO_CHAR(assessed_at, 'YYYY-MM-DD') as date, verdict, COUNT(*) as count").
		Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ?", "summary", "schema", since).
		Group("date, verdict").Order("date").Scan(&rows)

	dayMap := make(map[string]*SummaryDailyTrend)
	for _, r := range rows {
		if _, ok := dayMap[r.Date]; !ok {
			dayMap[r.Date] = &SummaryDailyTrend{Date: r.Date}
		}
		d := dayMap[r.Date]
		switch r.Verdict {
		case "pass":
			d.Pass = r.Count
		case "warn":
			d.Warn = r.Count
		case "fail":
			d.Fail = r.Count
		}
		d.Total += r.Count
	}

	var out []SummaryDailyTrend
	for _, d := range dayMap {
		if d.Total > 0 {
			d.PassRate = float64(d.Pass) / float64(d.Total) * 100
		}
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

func (c *SummaryAssessmentController) getByResourceType(since time.Time) []ResourceTypeStats {
	type row struct {
		SkillName string  `gorm:"column:skill_name"`
		Count     int64
		AvgScore  float64 `gorm:"column:avg_score"`
		PassCount int64   `gorm:"column:pass_count"`
	}
	var rows []row
	c.db.Model(&models.SkillAssessmentResult{}).
		Select("skill_name, COUNT(*) as count, AVG(score) as avg_score, SUM(CASE WHEN verdict = 'pass' THEN 1 ELSE 0 END) as pass_count").
		Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ?", "summary", "schema", since).
		Group("skill_name").Order("count DESC").Scan(&rows)

	var out []ResourceTypeStats
	for _, r := range rows {
		passRate := float64(0)
		if r.Count > 0 {
			passRate = float64(r.PassCount) / float64(r.Count) * 100
		}
		out = append(out, ResourceTypeStats{
			ResourceType: r.SkillName, Count: r.Count, PassRate: passRate, AvgScore: r.AvgScore,
		})
	}
	return out
}

// IssueResource 问题资源详情
type IssueResource struct {
	ResourceID      uint   `json:"resource_id"`
	ResourceType    string `json:"resource_type"`
	ResourceName    string `json:"resource_name"`
	ResourceSummary string `json:"resource_summary"`
	Verdict         string `json:"verdict"`
	Score           int    `json:"score"`
	Details         string `json:"details"` // 具体问题内容
}

// GetIssueResources 查询指定问题类型的受影响资源
// @Summary Get issue resources
// @Description Query resources affected by a specific issue type (format violation, hallucination, security_miss, etc.)
// @Tags Summary Assessment
// @Produce json
// @Param type query string true "Issue type: over_length, markdown_syntax, first_line_format, empty_summary, hallucination, security_miss, security_miss:{rule_name}"
// @Param days query int false "Number of days" default(7)
// @Success 200 {array} IssueResource
// @Failure 400 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/summary-assessment/issue-resources [get]
func (c *SummaryAssessmentController) GetIssueResources(ctx *gin.Context) {
	issueType := ctx.Query("type")
	if issueType == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "type parameter required"})
		return
	}
	days, _ := strconv.Atoi(ctx.DefaultQuery("days", "7"))
	if days < 1 || days > 365 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	var assessments []models.SkillAssessmentResult
	baseQuery := c.db.Where("source_type = ? AND assessment_layer = ? AND assessed_at >= ?", "summary", "schema", since)

	switch {
	case issueType == "hallucination":
		baseQuery = baseQuery.Where("hallucination_suspects IS NOT NULL AND array_length(hallucination_suspects, 1) > 0")
	case issueType == "security_miss":
		baseQuery = baseQuery.Where("security_tag_misses IS NOT NULL AND security_tag_misses != 'null' AND security_tag_misses != '[]'")
	case strings.HasPrefix(issueType, "security_miss:"):
		ruleName := strings.TrimPrefix(issueType, "security_miss:")
		baseQuery = baseQuery.Where("security_tag_misses::text LIKE ?", "%"+escapeLike(ruleName)+"%")
	default:
		// format violation type: use array contains
		baseQuery = baseQuery.Where("? = ANY(format_violations)", issueType)
	}

	baseQuery.Select("resource_id, skill_name, verdict, score, format_violations, security_tag_misses, hallucination_suspects").
		Order("assessed_at DESC").
		Limit(100).
		Find(&assessments)

	// Collect resource IDs
	resourceIDs := make([]uint, 0, len(assessments))
	for _, a := range assessments {
		if a.ResourceID != nil {
			resourceIDs = append(resourceIDs, *a.ResourceID)
		}
	}

	// Batch fetch resource details
	resourceMap := make(map[uint]models.ResourceIndex)
	if len(resourceIDs) > 0 {
		var resources []models.ResourceIndex
		c.db.Where("id IN ?", resourceIDs).
			Select("id, resource_type, resource_name, resource_summary, terraform_address").
			Find(&resources)
		for _, r := range resources {
			resourceMap[r.ID] = r
		}
	}

	// Build response
	var result []IssueResource
	for _, a := range assessments {
		if a.ResourceID == nil {
			continue
		}
		r := resourceMap[*a.ResourceID]
		name := r.ResourceName
		if name == "" {
			name = r.TerraformAddress
		}

		// Build details string based on issue type
		details := ""
		switch {
		case issueType == "hallucination":
			details = formatTextArray(a.HallucinationSuspects)
		case issueType == "security_miss" || (len(issueType) > 14 && issueType[:14] == "security_miss:"):
			if a.SecurityTagMisses != nil {
				details = string(*a.SecurityTagMisses)
			}
		default:
			details = formatTextArray(a.FormatViolations)
		}

		result = append(result, IssueResource{
			ResourceID:      *a.ResourceID,
			ResourceType:    a.SkillName,
			ResourceName:    name,
			ResourceSummary: r.ResourceSummary,
			Verdict:         string(a.Verdict),
			Score:           int(a.Score),
			Details:         details,
		})
	}

	ctx.JSON(http.StatusOK, result)
}

// RegenerateSummaries 重新生成指定资源的摘要
// @Summary Regenerate resource summaries
// @Description Schedule regeneration of AI summaries for specified resources (max 100 per request)
// @Tags Summary Assessment
// @Accept json
// @Produce json
// @Param request body object true "Resource IDs to regenerate"
// @Success 200 {object} map[string]interface{} "Regeneration scheduled"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Server error"
// @Security BearerAuth
// @Router /api/v1/admin/summary-assessment/regenerate [post]
func (c *SummaryAssessmentController) RegenerateSummaries(ctx *gin.Context) {
	var req struct {
		ResourceIDs []uint `json:"resource_ids" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.ResourceIDs) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "resource_ids is empty"})
		return
	}
	if len(req.ResourceIDs) > 100 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "max 100 resources per request"})
		return
	}

	// 1. 收集每个资源的评估问题，构建 regeneration hint
	hints := c.buildRegenerationHints(req.ResourceIDs)

	// 2. 在事务中完成：清 hash + 写 hint + 删旧评估 + 建 job
	var affectedCount int64
	var jobCount int
	txErr := c.db.Transaction(func(tx *gorm.DB) error {
		// 清空 summary_hash 和评估状态，写入 hint
		for _, id := range req.ResourceIDs {
			hint := hints[id]
			if len(hint) > 2000 {
				hint = hint[:2000] + "...(truncated)"
			}
			result := tx.Model(&models.ResourceIndex{}).Where("id = ?", id).Updates(map[string]interface{}{
				"summary_hash":               "",
				"summary_assessment_status":  "",
				"summary_regeneration_hint":  hint,
			})
			if result.Error != nil {
				return result.Error
			}
			affectedCount += result.RowsAffected
		}

		// 删除旧的评估记录
		if err := tx.Where("source_type = ? AND resource_id IN ?", "summary", req.ResourceIDs).
			Delete(&models.SkillAssessmentResult{}).Error; err != nil {
			return err
		}

		// 按 source 分组，为每个 source 创建 summary + assessment job
		type sourceGroup struct {
			ExternalSourceID string
		}
		var groups []sourceGroup
		tx.Model(&models.ResourceIndex{}).
			Select("DISTINCT external_source_id").
			Where("id IN ? AND external_source_id != ''", req.ResourceIDs).
			Scan(&groups)

		for _, g := range groups {
			var activeCount int64
			tx.Model(&models.PostSyncJob{}).
				Where("source_id = ? AND job_type = ? AND status IN ?", g.ExternalSourceID,
					models.PostSyncJobTypeSummary,
					[]string{models.PostSyncJobStatusPending, models.PostSyncJobStatusProcessing}).
				Count(&activeCount)
			if activeCount > 0 {
				continue
			}

			now := time.Now()
			summaryJob := models.PostSyncJob{
				SourceID: g.ExternalSourceID, JobType: models.PostSyncJobTypeSummary,
				Status: models.PostSyncJobStatusPending, CreatedAt: now,
			}
			if err := tx.Create(&summaryJob).Error; err != nil {
				return err
			}

			assessmentJob := models.PostSyncJob{
				SourceID: g.ExternalSourceID, JobType: models.PostSyncJobTypeSummaryAssessment,
				Status: models.PostSyncJobStatusPending, DependsOn: &summaryJob.ID, CreatedAt: now,
			}
			if err := tx.Create(&assessmentJob).Error; err != nil {
				return err
			}
			jobCount++
		}
		return nil
	})

	if txErr != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": txErr.Error()})
		return
	}

	log.Printf("[SummaryAssessment] 重新生成 %d 个资源的摘要，创建 %d 组 job", affectedCount, jobCount)

	ctx.JSON(http.StatusOK, gin.H{
		"message":            "regeneration scheduled",
		"resources_affected": affectedCount,
		"jobs_created":       jobCount,
	})
}

// buildRegenerationHints 从评估记录中收集问题，为每个资源构建 regeneration hint
func (c *SummaryAssessmentController) buildRegenerationHints(resourceIDs []uint) map[uint]string {
	var assessments []models.SkillAssessmentResult
	c.db.Where("source_type = ? AND resource_id IN ?", "summary", resourceIDs).
		Order("resource_id, assessment_layer").
		Find(&assessments)

	hints := make(map[uint]string)
	// Group by resource_id
	grouped := make(map[uint][]models.SkillAssessmentResult)
	for _, a := range assessments {
		if a.ResourceID != nil {
			grouped[*a.ResourceID] = append(grouped[*a.ResourceID], a)
		}
	}

	for rid, results := range grouped {
		var parts []string
		for _, r := range results {
			layerName := string(r.AssessmentLayer)
			switch r.AssessmentLayer {
			case models.AssessmentLayerSchema:
				layerName = "文本规则检测"
			case models.AssessmentLayerRule:
				layerName = "Prompt遵从度"
			case models.AssessmentLayerSemantic:
				layerName = "内容质量"
			}

			if r.Verdict == models.AssessmentVerdictPass {
				continue
			}

			issues := []string{}
			if len(r.FormatViolations) > 0 {
				issues = append(issues, "格式问题: "+formatTextArray(r.FormatViolations))
			}
			if len(r.HallucinationSuspects) > 0 {
				issues = append(issues, "疑似幻觉值: "+formatTextArray(r.HallucinationSuspects))
			}
			if r.SecurityTagMisses != nil {
				var misses []map[string]string
				json.Unmarshal(*r.SecurityTagMisses, &misses)
				for _, m := range misses {
					issues = append(issues, fmt.Sprintf("缺失安全标注: %s", m["expected_tag"]))
				}
			}
			if r.RuleViolations != nil {
				var violations []map[string]interface{}
				json.Unmarshal(*r.RuleViolations, &violations)
				for _, v := range violations {
					if detail, ok := v["detail"].(string); ok {
						issues = append(issues, detail)
					}
				}
			}
			if r.QualityIssues != nil {
				var qis []map[string]interface{}
				json.Unmarshal(*r.QualityIssues, &qis)
				for _, qi := range qis {
					if detail, ok := qi["detail"].(string); ok {
						issues = append(issues, detail)
					}
				}
			}

			if len(issues) > 0 {
				parts = append(parts, fmt.Sprintf("[%s] %s", layerName, strings.Join(issues, "; ")))
			}
		}

		if len(parts) > 0 {
			hints[rid] = strings.Join(parts, "\n")
		}
	}

	return hints
}

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func formatTextArray(arr models.TextArray) string {
	if len(arr) == 0 {
		return ""
	}
	result := ""
	for i, s := range arr {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
