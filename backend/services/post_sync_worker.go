package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// PostSyncWorker CMDB 同步后处理 worker（守护协程）
// 按 source_id 串行消费 summary → embedding 任务
type PostSyncWorker struct {
	db               *gorm.DB
	embeddingService *EmbeddingService
	running          bool
	mu               sync.Mutex
	ctx              context.Context
	cancel           context.CancelFunc
}

func NewPostSyncWorker(db *gorm.DB) *PostSyncWorker {
	return &PostSyncWorker{
		db:               db,
		embeddingService: NewEmbeddingService(db),
	}
}

func (w *PostSyncWorker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		log.Println("[PostSyncWorker] 已经在运行中")
		return
	}
	w.running = true
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.mu.Unlock()

	log.Println("[PostSyncWorker] ========== 启动守护协程 ==========")

	w.recoverProcessingJobs()
	w.compensatePendingAssessments()
	w.processPendingJobs()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			log.Println("[PostSyncWorker] ========== 停止守护协程 ==========")
			w.mu.Lock()
			w.running = false
			w.mu.Unlock()
			return
		case <-ticker.C:
			w.processPendingJobs()
		case <-cleanupTicker.C:
			w.cleanupExpiredJobs()
		}
	}
}

func (w *PostSyncWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *PostSyncWorker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *PostSyncWorker) recoverProcessingJobs() {
	result := w.db.Model(&models.PostSyncJob{}).
		Where("status = ?", models.PostSyncJobStatusProcessing).
		Update("status", models.PostSyncJobStatusPending)
	if result.RowsAffected > 0 {
		log.Printf("[PostSyncWorker] 恢复 %d 个 processing 状态的任务", result.RowsAffected)
	}
}

// compensatePendingAssessments 启动补偿：为滞留在 pending/partial 的摘要评估资源创建 assessment job
func (w *PostSyncWorker) compensatePendingAssessments() {
	// 按 external_source_id 分组，找出有 pending/partial 评估资源但没有活跃 assessment job 的 source
	type sourceGroup struct {
		ExternalSourceID string
		Count            int64
	}
	var groups []sourceGroup
	w.db.Model(&models.ResourceIndex{}).
		Select("external_source_id, COUNT(*) as count").
		Where("summary_assessment_status IN ? AND external_source_id != ''", []string{
			string(models.AssessmentStatusPending),
			string(models.AssessmentStatusPartial),
		}).
		Group("external_source_id").
		Scan(&groups)

	for _, g := range groups {
		// 检查该 source 是否已有活跃的 assessment job
		var activeCount int64
		w.db.Model(&models.PostSyncJob{}).
			Where("source_id = ? AND job_type = ? AND status IN ?", g.ExternalSourceID,
				models.PostSyncJobTypeSummaryAssessment,
				[]string{models.PostSyncJobStatusPending, models.PostSyncJobStatusProcessing}).
			Count(&activeCount)

		if activeCount == 0 {
			job := models.PostSyncJob{
				SourceID:  g.ExternalSourceID,
				JobType:   models.PostSyncJobTypeSummaryAssessment,
				Status:    models.PostSyncJobStatusPending,
				CreatedAt: time.Now(),
			}
			w.db.Create(&job)
			log.Printf("[PostSyncWorker] 补偿：source %s 有 %d 条待评估/部分评估资源，创建 assessment job %d",
				g.ExternalSourceID, g.Count, job.ID)
		}
	}
}

func (w *PostSyncWorker) cleanupExpiredJobs() {
	expireTime := time.Now().AddDate(0, 0, -models.PostSyncJobExpireDays)

	result := w.db.Where("completed_at < ? AND status = ?", expireTime, models.PostSyncJobStatusCompleted).
		Delete(&models.PostSyncJob{})
	if result.RowsAffected > 0 {
		log.Printf("[PostSyncWorker] 清理 %d 个过期已完成任务", result.RowsAffected)
	}

	result = w.db.Where("created_at < ? AND status IN ?", expireTime, []string{
		models.PostSyncJobStatusPending,
		models.PostSyncJobStatusFailed,
	}).Delete(&models.PostSyncJob{})
	if result.RowsAffected > 0 {
		log.Printf("[PostSyncWorker] 清理 %d 个过期未完成任务", result.RowsAffected)
	}
}

