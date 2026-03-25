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
	DailyTrend     []DailyTrendItem   `json:"daily_trend"`
}

// CapabilityStats holds per-capability verdict breakdown
type CapabilityStats struct {
	Capability string  `json:"capability"`
	Total      int64   `json:"total"`
	Pass       int64   `json:"pass"`
	Fail       int64   `json:"fail"`
	Warn       int64   `json:"warn"`
	PassRate   float64 `json:"pass_rate"`
}

// RecentFailure holds details of a recent failed assessment
type RecentFailure struct {
	UsageLogID    string          `json:"usage_log_id"`
	Capability    string          `json:"capability"`
	SkillName     string          `json:"skill_name"`
	Verdict       string          `json:"verdict"`
	MissingFields json.RawMessage `json:"missing_fields"`
	InvalidEnums  json.RawMessage `json:"invalid_enum_fields"`
	AssessedAt    time.Time       `json:"assessed_at"`
	ContentHash   string          `json:"content_hash"`
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

// capabilityVerdictRow is a helper for scanning capability × verdict results
type capabilityVerdictRow struct {
	Capability string
	Verdict    string
	Cnt        int64
}

// dailyVerdictRow is a helper for scanning daily trend results
type dailyVerdictRow struct {
	Date    string
	Verdict string
	Cnt     int64
}

// GetOverview returns aggregated assessment data for the given time window
func (s *SkillAssessmentService) GetOverview(days int) (*AssessmentOverview, error) {
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

	// 4. By capability
	var capRows []capabilityVerdictRow
	if err := s.db.Raw(`
		SELECT l.capability, r.verdict, count(*) as cnt
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE l.created_at > ?
		GROUP BY l.capability, r.verdict`, since).Scan(&capRows).Error; err != nil {
		return nil, err
	}

	capMap := make(map[string]*CapabilityStats)
	for _, row := range capRows {
		cs, ok := capMap[row.Capability]
		if !ok {
			cs = &CapabilityStats{Capability: row.Capability}
			capMap[row.Capability] = cs
		}
		cs.Total += row.Cnt
		switch models.AssessmentVerdict(row.Verdict) {
		case models.AssessmentVerdictPass:
			cs.Pass = row.Cnt
		case models.AssessmentVerdictFail:
			cs.Fail = row.Cnt
		case models.AssessmentVerdictWarn:
			cs.Warn = row.Cnt
		}
	}
	for _, cs := range capMap {
		if cs.Total > 0 {
			cs.PassRate = float64(cs.Pass) / float64(cs.Total) * 100
		}
		overview.ByCapability = append(overview.ByCapability, *cs)
	}

	// 5. High risk skills (fail rate > 20%)
	for _, cs := range overview.ByCapability {
		if cs.Total > 0 && float64(cs.Fail)/float64(cs.Total)*100 > 20 {
			overview.HighRiskSkills = append(overview.HighRiskSkills, cs.Capability)
		}
	}

	// 6. Recent failures (limit 10, 按调用时间过滤)
	if err := s.db.Raw(`
		SELECT r.usage_log_id, l.capability, r.skill_name, r.verdict,
		       COALESCE(array_to_json(r.missing_fields), '[]') as missing_fields,
		       COALESCE(array_to_json(r.invalid_enum_fields), '[]') as invalid_enum_fields,
		       r.assessed_at,
		       r.skill_content_hash as content_hash
		FROM skill_assessment_results r
		JOIN skill_usage_logs l ON l.id = r.usage_log_id
		WHERE r.verdict = 'fail' AND l.created_at > ?
		ORDER BY r.assessed_at DESC LIMIT 10`, since).Scan(&overview.RecentFailures).Error; err != nil {
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

	return overview, nil
}
