package services

import (
	"encoding/json"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// SkillAssessmentService queries aggregated assessment data for the dashboard
type SkillAssessmentService struct {
	db *gorm.DB
}

// NewSkillAssessmentService creates a new service instance
func NewSkillAssessmentService(db *gorm.DB) *SkillAssessmentService {
	return &SkillAssessmentService{db: db}
}

// AssessmentOverview holds dashboard KPI data
type AssessmentOverview struct {
	TotalLogs      int64              `json:"total_logs"`
	AssessedLogs   int64              `json:"assessed_logs"`
	TotalPass      int64              `json:"total_pass"`
	TotalFail      int64              `json:"total_fail"`
	TotalWarn      int64              `json:"total_warn"`
	PassRate       float64            `json:"pass_rate"`
	ActiveSkills   int64              `json:"active_skills"`
	HighRiskSkills []string           `json:"high_risk_skills"`
	ByCapability   []CapabilityStats  `json:"by_capability"`
	RecentFailures []RecentFailure    `json:"recent_failures"`
	FailureTotal   int64              `json:"failure_total"`
	DailyTrend     []DailyTrendItem   `json:"daily_trend"`
	// 业务指标
	AcceptRate       *float64 `json:"accept_rate"`        // 采纳率：accepted / (accepted+modified+aborted)
	ModifyRate       *float64 `json:"modify_rate"`        // 修改率：modified / (accepted+modified+aborted)
	NegativeFeedback *float64 `json:"negative_feedback"`  // 差评率：feedback<=2 / 有反馈总数
}

// CapabilityStats holds per-capability verdict breakdown
type CapabilityStats struct {
	Capability   string  `json:"capability"`
	Total        int64   `json:"total"`
	Pass         int64   `json:"pass"`
	Fail         int64   `json:"fail"`
	Warn         int64   `json:"warn"`
	PassRate     float64 `json:"pass_rate"`
	AvgScore     float64 `json:"avg_score"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
}

// RecentFailure holds details of a recent failed assessment
type RecentFailure struct {
	UsageLogID    string          `json:"usage_log_id"`
	Capability    string          `json:"capability"`
	SkillName     string          `json:"skill_name"`
	Verdict       string          `json:"verdict"`
	Score         int             `json:"score"`
	MissingFields json.RawMessage `json:"missing_fields"`
	InvalidEnums  json.RawMessage `json:"invalid_enum_fields"`
	AssessedAt    time.Time       `json:"assessed_at"`
	ContentHash   string          `json:"content_hash"`
	LatencyMs     *int            `json:"latency_ms"`
}

// DailyTrendItem holds daily verdict counts
type DailyTrendItem struct {
	Date string `json:"date"` // YYYY-MM-DD
	Pass int64  `json:"pass"`
	Fail int64  `json:"fail"`
	Warn int64  `json:"warn"`
}

// verdictCount is a helper struct for scanning verdict aggregation results
type verdictCount struct {
	Verdict string
	Cnt     int64
}

// dailyVerdictRow is a helper for scanning daily trend results
type dailyVerdictRow struct {
	Date    string
	Verdict string
	Cnt     int64
}

// GetOverview returns aggregated assessment data for the given time window
func (s *SkillAssessmentService) GetOverview(days, failPage, failPageSize int) (*AssessmentOverview, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	overview := &AssessmentOverview{
		HighRiskSkills: []string{},
		ByCapability:   []CapabilityStats{},
		RecentFailures: []RecentFailure{},
		DailyTrend:     []DailyTrendItem{},
	}

	// 1. Total logs and assessed logs
	var logStats struct {
		Total    int64
		Assessed int64
	}
	if err := s.db.Model(&models.SkillUsageLog{}).
		Select("count(*) as total, count(CASE WHEN assessment_status = 'assessed' THEN 1 END) as assessed").
		Where("created_at > ?", since).
		Scan(&logStats).Error; err != nil {
		return nil, err
	}
	overview.TotalLogs = logStats.Total
	overview.AssessedLogs = logStats.Assessed

	// 2. Verdict counts (按调用时间过滤，不是评估时间)
	var verdicts []verdictCount
	if err := s.db.Raw(`
		SELECT r.verdict, count(*) as cnt
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.created_at > ?
		GROUP BY r.verdict`, since).Scan(&verdicts).Error; err != nil {
		return nil, err
	}
	for _, v := range verdicts {
		switch models.AssessmentVerdict(v.Verdict) {
		case models.AssessmentVerdictPass:
			overview.TotalPass = v.Cnt
		case models.AssessmentVerdictFail:
			overview.TotalFail = v.Cnt
		case models.AssessmentVerdictWarn:
			overview.TotalWarn = v.Cnt
		}
	}
	totalAssessed := overview.TotalPass + overview.TotalFail + overview.TotalWarn
	if totalAssessed > 0 {
		overview.PassRate = float64(overview.TotalPass) / float64(totalAssessed) * 100
	}

	// 3. Active skills (distinct capabilities)
	if err := s.db.Model(&models.SkillUsageLog{}).
		Where("created_at > ?", since).
		Distinct("capability").
		Count(&overview.ActiveSkills).Error; err != nil {
		return nil, err
	}

	// 4. By capability (verdict counts + avg score + avg latency)
	type capabilityAggRow struct {
		Capability   string  `json:"capability"`
		Total        int64   `json:"total"`
		Pass         int64   `json:"pass"`
		Fail         int64   `json:"fail"`
		Warn         int64   `json:"warn"`
		AvgScore     float64 `json:"avg_score"`
		AvgLatencyMs float64 `json:"avg_latency_ms"`
	}
	var capAggRows []capabilityAggRow
	if err := s.db.Raw(`
		SELECT l.capability,
		       count(*) as total,
		       count(CASE WHEN r.verdict = 'pass' THEN 1 END) as pass,
		       count(CASE WHEN r.verdict = 'fail' THEN 1 END) as fail,
		       count(CASE WHEN r.verdict = 'warn' THEN 1 END) as warn,
		       COALESCE(avg(r.score), 0) as avg_score,
		       COALESCE(avg(l.latency_ms), 0) as avg_latency_ms
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.created_at > ?
		GROUP BY l.capability`, since).Scan(&capAggRows).Error; err != nil {
		return nil, err
	}

	for _, row := range capAggRows {
		passRate := float64(0)
		if row.Total > 0 {
			passRate = float64(row.Pass) / float64(row.Total) * 100
		}
		overview.ByCapability = append(overview.ByCapability, CapabilityStats{
			Capability:   row.Capability,
			Total:        row.Total,
			Pass:         row.Pass,
			Fail:         row.Fail,
			Warn:         row.Warn,
			PassRate:     passRate,
			AvgScore:     row.AvgScore,
			AvgLatencyMs: row.AvgLatencyMs,
		})
	}

	// 5. High risk skills (fail rate > 20%)
	for _, cs := range overview.ByCapability {
		if cs.Total > 0 && float64(cs.Fail)/float64(cs.Total)*100 > 20 {
			overview.HighRiskSkills = append(overview.HighRiskSkills, cs.Capability)
		}
	}

	// 6. Recent failures with pagination
	if failPage < 1 {
		failPage = 1
	}
	if failPageSize < 1 || failPageSize > 50 {
		failPageSize = 10
	}
	failOffset := (failPage - 1) * failPageSize

	s.db.Raw(`
		SELECT count(*)
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE r.verdict = 'fail' AND l.created_at > ?`, since).Scan(&overview.FailureTotal)

	if err := s.db.Raw(`
		SELECT r.usage_log_id, l.capability, r.skill_name, r.verdict, r.score,
		       COALESCE(array_to_json(r.missing_fields), '[]') as missing_fields,
		       COALESCE(array_to_json(r.invalid_enum_fields), '[]') as invalid_enum_fields,
		       r.assessed_at, r.assessment_latency_ms as latency_ms,
		       r.skill_content_hash as content_hash
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE r.verdict = 'fail' AND l.created_at > ?
		ORDER BY r.assessed_at DESC LIMIT ? OFFSET ?`, since, failPageSize, failOffset).Scan(&overview.RecentFailures).Error; err != nil {
		return nil, err
	}

	// 7. Daily trend (按调用日期聚合)
	var trendRows []dailyVerdictRow
	if err := s.db.Raw(`
		SELECT TO_CHAR(l.created_at, 'YYYY-MM-DD') as date, r.verdict, count(*) as cnt
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.created_at > ?
		GROUP BY TO_CHAR(l.created_at, 'YYYY-MM-DD'), r.verdict
		ORDER BY date`, since).Scan(&trendRows).Error; err != nil {
		return nil, err
	}

	trendMap := make(map[string]*DailyTrendItem)
	var trendOrder []string
	for _, row := range trendRows {
		dt, ok := trendMap[row.Date]
		if !ok {
			dt = &DailyTrendItem{Date: row.Date}
			trendMap[row.Date] = dt
			trendOrder = append(trendOrder, row.Date)
		}
		switch models.AssessmentVerdict(row.Verdict) {
		case models.AssessmentVerdictPass:
			dt.Pass = row.Cnt
		case models.AssessmentVerdictFail:
			dt.Fail = row.Cnt
		case models.AssessmentVerdictWarn:
			dt.Warn = row.Cnt
		}
	}
	for _, date := range trendOrder {
		overview.DailyTrend = append(overview.DailyTrend, *trendMap[date])
	}

	// 8. Business metrics: accept/modify/negative rates
	type actionStats struct {
		Accepted int64
		Modified int64
		Aborted  int64
	}
	var as actionStats
	s.db.Model(&models.SkillUsageLog{}).
		Select(`count(CASE WHEN user_action = 'accepted' THEN 1 END) as accepted,
		        count(CASE WHEN user_action = 'modified' THEN 1 END) as modified,
		        count(CASE WHEN user_action = 'aborted' THEN 1 END) as aborted`).
		Where("created_at > ? AND user_action IS NOT NULL", since).
		Scan(&as)

	actionTotal := as.Accepted + as.Modified + as.Aborted
	if actionTotal > 0 {
		ar := float64(as.Accepted) / float64(actionTotal) * 100
		mr := float64(as.Modified) / float64(actionTotal) * 100
		overview.AcceptRate = &ar
		overview.ModifyRate = &mr
	}

	type feedbackStats struct {
		Total    int64
		Negative int64
	}
	var fs feedbackStats
	s.db.Model(&models.SkillUsageLog{}).
		Select(`count(*) as total, count(CASE WHEN user_feedback <= 2 THEN 1 END) as negative`).
		Where("created_at > ? AND user_feedback IS NOT NULL", since).
		Scan(&fs)

	if fs.Total > 0 {
		nf := float64(fs.Negative) / float64(fs.Total) * 100
		overview.NegativeFeedback = &nf
	}

	return overview, nil
}

// ========== Version Compare API ==========

// VersionCompare holds comparison data between two content_hash versions
type VersionCompare struct {
	Capability string       `json:"capability"`
	VersionA   VersionStats `json:"version_a"`
	VersionB   VersionStats `json:"version_b"`
	ScoreDelta struct {
		L1 *float64 `json:"l1"`
		L2 *float64 `json:"l2"`
		L3 *float64 `json:"l3"`
	} `json:"score_delta"`
}

// CompareVersions returns comparison data between two content_hash versions of a capability
func (s *SkillAssessmentService) CompareVersions(capability, hashA, hashB string, days int) (*VersionCompare, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	// Helper to query version stats for a specific content_hash
	queryVersionStats := func(contentHash string) (VersionStats, error) {
		type versionRow struct {
			ContentHash string    `json:"content_hash"`
			Total       int64     `json:"total"`
			Pass        int64     `json:"pass"`
			Fail        int64     `json:"fail"`
			AvgScore    float64   `json:"avg_score"`
			FirstSeen   time.Time `json:"first_seen"`
			L1Total     int64     `json:"l1_total"`
			L1Pass      int64     `json:"l1_pass"`
			L2Total     int64     `json:"l2_total"`
			L2Pass      int64     `json:"l2_pass"`
			L2AvgScore  float64   `json:"l2_avg_score"`
			L3Total     int64     `json:"l3_total"`
			L3Pass      int64     `json:"l3_pass"`
			L3AvgScore  float64   `json:"l3_avg_score"`
		}
		var v versionRow
		if err := s.db.Raw(`
			SELECT l.skill_content_hash as content_hash,
			       count(*) as total,
			       count(CASE WHEN r.verdict = 'pass' THEN 1 END) as pass,
			       count(CASE WHEN r.verdict = 'fail' THEN 1 END) as fail,
			       COALESCE(avg(r.score), 0) as avg_score,
			       min(l.created_at) as first_seen,
			       count(CASE WHEN r.assessment_layer = 'schema' THEN 1 END) as l1_total,
			       count(CASE WHEN r.assessment_layer = 'schema' AND r.verdict = 'pass' THEN 1 END) as l1_pass,
			       count(CASE WHEN r.assessment_layer = 'rule' THEN 1 END) as l2_total,
			       count(CASE WHEN r.assessment_layer = 'rule' AND r.verdict = 'pass' THEN 1 END) as l2_pass,
			       COALESCE(avg(CASE WHEN r.assessment_layer = 'rule' THEN r.score END), 0) as l2_avg_score,
			       count(CASE WHEN r.assessment_layer = 'semantic' THEN 1 END) as l3_total,
			       count(CASE WHEN r.assessment_layer = 'semantic' AND r.verdict = 'pass' THEN 1 END) as l3_pass,
			       COALESCE(avg(CASE WHEN r.assessment_layer = 'semantic' THEN r.score END), 0) as l3_avg_score
			FROM skill_assessment_results r
			JOIN skill_usage_logs l ON l.id = r.usage_log_id
			WHERE l.capability = ? AND l.created_at > ? AND l.skill_content_hash = ?
			GROUP BY l.skill_content_hash`, capability, since, contentHash).Scan(&v).Error; err != nil {
			return VersionStats{}, err
		}

		vs := VersionStats{
			ContentHash: v.ContentHash,
			Total:       v.Total,
			Pass:        v.Pass,
			Fail:        v.Fail,
			AvgScore:    v.AvgScore,
			FirstSeen:   v.FirstSeen,
		}
		if v.Total > 0 {
			vs.PassRate = float64(v.Pass) / float64(v.Total) * 100
		}
		if v.L1Total > 0 {
			r := float64(v.L1Pass) / float64(v.L1Total) * 100
			vs.L1PassRate = &r
		}
		if v.L2Total > 0 {
			r := float64(v.L2Pass) / float64(v.L2Total) * 100
			vs.L2PassRate = &r
			vs.L2AvgScore = &v.L2AvgScore
		}
		if v.L3Total > 0 {
			r := float64(v.L3Pass) / float64(v.L3Total) * 100
			vs.L3PassRate = &r
			vs.L3AvgScore = &v.L3AvgScore
		}
		return vs, nil
	}

	vA, err := queryVersionStats(hashA)
	if err != nil {
		return nil, err
	}
	vB, err := queryVersionStats(hashB)
	if err != nil {
		return nil, err
	}

	result := &VersionCompare{
		Capability: capability,
		VersionA:   vA,
		VersionB:   vB,
	}

	// Compute deltas (B - A) for each layer
	if vA.L1PassRate != nil && vB.L1PassRate != nil {
		d := *vB.L1PassRate - *vA.L1PassRate
		result.ScoreDelta.L1 = &d
	}
	if vA.L2AvgScore != nil && vB.L2AvgScore != nil {
		d := *vB.L2AvgScore - *vA.L2AvgScore
		result.ScoreDelta.L2 = &d
	}
	if vA.L3AvgScore != nil && vB.L3AvgScore != nil {
		d := *vB.L3AvgScore - *vA.L3AvgScore
		result.ScoreDelta.L3 = &d
	}

	return result, nil
}

// ========== Top Violations API ==========

// TopViolation holds a single high-frequency violation entry
type TopViolation struct {
	RuleName string `json:"rule_name"`
	Count    int64  `json:"count"`
	Layer    string `json:"layer"` // rule | semantic
}

// GetTopViolations returns the most frequent rule violations across all assessments
func (s *SkillAssessmentService) GetTopViolations(capability string, days, limit int) ([]TopViolation, error) {
	if days <= 0 {
		days = 7
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	since := time.Now().AddDate(0, 0, -days)

	var results []TopViolation

	// Layer 2: rule_violations
	var ruleViolations []TopViolation
	query := `
		SELECT v->>'rule' as rule_name, count(*) as count, 'rule' as layer
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id,
		     jsonb_array_elements(r.rule_violations) as v
		WHERE r.assessment_layer = 'rule'
		  AND r.rule_violations IS NOT NULL AND r.rule_violations != 'null'
		  AND l.created_at > ?`
	args := []interface{}{since}
	if capability != "" {
		query += " AND l.capability = ?"
		args = append(args, capability)
	}
	query += " GROUP BY v->>'rule' ORDER BY count DESC LIMIT ?"
	args = append(args, limit)
	s.db.Raw(query, args...).Scan(&ruleViolations)
	results = append(results, ruleViolations...)

	// Layer 3: quality_issues
	var qualityIssues []TopViolation
	query2 := `
		SELECT COALESCE(v->>'field', 'general') || ': ' || COALESCE(LEFT(v->>'issue', 60), '') as rule_name,
		       count(*) as count, 'semantic' as layer
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id,
		     jsonb_array_elements(r.quality_issues) as v
		WHERE r.assessment_layer = 'semantic'
		  AND r.quality_issues IS NOT NULL AND r.quality_issues != 'null'
		  AND l.created_at > ?`
	args2 := []interface{}{since}
	if capability != "" {
		query2 += " AND l.capability = ?"
		args2 = append(args2, capability)
	}
	query2 += " GROUP BY rule_name ORDER BY count DESC LIMIT ?"
	args2 = append(args2, limit)
	s.db.Raw(query2, args2...).Scan(&qualityIssues)
	results = append(results, qualityIssues...)

	return results, nil
}

// ========== Skill Detail API ==========

// CapabilityDetail holds per-capability detail data
// FeedbackMatrix 评估结果 vs 用户反馈交叉矩阵
type FeedbackMatrix struct {
	PassPositive   int64 `json:"pass_positive"`   // 评估pass + 好评(>=4)
	PassNegative   int64 `json:"pass_negative"`   // 评估pass + 差评(<=2)
	PassNoFeedback int64 `json:"pass_no_feedback"` // 评估pass + 无反馈
	WarnPositive   int64 `json:"warn_positive"`
	WarnNegative   int64 `json:"warn_negative"`
	WarnNoFeedback int64 `json:"warn_no_feedback"`
	FailPositive   int64 `json:"fail_positive"`
	FailNegative   int64 `json:"fail_negative"`
	FailNoFeedback int64 `json:"fail_no_feedback"`
}

type CapabilityDetail struct {
	Capability     string               `json:"capability"`
	PassRate       float64              `json:"pass_rate"`
	Total          int64                `json:"total"`
	Pass           int64                `json:"pass"`
	Fail           int64                `json:"fail"`
	AvgScore       float64              `json:"avg_score"`
	AvgLatencyMs   float64              `json:"avg_latency_ms"`
	LatestHash     string               `json:"latest_hash"`
	TaskSkill      string               `json:"task_skill"`
	Versions        []VersionStats       `json:"versions"`
	Assessments     []AssessmentRecord   `json:"assessments"`
	AssessmentTotal int64                `json:"assessment_total"`
	FeedbackMatrix  *FeedbackMatrix      `json:"feedback_matrix"`
}

// VersionStats holds per content_hash stats
type VersionStats struct {
	ContentHash    string    `json:"content_hash"`
	Total          int64     `json:"total"`
	Pass           int64     `json:"pass"`
	Fail           int64     `json:"fail"`
	AvgScore       float64   `json:"avg_score"`
	PassRate       float64   `json:"pass_rate"`
	FirstSeen      time.Time `json:"first_seen"`
	L1PassRate     *float64  `json:"l1_pass_rate"`
	L2PassRate     *float64  `json:"l2_pass_rate"`
	L2AvgScore     *float64  `json:"l2_avg_score"`
	L3PassRate     *float64  `json:"l3_pass_rate"`
	L3AvgScore     *float64  `json:"l3_avg_score"`
}

// AssessmentRecord holds a single assessment record for the detail table
type AssessmentRecord struct {
	UsageLogID    string          `json:"usage_log_id"`
	Layer         string          `json:"layer"`
	Verdict       string          `json:"verdict"`
	Score         int             `json:"score"`
	LatencyMs     *int            `json:"latency_ms"`
	MissingFields json.RawMessage `json:"missing_fields"`
	InvalidEnums  json.RawMessage `json:"invalid_enum_fields"`
	AssessedAt    time.Time       `json:"assessed_at"`
	ContentHash   string          `json:"content_hash"`
}

// GetCapabilityDetail returns detail data for a single capability
func (s *SkillAssessmentService) GetCapabilityDetail(capability string, days, page, pageSize int) (*CapabilityDetail, error) {
	if days <= 0 {
		days = 7
	}
	since := time.Now().AddDate(0, 0, -days)

	detail := &CapabilityDetail{
		Capability:  capability,
		Versions:    []VersionStats{},
		Assessments: []AssessmentRecord{},
	}

	// 1. Aggregate stats for this capability
	type aggRow struct {
		Total        int64   `json:"total"`
		Pass         int64   `json:"pass"`
		Fail         int64   `json:"fail"`
		AvgScore     float64 `json:"avg_score"`
		AvgLatencyMs float64 `json:"avg_latency_ms"`
	}
	var agg aggRow
	if err := s.db.Raw(`
		SELECT count(*) as total,
		       count(CASE WHEN r.verdict = 'pass' THEN 1 END) as pass,
		       count(CASE WHEN r.verdict = 'fail' THEN 1 END) as fail,
		       COALESCE(avg(r.score), 0) as avg_score,
		       COALESCE(avg(l.latency_ms), 0) as avg_latency_ms
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.capability = ? AND l.created_at > ?`, capability, since).Scan(&agg).Error; err != nil {
		return nil, err
	}
	detail.Total = agg.Total
	detail.Pass = agg.Pass
	detail.Fail = agg.Fail
	detail.AvgScore = agg.AvgScore
	detail.AvgLatencyMs = agg.AvgLatencyMs
	if agg.Total > 0 {
		detail.PassRate = float64(agg.Pass) / float64(agg.Total) * 100
	}

	// 2. Version stats (per content_hash, with per-layer breakdown)
	type versionRow struct {
		ContentHash string    `json:"content_hash"`
		Total       int64     `json:"total"`
		Pass        int64     `json:"pass"`
		Fail        int64     `json:"fail"`
		AvgScore    float64   `json:"avg_score"`
		FirstSeen   time.Time `json:"first_seen"`
		L1Total     int64     `json:"l1_total"`
		L1Pass      int64     `json:"l1_pass"`
		L2Total     int64     `json:"l2_total"`
		L2Pass      int64     `json:"l2_pass"`
		L2AvgScore  float64   `json:"l2_avg_score"`
		L3Total     int64     `json:"l3_total"`
		L3Pass      int64     `json:"l3_pass"`
		L3AvgScore  float64   `json:"l3_avg_score"`
	}
	var vRows []versionRow
	if err := s.db.Raw(`
		SELECT l.skill_content_hash as content_hash,
		       count(*) as total,
		       count(CASE WHEN r.verdict = 'pass' THEN 1 END) as pass,
		       count(CASE WHEN r.verdict = 'fail' THEN 1 END) as fail,
		       COALESCE(avg(r.score), 0) as avg_score,
		       min(l.created_at) as first_seen,
		       count(CASE WHEN r.assessment_layer = 'schema' THEN 1 END) as l1_total,
		       count(CASE WHEN r.assessment_layer = 'schema' AND r.verdict = 'pass' THEN 1 END) as l1_pass,
		       count(CASE WHEN r.assessment_layer = 'rule' THEN 1 END) as l2_total,
		       count(CASE WHEN r.assessment_layer = 'rule' AND r.verdict = 'pass' THEN 1 END) as l2_pass,
		       COALESCE(avg(CASE WHEN r.assessment_layer = 'rule' THEN r.score END), 0) as l2_avg_score,
		       count(CASE WHEN r.assessment_layer = 'semantic' THEN 1 END) as l3_total,
		       count(CASE WHEN r.assessment_layer = 'semantic' AND r.verdict = 'pass' THEN 1 END) as l3_pass,
		       COALESCE(avg(CASE WHEN r.assessment_layer = 'semantic' THEN r.score END), 0) as l3_avg_score
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.capability = ? AND l.created_at > ? AND l.skill_content_hash != ''
		GROUP BY l.skill_content_hash
		ORDER BY first_seen DESC`, capability, since).Scan(&vRows).Error; err != nil {
		return nil, err
	}
	for _, v := range vRows {
		passRate := float64(0)
		if v.Total > 0 {
			passRate = float64(v.Pass) / float64(v.Total) * 100
		}
		vs := VersionStats{
			ContentHash: v.ContentHash,
			Total:       v.Total,
			Pass:        v.Pass,
			Fail:        v.Fail,
			AvgScore:    v.AvgScore,
			PassRate:    passRate,
			FirstSeen:   v.FirstSeen,
		}
		if v.L1Total > 0 {
			r := float64(v.L1Pass) / float64(v.L1Total) * 100
			vs.L1PassRate = &r
		}
		if v.L2Total > 0 {
			r := float64(v.L2Pass) / float64(v.L2Total) * 100
			vs.L2PassRate = &r
			vs.L2AvgScore = &v.L2AvgScore
		}
		if v.L3Total > 0 {
			r := float64(v.L3Pass) / float64(v.L3Total) * 100
			vs.L3PassRate = &r
			vs.L3AvgScore = &v.L3AvgScore
		}
		detail.Versions = append(detail.Versions, vs)
	}
	if len(vRows) > 0 {
		detail.LatestHash = vRows[0].ContentHash
	}

	// 3. Task skill name (from latest usage log)
	var taskSkill struct{ SkillName string }
	s.db.Raw(`
		SELECT r.skill_name
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.capability = ? AND l.created_at > ?
		ORDER BY r.assessed_at DESC LIMIT 1`, capability, since).Scan(&taskSkill)
	detail.TaskSkill = taskSkill.SkillName

	// 4. Assessments with pagination
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Total count
	s.db.Raw(`
		SELECT count(*)
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.capability = ? AND l.created_at > ?`, capability, since).Scan(&detail.AssessmentTotal)

	if err := s.db.Raw(`
		SELECT r.usage_log_id, r.assessment_layer as layer, r.verdict, r.score,
		       r.assessment_latency_ms as latency_ms,
		       COALESCE(array_to_json(r.missing_fields), '[]') as missing_fields,
		       COALESCE(array_to_json(r.invalid_enum_fields), '[]') as invalid_enum_fields,
		       r.assessed_at, r.skill_content_hash as content_hash
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.capability = ? AND l.created_at > ?
		ORDER BY r.assessed_at DESC LIMIT ? OFFSET ?`, capability, since, pageSize, offset).Scan(&detail.Assessments).Error; err != nil {
		return nil, err
	}

	// 5. Feedback matrix: 评估结果 vs 用户反馈
	var fm FeedbackMatrix
	s.db.Raw(`
		SELECT
		  count(CASE WHEN r.verdict='pass' AND l.user_feedback >= 4 THEN 1 END) as pass_positive,
		  count(CASE WHEN r.verdict='pass' AND l.user_feedback <= 2 THEN 1 END) as pass_negative,
		  count(CASE WHEN r.verdict='pass' AND l.user_feedback IS NULL THEN 1 END) as pass_no_feedback,
		  count(CASE WHEN r.verdict='warn' AND l.user_feedback >= 4 THEN 1 END) as warn_positive,
		  count(CASE WHEN r.verdict='warn' AND l.user_feedback <= 2 THEN 1 END) as warn_negative,
		  count(CASE WHEN r.verdict='warn' AND l.user_feedback IS NULL THEN 1 END) as warn_no_feedback,
		  count(CASE WHEN r.verdict='fail' AND l.user_feedback >= 4 THEN 1 END) as fail_positive,
		  count(CASE WHEN r.verdict='fail' AND l.user_feedback <= 2 THEN 1 END) as fail_negative,
		  count(CASE WHEN r.verdict='fail' AND l.user_feedback IS NULL THEN 1 END) as fail_no_feedback
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.capability = ? AND l.created_at > ?`, capability, since).Scan(&fm)
	detail.FeedbackMatrix = &fm

	return detail, nil
}