// processPendingJobs 查找并执行就绪的 job
// 条件：pending + 未超重试 + 依赖已完成 + 同 source 无 processing（串行保证）
func (w *PostSyncWorker) processPendingJobs() {
	var jobs []models.PostSyncJob
	w.db.Raw(`
		SELECT j.* FROM cmdb_post_sync_jobs j
		WHERE j.status = ?
		  AND j.retry_count < ?
		  AND (j.depends_on IS NULL OR EXISTS (
		      SELECT 1 FROM cmdb_post_sync_jobs d
		      WHERE d.id = j.depends_on AND d.status = ?
		  ))
		  AND NOT EXISTS (
		      SELECT 1 FROM cmdb_post_sync_jobs p
		      WHERE p.source_id = j.source_id AND p.status = ?
		  )
		ORDER BY j.created_at ASC
		LIMIT 10
	`, models.PostSyncJobStatusPending, models.PostSyncJobMaxRetries,
		models.PostSyncJobStatusCompleted, models.PostSyncJobStatusProcessing).
		Scan(&jobs)

	for _, job := range jobs {
		select {
		case <-w.ctx.Done():
			return
		default:
		}
		w.processJob(job)
	}
}

func (w *PostSyncWorker) processJob(job models.PostSyncJob) {
	now := time.Now()
	w.db.Model(&job).Updates(map[string]interface{}{
		"status":     models.PostSyncJobStatusProcessing,
		"started_at": now,
	})

	log.Printf("[PostSyncWorker] 处理 job %d: source=%s, type=%s", job.ID, job.SourceID, job.JobType)

	var err error
	switch job.JobType {
	case models.PostSyncJobTypeSummary:
		err = w.executeSummaryJob(job)
	case models.PostSyncJobTypeEmbedding:
		err = w.executeEmbeddingJob(job)
	case models.PostSyncJobTypeSummaryAssessment:
		err = w.executeSummaryAssessmentJob(job)
	default:
		err = fmt.Errorf("unknown job type: %s", job.JobType)
	}

	completedAt := time.Now()
	if err != nil {
		log.Printf("[PostSyncWorker] job %d 失败: %v", job.ID, err)
		newRetryCount := job.RetryCount + 1
		if newRetryCount >= models.PostSyncJobMaxRetries {
			// 最终失败
			w.db.Model(&job).Updates(map[string]interface{}{
				"status":        models.PostSyncJobStatusFailed,
				"error_message": err.Error(),
				"retry_count":   newRetryCount,
			})
			// 级联失败：将依赖此 job 的 pending job 也标记为 failed
			cascadeResult := w.db.Model(&models.PostSyncJob{}).
				Where("depends_on = ? AND status = ?", job.ID, models.PostSyncJobStatusPending).
				Updates(map[string]interface{}{
					"status":        models.PostSyncJobStatusFailed,
					"error_message": fmt.Sprintf("dependency job %d failed", job.ID),
				})
			if cascadeResult.RowsAffected > 0 {
				log.Printf("[PostSyncWorker] 级联标记 %d 个依赖 job 为 failed", cascadeResult.RowsAffected)
			}
		} else {
			// 可重试，回退到 pending
			w.db.Model(&job).Updates(map[string]interface{}{
				"status":        models.PostSyncJobStatusPending,
				"error_message": err.Error(),
				"retry_count":   newRetryCount,
			})
		}
	} else {
		log.Printf("[PostSyncWorker] job %d 完成", job.ID)
		w.db.Model(&job).Updates(map[string]interface{}{
			"status":       models.PostSyncJobStatusCompleted,
			"completed_at": completedAt,
		})
		// 旁路：summary job 完成后 enqueue assessment job
		if job.JobType == models.PostSyncJobTypeSummary {
			w.enqueueSummaryAssessment(job)
		}
	}
}

// executeSummaryJob 执行摘要生成
func (w *PostSyncWorker) executeSummaryJob(job models.PostSyncJob) error {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Minute)
	defer cancel()

	summaryService := NewResourceSummaryService(w.db)
	if err := summaryService.GenerateSummariesForExternalSource(ctx, job.SourceID); err != nil {
		return fmt.Errorf("summary generation failed: %w", err)
	}
	return nil
}

