package models

import (
	"time"

	"gorm.io/gorm"
)

// CMDBSearchLog 搜索日志
type CMDBSearchLog struct {
	ID             int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Query          string    `json:"query" gorm:"type:text;not null"`
	ResourceType   string    `json:"resource_type" gorm:"type:varchar(100)"`
	SearchMethod   string    `json:"search_method" gorm:"type:varchar(20);not null"`
	Source         string    `json:"source" gorm:"type:varchar(10);not null;default:'manual'"`
	TotalCount     int       `json:"total_count" gorm:"not null;default:0"`
	VectorCount    int       `json:"vector_count" gorm:"not null;default:0"`
	KeywordCount   int       `json:"keyword_count" gorm:"not null;default:0"`
	TopSimilarity  float32   `json:"top_similarity" gorm:"default:0"`
	AvgSimilarity  float32   `json:"avg_similarity" gorm:"default:0"`
	DurationMs     int       `json:"duration_ms" gorm:"not null;default:0"`
	FallbackReason string    `json:"fallback_reason" gorm:"type:varchar(200)"`
	UserID         string    `json:"user_id" gorm:"type:varchar(100)"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (CMDBSearchLog) TableName() string {
	return "cmdb_search_logs"
}

// BeforeCreate 按字符数截断超长查询，防止粘贴整段资源摘要等导致日志膨胀/展示越界。
// 按 rune 截断而非 byte，避免截断中文产生乱码。
func (l *CMDBSearchLog) BeforeCreate(tx *gorm.DB) error {
	const maxQueryRunes = 120
	r := []rune(l.Query)
	if len(r) > maxQueryRunes {
		l.Query = string(r[:maxQueryRunes])
	}
	return nil
}

// CMDBSearchAnalytics 搜索分析聚合结果
type CMDBSearchAnalytics struct {
	Period            string                     `json:"period"`
	Usage             CMDBSearchUsage            `json:"usage"`
	Quality           CMDBSearchQuality          `json:"quality"`
	TopQueries        []CMDBSearchQueryStat      `json:"top_queries"`
	ZeroResultQueries []CMDBSearchZeroResultStat `json:"zero_result_queries"`
}

// CMDBSearchUsage 搜索使用统计
type CMDBSearchUsage struct {
	TotalSearches   int64   `json:"total_searches"`
	ZeroResultCount int64   `json:"zero_result_count"`
	ZeroResultRate  float64 `json:"zero_result_rate"`
	AvgResultCount  float64 `json:"avg_result_count"`
	UniqueQueries   int64   `json:"unique_queries"`
}

// CMDBSearchQuality 搜索质量指标
type CMDBSearchQuality struct {
	MethodDistribution map[string]int64 `json:"method_distribution"`
	AvgTopSimilarity   float64          `json:"avg_top_similarity"`
	AvgSimilarity      float64          `json:"avg_similarity"`
	FallbackRate       float64          `json:"fallback_rate"`
	AvgDurationMs      float64          `json:"avg_duration_ms"`
}

// CMDBSearchQueryStat 热门查询统计
type CMDBSearchQueryStat struct {
	Query      string  `json:"query" gorm:"column:query"`
	Count      int64   `json:"count" gorm:"column:count"`
	AvgResults float64 `json:"avg_results" gorm:"column:avg_results"`
}

// CMDBSearchZeroResultStat 零结果查询统计
type CMDBSearchZeroResultStat struct {
	Query  string    `json:"query" gorm:"column:query"`
	Count  int64     `json:"count" gorm:"column:count"`
	LastAt time.Time `json:"last_at" gorm:"column:last_at"`
}
