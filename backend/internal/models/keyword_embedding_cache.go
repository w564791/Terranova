package models

import (
	"time"
)

// KeywordEmbeddingCache 关键词向量缓存
// 注意：Embedding 字段使用 gorm:"-" 忽略自动映射，因为 GORM 不支持 pgvector 类型的自动序列化/反序列化
// embedding 的读写使用原生 SQL 操作（如 UPDATE ... SET embedding = ?::vector）
type KeywordEmbeddingCache struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Keyword        string     `gorm:"size:500;not null" json:"keyword"`
	KeywordHash    string     `gorm:"size:64;uniqueIndex;not null" json:"keyword_hash"`
	Embedding      []float32  `gorm:"-" json:"-"` // 向量数据，使用原生 SQL 操作
	EmbeddingModel string     `gorm:"size:100;not null" json:"embedding_model"`
	HitCount       int        `gorm:"default:0" json:"hit_count"`
	LastHitAt      *time.Time `json:"last_hit_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 指定表名
func (KeywordEmbeddingCache) TableName() string {
	return "keyword_embedding_cache"
}

// EmbeddingCacheStats 缓存统计信息
type EmbeddingCacheStats struct {
	TotalCount  int64   `json:"total_count"`
	TotalHits   int64   `json:"total_hits"`
	AvgHitCount float64 `json:"avg_hit_count"`
	CacheSize   string  `json:"cache_size"`
	OldestEntry string  `json:"oldest_entry"`
	NewestEntry string  `json:"newest_entry"`
	TopKeywords []struct {
		Keyword  string `json:"keyword"`
		HitCount int    `json:"hit_count"`
	} `json:"top_keywords"`
}
