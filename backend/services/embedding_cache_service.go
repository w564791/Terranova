package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// WarmupProgress 预热进度
type WarmupProgress struct {
	IsRunning      bool      `json:"is_running"`
	TotalKeywords  int       `json:"total_keywords"`
	ProcessedCount int       `json:"processed_count"`
	CachedCount    int       `json:"cached_count"`
	NewCount       int       `json:"new_count"`
	FailedCount    int       `json:"failed_count"`
	CurrentBatch   int       `json:"current_batch"`
	TotalBatches   int       `json:"total_batches"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	InternalCount  int       `json:"internal_count"` // 内部 CMDB 关键词数
	ExternalCount  int       `json:"external_count"` // 外部 CMDB 关键词数
	StaticCount    int       `json:"static_count"`   // 静态词库数
}

// EmbeddingCacheService 向量缓存服务
type EmbeddingCacheService struct {
	db               *gorm.DB
	embeddingService *EmbeddingService
	memoryCache      sync.Map // 内存缓存，key: keyword_hash, value: []float32
	currentModel     string   // 当前使用的 embedding 模型
	warmupProgress   *WarmupProgress
	warmupMutex      sync.Mutex
}

// NewEmbeddingCacheService 创建缓存服务实例
func NewEmbeddingCacheService(db *gorm.DB, embeddingService *EmbeddingService) *EmbeddingCacheService {
	return &EmbeddingCacheService{
		db:               db,
		embeddingService: embeddingService,
	}
}

// hashKeyword 计算关键词的 SHA256 哈希
func (s *EmbeddingCacheService) hashKeyword(keyword string) string {
	// 标准化关键词：小写、去除首尾空格
	normalized := strings.ToLower(strings.TrimSpace(keyword))
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// GetEmbedding 获取关键词的向量（优先从缓存）
func (s *EmbeddingCacheService) GetEmbedding(keyword string) ([]float32, error) {
	keywordHash := s.hashKeyword(keyword)

	// 1. 先查内存缓存
	if cached, ok := s.memoryCache.Load(keywordHash); ok {
		log.Printf("[EmbeddingCache] 内存缓存命中: %s", keyword)
		return cached.([]float32), nil
	}

	// 2. 再查数据库缓存
	embedding, err := s.getFromDBCache(keywordHash)
	if err == nil && len(embedding) > 0 {
		log.Printf("[EmbeddingCache] 数据库缓存命中: %s", keyword)
		// 更新命中计数（异步）
		go s.updateHitCount(keywordHash)
		// 存入内存缓存
		s.memoryCache.Store(keywordHash, embedding)
		return embedding, nil
	}

	// 3. 缓存未命中，调用 API
	log.Printf("[EmbeddingCache] 缓存未命中，调用 API: %s", keyword)
	embedding, err = s.embeddingService.GenerateEmbedding(keyword)
	if err != nil {
		return nil, err
	}

	// 4. 异步存入缓存
	go s.saveToCache(keyword, keywordHash, embedding)

	return embedding, nil
}

// GetEmbeddingsBatch 批量获取关键词的向量（优先从缓存）
func (s *EmbeddingCacheService) GetEmbeddingsBatch(keywords []string) ([][]float32, error) {
	results := make([][]float32, len(keywords))
	var uncachedKeywords []string
	var uncachedIndices []int

	// 1. 先从缓存获取
	for i, keyword := range keywords {
		keywordHash := s.hashKeyword(keyword)

		// 检查内存缓存
		if cached, ok := s.memoryCache.Load(keywordHash); ok {
			results[i] = cached.([]float32)
			log.Printf("[EmbeddingCache] 内存缓存命中: %s", keyword)
			continue
		}

		// 检查数据库缓存
		embedding, err := s.getFromDBCache(keywordHash)
		if err == nil && len(embedding) > 0 {
			results[i] = embedding
			s.memoryCache.Store(keywordHash, embedding)
			go s.updateHitCount(keywordHash)
			log.Printf("[EmbeddingCache] 数据库缓存命中: %s", keyword)
			continue
		}

		// 记录未命中的关键词
		uncachedKeywords = append(uncachedKeywords, keyword)
		uncachedIndices = append(uncachedIndices, i)
	}

	// 2. 批量调用 API 获取未缓存的向量
	if len(uncachedKeywords) > 0 {
		log.Printf("[EmbeddingCache] 批量调用 API: %d 个关键词", len(uncachedKeywords))
		embeddings, err := s.embeddingService.GenerateEmbeddingsBatch(uncachedKeywords)
		if err != nil {
			// 降级到逐个调用
			log.Printf("[EmbeddingCache] 批量 API 失败，降级到逐个调用: %v", err)
			for j, keyword := range uncachedKeywords {
				embedding, err := s.embeddingService.GenerateEmbedding(keyword)
				if err != nil {
					log.Printf("[EmbeddingCache] 关键词 '%s' 生成失败: %v", keyword, err)
					continue
				}
				results[uncachedIndices[j]] = embedding
				// 异步存入缓存
				go s.saveToCache(keyword, s.hashKeyword(keyword), embedding)
			}
		} else {
			// 批量成功
			for j, embedding := range embeddings {
				if len(embedding) > 0 {
					results[uncachedIndices[j]] = embedding
					// 异步存入缓存
					keyword := uncachedKeywords[j]
					go s.saveToCache(keyword, s.hashKeyword(keyword), embedding)
				}
			}
		}
	}

	return results, nil
}

// getFromDBCache 从数据库缓存获取向量
func (s *EmbeddingCacheService) getFromDBCache(keywordHash string) ([]float32, error) {
	var result struct {
		Embedding string `gorm:"column:embedding"`
	}

	err := s.db.Table("keyword_embedding_cache").
		Select("embedding::text as embedding").
		Where("keyword_hash = ?", keywordHash).
		First(&result).Error

	if err != nil {
		return nil, err
	}

	// 解析向量字符串 "[0.1,0.2,...]"
	return parseVectorString(result.Embedding), nil
}

// saveToCache 保存向量到缓存
func (s *EmbeddingCacheService) saveToCache(keyword, keywordHash string, embedding []float32) {
	// 获取当前模型 ID
	modelID := s.getCurrentModelID()

	// 构建向量字符串
	vectorStr := VectorToString(embedding)

	// 使用原生 SQL 插入（因为 GORM 不支持 pgvector 类型）
	sql := `
		INSERT INTO keyword_embedding_cache (keyword, keyword_hash, embedding, embedding_model, created_at, updated_at)
		VALUES (?, ?, ?::vector, ?, NOW(), NOW())
		ON CONFLICT (keyword_hash) DO UPDATE SET
			embedding = EXCLUDED.embedding,
			embedding_model = EXCLUDED.embedding_model,
			updated_at = NOW()
	`

	if err := s.db.Exec(sql, keyword, keywordHash, vectorStr, modelID).Error; err != nil {
		log.Printf("[EmbeddingCache] 保存缓存失败: %v", err)
	} else {
		log.Printf("[EmbeddingCache] 缓存已保存: %s", keyword)
		// 同时存入内存缓存
		s.memoryCache.Store(keywordHash, embedding)
	}
}

// updateHitCount 更新命中计数
func (s *EmbeddingCacheService) updateHitCount(keywordHash string) {
	sql := `
		UPDATE keyword_embedding_cache 
		SET hit_count = hit_count + 1, last_hit_at = NOW()
		WHERE keyword_hash = ?
	`
	s.db.Exec(sql, keywordHash)
}

// getCurrentModelID 获取当前使用的 embedding 模型 ID
func (s *EmbeddingCacheService) getCurrentModelID() string {
	if s.currentModel != "" {
		return s.currentModel
	}

	// 从配置服务获取
	configService := NewAIConfigService(s.db)
	config, err := configService.GetConfigForCapability("embedding")
	if err == nil && config != nil {
		s.currentModel = config.ModelID
		return s.currentModel
	}

	return "unknown"
}

// GetWarmupProgress 获取预热进度
func (s *EmbeddingCacheService) GetWarmupProgress() *WarmupProgress {
	s.warmupMutex.Lock()
	defer s.warmupMutex.Unlock()

	if s.warmupProgress == nil {
		return &WarmupProgress{IsRunning: false}
	}
	// 返回副本
	progress := *s.warmupProgress
	return &progress
}

// IsWarmupRunning 检查预热是否正在运行
func (s *EmbeddingCacheService) IsWarmupRunning() bool {
	s.warmupMutex.Lock()
	defer s.warmupMutex.Unlock()
	return s.warmupProgress != nil && s.warmupProgress.IsRunning
}

// WarmUp 预热缓存
func (s *EmbeddingCacheService) WarmUp() error {
	return s.WarmUpWithForce(false)
}

// WarmUpWithForce 预热缓存（可选强制重新生成）
func (s *EmbeddingCacheService) WarmUpWithForce(force bool) error {
	// 检查是否已在运行
	if s.IsWarmupRunning() {
		return fmt.Errorf("预热任务已在运行中")
	}

	// 初始化进度
	s.warmupMutex.Lock()
	s.warmupProgress = &WarmupProgress{
		IsRunning: true,
		StartedAt: time.Now(),
	}
	s.warmupMutex.Unlock()

	log.Printf("[EmbeddingCache] 开始预热缓存... (force=%v)", force)

	// 1. 预热静态词库
	staticKeywords := s.getStaticKeywords()
	log.Printf("[EmbeddingCache] 静态词库: %d 个关键词", len(staticKeywords))

	// 2. 从资源索引提取高频资源名称（带统计）
	dynamicKeywords, internalCount, externalCount := s.getDynamicKeywordsWithStats()
	log.Printf("[EmbeddingCache] 动态词库: %d 个关键词 (内部: %d, 外部: %d)",
		len(dynamicKeywords), internalCount, externalCount)

	// 合并并去重
	allKeywords := make(map[string]bool)
	for _, kw := range staticKeywords {
		allKeywords[kw] = true
	}
	for _, kw := range dynamicKeywords {
		allKeywords[kw] = true
	}

	var keywords []string
	for kw := range allKeywords {
		keywords = append(keywords, kw)
	}

	log.Printf("[EmbeddingCache] 总计需要预热: %d 个关键词", len(keywords))

	// 更新进度
	batchSize := 20
	totalBatches := (len(keywords) + batchSize - 1) / batchSize

	s.warmupMutex.Lock()
	s.warmupProgress.TotalKeywords = len(keywords)
	s.warmupProgress.TotalBatches = totalBatches
	s.warmupProgress.StaticCount = len(staticKeywords)
	s.warmupProgress.InternalCount = internalCount
	s.warmupProgress.ExternalCount = externalCount
	s.warmupMutex.Unlock()

	// 3. 批量生成向量
	for i := 0; i < len(keywords); i += batchSize {
		end := i + batchSize
		if end > len(keywords) {
			end = len(keywords)
		}
		batch := keywords[i:end]
		currentBatch := i/batchSize + 1

		log.Printf("[EmbeddingCache] 预热批次 %d/%d: %d 个关键词", currentBatch, totalBatches, len(batch))

		// 更新当前批次
		s.warmupMutex.Lock()
		s.warmupProgress.CurrentBatch = currentBatch
		s.warmupMutex.Unlock()

		// 处理批次
		cachedInBatch := 0
		newInBatch := 0
		failedInBatch := 0

		if force {
			// 强制模式：直接调用 API 生成新向量
			embeddings, err := s.embeddingService.GenerateEmbeddingsBatch(batch)
			if err != nil {
				log.Printf("[EmbeddingCache] 预热批次失败: %v", err)
				failedInBatch = len(batch)
				s.warmupMutex.Lock()
				s.warmupProgress.LastError = err.Error()
				s.warmupMutex.Unlock()
			} else {
				for j, embedding := range embeddings {
					if len(embedding) > 0 {
						s.saveToCache(batch[j], s.hashKeyword(batch[j]), embedding)
						newInBatch++
					} else {
						failedInBatch++
					}
				}
			}
		} else {
			// 普通模式：优先使用缓存
			for _, keyword := range batch {
				keywordHash := s.hashKeyword(keyword)

				// 检查缓存
				_, err := s.getFromDBCache(keywordHash)
				if err == nil {
					cachedInBatch++
					continue
				}

				// 缓存未命中，生成新向量
				embedding, err := s.embeddingService.GenerateEmbedding(keyword)
				if err != nil {
					log.Printf("[EmbeddingCache] 关键词 '%s' 生成失败: %v", keyword, err)
					failedInBatch++
					continue
				}
				s.saveToCache(keyword, keywordHash, embedding)
				newInBatch++
			}
		}

		// 更新进度
		s.warmupMutex.Lock()
		s.warmupProgress.ProcessedCount += len(batch)
		s.warmupProgress.CachedCount += cachedInBatch
		s.warmupProgress.NewCount += newInBatch
		s.warmupProgress.FailedCount += failedInBatch
		s.warmupMutex.Unlock()

		// 避免 API 限流
		time.Sleep(500 * time.Millisecond)
	}

	// 完成
	s.warmupMutex.Lock()
	s.warmupProgress.IsRunning = false
	s.warmupProgress.CompletedAt = time.Now()
	s.warmupMutex.Unlock()

	log.Printf("[EmbeddingCache] 预热完成: 总计 %d, 缓存命中 %d, 新生成 %d, 失败 %d",
		s.warmupProgress.TotalKeywords,
		s.warmupProgress.CachedCount,
		s.warmupProgress.NewCount,
		s.warmupProgress.FailedCount)

	return nil
}

// getDynamicKeywordsWithStats 从资源索引提取高频资源名称（带统计）
func (s *EmbeddingCacheService) getDynamicKeywordsWithStats() (keywords []string, internalCount, externalCount int) {
	// 1. 从内部 CMDB (resource_index) 提取高频资源名称
	var internalNames []string
	s.db.Table("resource_index").
		Select("DISTINCT cloud_resource_name").
		Where("cloud_resource_name IS NOT NULL AND cloud_resource_name != '' AND (source_type IS NULL OR source_type != 'external')").
		Limit(100).
		Pluck("cloud_resource_name", &internalNames)
	keywords = append(keywords, internalNames...)
	internalCount += len(internalNames)

	// 2. 从内部 CMDB 提取资源 ID
	var internalIDs []string
	s.db.Table("resource_index").
		Select("DISTINCT cloud_resource_id").
		Where("cloud_resource_id IS NOT NULL AND cloud_resource_id != '' AND (source_type IS NULL OR source_type != 'external')").
		Limit(100).
		Pluck("cloud_resource_id", &internalIDs)
	keywords = append(keywords, internalIDs...)
	internalCount += len(internalIDs)

	// 3. 从内部 CMDB 提取描述
	var internalDescs []string
	s.db.Table("resource_index").
		Select("DISTINCT description").
		Where("description IS NOT NULL AND description != '' AND LENGTH(description) < 100 AND (source_type IS NULL OR source_type != 'external')").
		Limit(50).
		Pluck("description", &internalDescs)
	keywords = append(keywords, internalDescs...)
	internalCount += len(internalDescs)

	log.Printf("[EmbeddingCache] 内部 CMDB: 名称 %d, ID %d, 描述 %d",
		len(internalNames), len(internalIDs), len(internalDescs))

	// 4. 从外部 CMDB (cmdb_external_sources) 检查是否有外部数据源
	var externalSourceCount int64
	s.db.Table("cmdb_external_sources").Where("is_enabled = true").Count(&externalSourceCount)

	if externalSourceCount > 0 {
		// 5. 从外部 CMDB 资源提取名称
		var externalNames []string
		s.db.Table("resource_index").
			Select("DISTINCT cloud_resource_name").
			Where("source_type = 'external' AND cloud_resource_name IS NOT NULL AND cloud_resource_name != ''").
			Limit(100).
			Pluck("cloud_resource_name", &externalNames)
		keywords = append(keywords, externalNames...)
		externalCount += len(externalNames)

		// 6. 从外部 CMDB 提取资源 ID
		var externalIDs []string
		s.db.Table("resource_index").
			Select("DISTINCT cloud_resource_id").
			Where("source_type = 'external' AND cloud_resource_id IS NOT NULL AND cloud_resource_id != ''").
			Limit(100).
			Pluck("cloud_resource_id", &externalIDs)
		keywords = append(keywords, externalIDs...)
		externalCount += len(externalIDs)

		// 7. 从外部 CMDB 提取描述
		var externalDescs []string
		s.db.Table("resource_index").
			Select("DISTINCT description").
			Where("source_type = 'external' AND description IS NOT NULL AND description != '' AND LENGTH(description) < 100").
			Limit(50).
			Pluck("description", &externalDescs)
		keywords = append(keywords, externalDescs...)
		externalCount += len(externalDescs)

		// 8. 提取外部数据源的账户 ID
		var accountIDs []string
		s.db.Table("cmdb_external_sources").
			Select("DISTINCT account_id").
			Where("account_id IS NOT NULL AND account_id != ''").
			Pluck("account_id", &accountIDs)
		keywords = append(keywords, accountIDs...)
		externalCount += len(accountIDs)

		log.Printf("[EmbeddingCache] 外部 CMDB: 名称 %d, ID %d, 描述 %d, 账户 %d",
			len(externalNames), len(externalIDs), len(externalDescs), len(accountIDs))
	}

	return keywords, internalCount, externalCount
}

// getStaticKeywords 获取静态词库
func (s *EmbeddingCacheService) getStaticKeywords() []string {
	return []string{
		// 地区
		"tokyo", "东京", "singapore", "新加坡", "seoul", "首尔",
		"hong kong", "香港", "shanghai", "上海", "beijing", "北京",
		"us-east", "美东", "us-west", "美西", "europe", "欧洲",
		"frankfurt", "法兰克福", "london", "伦敦", "sydney", "悉尼",
		// 可用区
		"ap-northeast-1a", "ap-northeast-1c", "ap-northeast-1d",
		"ap-southeast-1a", "ap-southeast-1b", "ap-southeast-1c",
		"us-east-1a", "us-east-1b", "us-east-1c",
		// 环境
		"production", "生产", "development", "开发", "test", "测试",
		"staging", "预发布", "demo", "演示",
		// 资源类型
		"private", "私有", "public", "公有",
		"database", "数据库", "network", "网络",
		"security", "安全", "web", "api",
		// 常用业务词
		"exchange", "trading", "payment", "order",
		"user", "admin", "backend", "frontend",
	}
}

// getDynamicKeywords 从资源索引提取高频资源名称（包括内部和外部 CMDB）
func (s *EmbeddingCacheService) getDynamicKeywords() []string {
	var keywords []string

	// 1. 从内部 CMDB (resource_index) 提取高频资源名称
	var internalNames []string
	s.db.Table("resource_index").
		Select("DISTINCT cloud_resource_name").
		Where("cloud_resource_name IS NOT NULL AND cloud_resource_name != ''").
		Limit(100).
		Pluck("cloud_resource_name", &internalNames)
	keywords = append(keywords, internalNames...)
	log.Printf("[EmbeddingCache] 内部 CMDB 资源名称: %d 个", len(internalNames))

	// 2. 从内部 CMDB 提取资源 ID
	var internalIDs []string
	s.db.Table("resource_index").
		Select("DISTINCT cloud_resource_id").
		Where("cloud_resource_id IS NOT NULL AND cloud_resource_id != ''").
		Limit(100).
		Pluck("cloud_resource_id", &internalIDs)
	keywords = append(keywords, internalIDs...)
	log.Printf("[EmbeddingCache] 内部 CMDB 资源 ID: %d 个", len(internalIDs))

	// 3. 从内部 CMDB 提取描述
	var internalDescs []string
	s.db.Table("resource_index").
		Select("DISTINCT description").
		Where("description IS NOT NULL AND description != '' AND LENGTH(description) < 100").
		Limit(50).
		Pluck("description", &internalDescs)
	keywords = append(keywords, internalDescs...)
	log.Printf("[EmbeddingCache] 内部 CMDB 描述: %d 个", len(internalDescs))

	// 4. 从外部 CMDB (cmdb_external_sources) 检查是否有外部数据源
	var externalSourceCount int64
	s.db.Table("cmdb_external_sources").Where("is_enabled = true").Count(&externalSourceCount)

	if externalSourceCount > 0 {
		// 5. 从外部 CMDB 资源提取名称（外部资源也存储在 resource_index 中，source_type = 'external'）
		var externalNames []string
		s.db.Table("resource_index").
			Select("DISTINCT cloud_resource_name").
			Where("source_type = 'external' AND cloud_resource_name IS NOT NULL AND cloud_resource_name != ''").
			Limit(100).
			Pluck("cloud_resource_name", &externalNames)
		keywords = append(keywords, externalNames...)
		log.Printf("[EmbeddingCache] 外部 CMDB 资源名称: %d 个", len(externalNames))

		// 6. 从外部 CMDB 提取资源 ID
		var externalIDs []string
		s.db.Table("resource_index").
			Select("DISTINCT cloud_resource_id").
			Where("source_type = 'external' AND cloud_resource_id IS NOT NULL AND cloud_resource_id != ''").
			Limit(100).
			Pluck("cloud_resource_id", &externalIDs)
		keywords = append(keywords, externalIDs...)
		log.Printf("[EmbeddingCache] 外部 CMDB 资源 ID: %d 个", len(externalIDs))

		// 7. 从外部 CMDB 提取描述
		var externalDescs []string
		s.db.Table("resource_index").
			Select("DISTINCT description").
			Where("source_type = 'external' AND description IS NOT NULL AND description != '' AND LENGTH(description) < 100").
			Limit(50).
			Pluck("description", &externalDescs)
		keywords = append(keywords, externalDescs...)
		log.Printf("[EmbeddingCache] 外部 CMDB 描述: %d 个", len(externalDescs))

		// 8. 提取外部数据源的账户 ID 和区域
		var accountIDs []string
		s.db.Table("cmdb_external_sources").
			Select("DISTINCT account_id").
			Where("account_id IS NOT NULL AND account_id != ''").
			Pluck("account_id", &accountIDs)
		keywords = append(keywords, accountIDs...)
		log.Printf("[EmbeddingCache] 外部 CMDB 账户 ID: %d 个", len(accountIDs))
	}

	return keywords
}

// GetStats 获取缓存统计信息
func (s *EmbeddingCacheService) GetStats() (*models.EmbeddingCacheStats, error) {
	stats := &models.EmbeddingCacheStats{}

	// 总数
	s.db.Table("keyword_embedding_cache").Count(&stats.TotalCount)

	// 总命中次数
	s.db.Table("keyword_embedding_cache").Select("COALESCE(SUM(hit_count), 0)").Scan(&stats.TotalHits)

	// 平均命中次数
	s.db.Table("keyword_embedding_cache").Select("COALESCE(AVG(hit_count), 0)").Scan(&stats.AvgHitCount)

	// 缓存大小（估算）
	var sizeBytes int64
	s.db.Raw("SELECT pg_total_relation_size('keyword_embedding_cache')").Scan(&sizeBytes)
	stats.CacheSize = formatBytes(sizeBytes)

	// 最早和最新条目
	var oldest, newest time.Time
	s.db.Table("keyword_embedding_cache").Select("MIN(created_at)").Scan(&oldest)
	s.db.Table("keyword_embedding_cache").Select("MAX(created_at)").Scan(&newest)
	if !oldest.IsZero() {
		stats.OldestEntry = oldest.Format(time.RFC3339)
	}
	if !newest.IsZero() {
		stats.NewestEntry = newest.Format(time.RFC3339)
	}

	// Top 10 关键词
	var topKeywords []struct {
		Keyword  string `gorm:"column:keyword"`
		HitCount int    `gorm:"column:hit_count"`
	}
	s.db.Table("keyword_embedding_cache").
		Select("keyword, hit_count").
		Order("hit_count DESC").
		Limit(10).
		Find(&topKeywords)

	for _, kw := range topKeywords {
		stats.TopKeywords = append(stats.TopKeywords, struct {
			Keyword  string `json:"keyword"`
			HitCount int    `json:"hit_count"`
		}{
			Keyword:  kw.Keyword,
			HitCount: kw.HitCount,
		})
	}

	return stats, nil
}

// ClearCache 清空缓存
func (s *EmbeddingCacheService) ClearCache() error {
	// 清空内存缓存
	s.memoryCache = sync.Map{}

	// 清空数据库缓存
	return s.db.Exec("TRUNCATE TABLE keyword_embedding_cache").Error
}

// CleanupLowHitCache 清理低命中率的缓存
func (s *EmbeddingCacheService) CleanupLowHitCache(minHitCount int, olderThanDays int) (int64, error) {
	result := s.db.Exec(`
		DELETE FROM keyword_embedding_cache 
		WHERE hit_count < ? AND created_at < NOW() - INTERVAL '? days'
	`, minHitCount, olderThanDays)

	return result.RowsAffected, result.Error
}

// formatBytes 格式化字节数
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// parseVectorString 解析向量字符串
func parseVectorString(s string) []float32 {
	// 移除方括号
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")

	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]float32, len(parts))
	for i, p := range parts {
		var f float64
		fmt.Sscanf(strings.TrimSpace(p), "%f", &f)
		result[i] = float32(f)
	}
	return result
}
