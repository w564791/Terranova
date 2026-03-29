package controllers

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
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
// GET /admin/summary-assessment/issue-resources?type=over_length&days=7
// type: format violation type (over_length, markdown_syntax, first_line_format, empty_summary)
//       or "hallucination" or "security_miss" or "security_miss:{rule_name}"
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
		baseQuery = baseQuery.Where("hallucination_suspects IS NOT NULL AND hallucination_suspects != '{}'")
	case issueType == "security_miss":
		baseQuery = baseQuery.Where("security_tag_misses IS NOT NULL AND security_tag_misses != 'null' AND security_tag_misses != '[]'")
	case len(issueType) > 14 && issueType[:14] == "security_miss:":
		// security_miss:公网暴露 — filter by specific rule name
		ruleName := issueType[14:]
		baseQuery = baseQuery.Where("security_tag_misses::text LIKE ?", "%"+ruleName+"%")
	default:
		// format violation type: over_length, markdown_syntax, first_line_format, empty_summary
		baseQuery = baseQuery.Where("format_violations::text LIKE ?", "%"+issueType+"%")
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
// POST /admin/summary-assessment/regenerate
// Body: {"resource_ids": [1, 2, 3]}
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

	// 1. 清空 summary_hash 和评估状态，触发重新生成
	result := c.db.Model(&models.ResourceIndex{}).
		Where("id IN ?", req.ResourceIDs).
		Updates(map[string]interface{}{
			"summary_hash":              "",
			"summary_assessment_status": "",
		})

	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	// 2. 删除旧的评估记录
	c.db.Where("source_type = ? AND resource_id IN ?", "summary", req.ResourceIDs).
		Delete(&models.SkillAssessmentResult{})

	// 3. 按 source 分组，为每个 source 创建 summary + assessment job
	type sourceGroup struct {
		ExternalSourceID string
	}
	var groups []sourceGroup
	c.db.Model(&models.ResourceIndex{}).
		Select("DISTINCT external_source_id").
		Where("id IN ? AND external_source_id != ''", req.ResourceIDs).
		Scan(&groups)

	jobCount := 0
	for _, g := range groups {
		// 检查是否已有活跃 job
		var activeCount int64
		c.db.Model(&models.PostSyncJob{}).
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
		c.db.Create(&summaryJob)

		assessmentJob := models.PostSyncJob{
			SourceID: g.ExternalSourceID, JobType: models.PostSyncJobTypeSummaryAssessment,
			Status: models.PostSyncJobStatusPending, DependsOn: &summaryJob.ID, CreatedAt: now,
		}
		c.db.Create(&assessmentJob)
		jobCount++
	}

	log.Printf("[SummaryAssessment] 重新生成 %d 个资源的摘要，创建 %d 组 job", result.RowsAffected, jobCount)

	ctx.JSON(http.StatusOK, gin.H{
		"message":            "regeneration scheduled",
		"resources_affected": result.RowsAffected,
		"jobs_created":       jobCount,
	})
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