// executeEmbeddingJob 执行 embedding 生成
// 优先使用 resource_summary，对无 summary 的资源 fallback 到 BuildEmbeddingText
func (w *PostSyncWorker) executeEmbeddingJob(job models.PostSyncJob) error {
	featureService := NewAIFeatureService(w.db)
	if !featureService.IsFeatureEnabled("embedding") {
		log.Printf("[PostSyncWorker] embedding 功能未启用，跳过")
		return nil
	}

	configStatus := w.embeddingService.GetConfigStatus()
	if !configStatus.Configured || !configStatus.HasAPIKey {
		log.Printf("[PostSyncWorker] embedding 配置不可用: %s", configStatus.Message)
		return nil
	}

	// 查找需要生成/刷新 embedding 的资源：
	//   1. 有 summary 且 embedding 缺失或过期（summary 变更）
	//   2. 无 summary 且 embedding 缺失或被标记重建（embedding_text 为空）
	var resources []models.ResourceIndex
	w.db.Where(`external_source_id = ? AND (
		(resource_summary IS NOT NULL AND resource_summary != '' AND (embedding IS NULL OR embedding_text != resource_summary))
		OR ((resource_summary IS NULL OR resource_summary = '') AND (embedding IS NULL OR embedding_text = ''))
	)`, job.SourceID).
		Find(&resources)

	if len(resources) == 0 {
		log.Printf("[PostSyncWorker] source %s 没有需要生成 embedding 的资源", job.SourceID)
		return nil
	}

	log.Printf("[PostSyncWorker] source %s 需要生成 embedding: %d 个资源", job.SourceID, len(resources))

	config, _ := w.embeddingService.configService.GetConfigForCapability("embedding")
	modelID := ""
	if config != nil {
		modelID = config.ModelID
	}

	successCount := 0
	for _, r := range resources {
		select {
		case <-w.ctx.Done():
			return fmt.Errorf("context cancelled, processed %d/%d", successCount, len(resources))
		default:
		}

		// 优先用 summary，无 summary 则 fallback 到 BuildEmbeddingText
		text := r.ResourceSummary
		if text == "" {
			text = w.embeddingService.BuildEmbeddingText(&r)
		}
		if text == "" {
			log.Printf("[PostSyncWorker] 资源 %d 无法构建 embedding 文本，跳过", r.ID)
			continue
		}

		embedding, err := w.embeddingService.GenerateEmbedding(text)
		if err != nil {
			log.Printf("[PostSyncWorker] 资源 %d 生成 embedding 失败: %v", r.ID, err)
			continue
		}

		now := time.Now()
		vectorStr := VectorToString(embedding)
		result := w.db.Exec(`
			UPDATE resource_index
			SET embedding = ?::vector,
			    embedding_text = ?,
			    embedding_model = ?,
			    embedding_updated_at = ?
			WHERE id = ?
		`, vectorStr, text, modelID, now, r.ID)

		if result.Error != nil {
			log.Printf("[PostSyncWorker] 资源 %d 更新 embedding 失败: %v", r.ID, result.Error)
		} else {
			successCount++
		}
	}

	log.Printf("[PostSyncWorker] source %s embedding 完成: %d/%d", job.SourceID, successCount, len(resources))

	if successCount == 0 && len(resources) > 0 {
		return fmt.Errorf("all %d embedding generations failed", len(resources))
	}
	return nil
}

// enqueueSummaryAssessment 旁路创建摘要评估 job（不阻塞 embedding 链）
func (w *PostSyncWorker) enqueueSummaryAssessment(summaryJob models.PostSyncJob) {
	assessmentJob := models.PostSyncJob{
		SourceID:  summaryJob.SourceID,
		JobType:   models.PostSyncJobTypeSummaryAssessment,
		Status:    models.PostSyncJobStatusPending,
		DependsOn: &summaryJob.ID,
		CreatedAt: time.Now(),
	}
	if err := w.db.Create(&assessmentJob).Error; err != nil {
		log.Printf("[PostSyncWorker] 创建 summary_assessment job 失败: %v", err)
		return
	}
	log.Printf("[PostSyncWorker] source %s 入队 summary_assessment job %d (依赖 summary job %d)",
		summaryJob.SourceID, assessmentJob.ID, summaryJob.ID)
}

// executeSummaryAssessmentJob 执行摘要质量评估
func (w *PostSyncWorker) executeSummaryAssessmentJob(job models.PostSyncJob) error {
	ctx, cancel := context.WithTimeout(w.ctx, 10*time.Minute)
	defer cancel()

	assessmentService := NewSummaryAssessmentService(w.db)
	if err := assessmentService.AssessSource(ctx, job.SourceID); err != nil {
		return fmt.Errorf("summary assessment failed: %w", err)
	}
	return nil
}
