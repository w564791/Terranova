package services

import (
	"fmt"
	"iac-platform/internal/models"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// AIFeatureService AI 能力开关服务
type AIFeatureService struct {
	db    *gorm.DB
	cache map[string]*featureCache
	mu    sync.RWMutex
}

type featureCache struct {
	enabled   bool
	expiresAt time.Time
}

const featureCacheTTL = 30 * time.Second

// NewAIFeatureService 创建 AI 能力开关服务
func NewAIFeatureService(db *gorm.DB) *AIFeatureService {
	return &AIFeatureService{
		db:    db,
		cache: make(map[string]*featureCache),
	}
}

// IsFeatureEnabled 检查某个 AI 能力是否启用（带 30 秒缓存）
func (s *AIFeatureService) IsFeatureEnabled(feature string) bool {
	key := "ai_feature_" + feature

	// 读缓存
	s.mu.RLock()
	if cached, ok := s.cache[key]; ok && time.Now().Before(cached.expiresAt) {
		s.mu.RUnlock()
		return cached.enabled
	}
	s.mu.RUnlock()

	// 查 DB
	var config models.SystemConfig
	enabled := true // 默认启用
	if err := s.db.Where("key = ? AND deleted_at IS NULL", key).First(&config).Error; err == nil {
		enabled = config.Value != `"false"` && config.Value != "false"
	}

	// 写缓存
	s.mu.Lock()
	s.cache[key] = &featureCache{
		enabled:   enabled,
		expiresAt: time.Now().Add(featureCacheTTL),
	}
	s.mu.Unlock()

	return enabled
}

// GetAllFeatures 获取所有 AI 能力开关状态
func (s *AIFeatureService) GetAllFeatures() map[string]bool {
	return map[string]bool{
		"embedding":              s.IsFeatureEnabled("embedding"),
		"cmdb_resource_summary":  s.IsFeatureEnabled("cmdb_resource_summary"),
		"execute_summary":        s.IsFeatureEnabled("execute_summary"),
	}
}

// UpdateFeatures 批量更新能力开关（只更新请求中包含的字段）
func (s *AIFeatureService) UpdateFeatures(features map[string]bool) error {
	validKeys := map[string]bool{
		"embedding":              true,
		"cmdb_resource_summary":  true,
		"execute_summary":        true,
	}

	for feature, enabled := range features {
		if !validKeys[feature] {
			continue
		}

		key := "ai_feature_" + feature
		value := fmt.Sprintf(`"%t"`, enabled)

		// Upsert
		var config models.SystemConfig
		if err := s.db.Where("key = ? AND deleted_at IS NULL", key).First(&config).Error; err != nil {
			// 不存在，创建
			config = models.SystemConfig{Key: key, Value: value}
			if err := s.db.Create(&config).Error; err != nil {
				return fmt.Errorf("failed to create feature config %s: %w", key, err)
			}
		} else {
			// 存在，更新
			if err := s.db.Model(&config).Update("value", value).Error; err != nil {
				return fmt.Errorf("failed to update feature config %s: %w", key, err)
			}
		}

		// 清缓存
		s.mu.Lock()
		delete(s.cache, key)
		s.mu.Unlock()

		log.Printf("[AIFeature] Feature %s set to %t", feature, enabled)
	}

	return nil
}
