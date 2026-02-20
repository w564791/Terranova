package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EmbeddingWorker embedding 后台处理 worker（守护协程）
// 负责从队列中取出任务，批量生成 embedding
type EmbeddingWorker struct {
	db               *gorm.DB
	embeddingService *EmbeddingService
	batchSize        int           // 每批处理数量
	batchInterval    time.Duration // 批次间隔
	maxRetries       int           // 最大重试次数
	expireDays       int           // 任务过期天数
	running          bool
	mu               sync.Mutex
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewEmbeddingWorker 创建 embedding worker 实例
func NewEmbeddingWorker(db *gorm.DB) *EmbeddingWorker {
	return &EmbeddingWorker{
		db:               db,
		embeddingService: NewEmbeddingService(db),
		batchSize:        models.EmbeddingTaskBatchSize,
		batchInterval:    time.Duration(models.EmbeddingTaskBatchInterval) * time.Second,
		maxRetries:       models.EmbeddingTaskMaxRetries,
		expireDays:       models.EmbeddingTaskExpireDays,
	}
}

// Start 启动 Worker（守护协程）
// 应用启动时调用此方法，启动一个 goroutine 守护队列
func (w *EmbeddingWorker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		log.Println("[EmbeddingWorker] 已经在运行中")
		return
	}
	w.running = true
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.mu.Unlock()

	log.Println("[EmbeddingWorker] ========== 启动守护协程 ==========")

	// 1. 启动时清理过期任务
	w.cleanupExpiredTasks()

	// 2. 启动时恢复 processing 状态的任务（可能是上次异常退出）
	w.recoverProcessingTasks()

	// 3. 启动时立即处理一次待处理任务
	w.processPendingTasks()

	// 4. 启动定时器，每秒检查一次（提高响应速度）
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// 5. 启动每日清理定时器
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			log.Println("[EmbeddingWorker] ========== 停止守护协程 ==========")
			w.mu.Lock()
			w.running = false
			w.mu.Unlock()
			return
		case <-ticker.C:
			w.processPendingTasks()
		case <-cleanupTicker.C:
			w.cleanupExpiredTasks()
		}
	}
}

// Stop 停止 Worker
func (w *EmbeddingWorker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cancel != nil {
		w.cancel()
	}
}

// IsRunning 检查 Worker 是否在运行
func (w *EmbeddingWorker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// cleanupExpiredTasks 清理过期任务（超过 3 天）
func (w *EmbeddingWorker) cleanupExpiredTasks() {
	expireTime := time.Now().AddDate(0, 0, -w.expireDays)

	// 删除超过 3 天的 pending 和 processing 任务
	result := w.db.Where("created_at < ? AND status IN ?", expireTime, []string{
		models.EmbeddingTaskStatusPending,
		models.EmbeddingTaskStatusProcessing,
	}).Delete(&models.EmbeddingTask{})

	if result.RowsAffected > 0 {
		log.Printf("[EmbeddingWorker] 清理 %d 个过期任务（超过 %d 天）", result.RowsAffected, w.expireDays)
	}

	// 清理已完成超过 7 天的任务记录（保持表干净）
	completedExpireTime := time.Now().AddDate(0, 0, -7)
	w.db.Where("completed_at < ? AND status = ?", completedExpireTime, models.EmbeddingTaskStatusCompleted).
		Delete(&models.EmbeddingTask{})
}

// recoverProcessingTasks 恢复 processing 状态的任务
// 应用异常退出时，可能有任务处于 processing 状态，需要恢复为 pending
func (w *EmbeddingWorker) recoverProcessingTasks() {
	result := w.db.Model(&models.EmbeddingTask{}).
		Where("status = ?", models.EmbeddingTaskStatusProcessing).
		Updates(map[string]interface{}{
			"status":     models.EmbeddingTaskStatusPending,
			"updated_at": time.Now(),
		})

	if result.RowsAffected > 0 {
		log.Printf("[EmbeddingWorker] 恢复 %d 个 processing 状态的任务", result.RowsAffected)
	}
}

// processPendingTasks 处理待处理的任务
func (w *EmbeddingWorker) processPendingTasks() {
	// 检查 embedding 配置是否可用
	configStatus := w.embeddingService.GetConfigStatus()
	if !configStatus.Configured || !configStatus.HasAPIKey {
		// 配置不可用，跳过处理
		return
	}

	// 获取 AI Config 的 batch size，用于控制每次处理的任务数量
	// 这样可以实现实时进度更新
	config, _ := w.embeddingService.configService.GetConfigForCapability("embedding")
	batchSize := w.batchSize // 默认使用 Worker 的 batch size
	if config != nil && config.EmbeddingBatchEnabled && config.EmbeddingBatchSize > 0 {
		batchSize = config.EmbeddingBatchSize // 使用 AI Config 的 batch size
	}

	expireTime := time.Now().AddDate(0, 0, -w.expireDays)

	for {
		// 获取一批待处理的任务（排除过期任务）
		// 使用 AI Config 的 batch size，这样每批完成后进度就会更新
		var tasks []models.EmbeddingTask
		w.db.Where("status = ? AND retry_count < ? AND created_at > ?",
			models.EmbeddingTaskStatusPending, w.maxRetries, expireTime).
			Order("created_at ASC").
			Limit(batchSize).
			Find(&tasks)

		if len(tasks) == 0 {
			break // 没有更多任务
		}

		log.Printf("[EmbeddingWorker] 处理 %d 个任务", len(tasks))

		// 标记为处理中
		taskIDs := make([]uint, len(tasks))
		resourceIDs := make([]uint, len(tasks))
		for i, t := range tasks {
			taskIDs[i] = t.ID
			resourceIDs[i] = t.ResourceID
		}
		w.db.Model(&models.EmbeddingTask{}).
			Where("id IN ?", taskIDs).
			Update("status", models.EmbeddingTaskStatusProcessing)

		// 批量生成 embedding
		w.processBatch(resourceIDs, taskIDs)

		// 批次间隔，避免 API 限流
		time.Sleep(w.batchInterval)

		// 检查是否需要停止
		select {
		case <-w.ctx.Done():
			return
		default:
		}
	}
}

// processBatch 处理一批任务
// 支持批量 API（如果配置启用）或逐个处理
func (w *EmbeddingWorker) processBatch(resourceIDs []uint, taskIDs []uint) {
	log.Printf("[EmbeddingWorker] processBatch 开始: %d 个资源, %d 个任务", len(resourceIDs), len(taskIDs))

	// 获取资源信息
	var resources []models.ResourceIndex
	w.db.Where("id IN ?", resourceIDs).Find(&resources)

	log.Printf("[EmbeddingWorker] 从数据库获取到 %d 个资源", len(resources))

	if len(resources) == 0 {
		// 资源已被删除，直接删除任务
		w.db.Where("id IN ?", taskIDs).Delete(&models.EmbeddingTask{})
		log.Printf("[EmbeddingWorker] 资源已删除，清理 %d 个任务", len(taskIDs))
		return
	}

	// 构建资源ID到任务ID的映射
	resourceToTask := make(map[uint]uint)
	for i, rid := range resourceIDs {
		if i < len(taskIDs) {
			resourceToTask[rid] = taskIDs[i]
		}
	}

	// 获取当前使用的模型配置
	config, _ := w.embeddingService.configService.GetConfigForCapability("embedding")
	modelID := ""
	batchEnabled := false
	if config != nil {
		modelID = config.ModelID
		batchEnabled = config.EmbeddingBatchEnabled
		// 添加详细日志
		log.Printf("[EmbeddingWorker] 使用配置: ID=%d, ServiceType=%s, ModelID=%s, BatchEnabled=%v, BatchSize=%d",
			config.ID, config.ServiceType, config.ModelID, config.EmbeddingBatchEnabled, config.EmbeddingBatchSize)
	} else {
		log.Printf("[EmbeddingWorker] 警告: 未找到 embedding 配置")
	}

	// 如果启用了批量处理，尝试使用批量 API
	if batchEnabled && len(resources) > 1 {
		log.Printf("[EmbeddingWorker] 使用批量 API 处理 %d 个资源", len(resources))
		w.processBatchWithBatchAPI(resources, resourceToTask, modelID)
		return
	}

	// 逐个处理资源，每完成一个就更新数据库
	log.Printf("[EmbeddingWorker] 使用逐个处理模式 (batchEnabled=%v, resourceCount=%d)", batchEnabled, len(resources))
	w.processBatchSequentially(resources, resourceToTask, modelID)
}

// processBatchWithBatchAPI 使用批量 API 处理
func (w *EmbeddingWorker) processBatchWithBatchAPI(resources []models.ResourceIndex, resourceToTask map[uint]uint, modelID string) {
	// 构建所有文本
	texts := make([]string, len(resources))
	for i, r := range resources {
		if r.EmbeddingText != "" {
			texts[i] = r.EmbeddingText
		} else {
			texts[i] = w.embeddingService.BuildEmbeddingText(&r)
		}
	}

	// 批量生成 embedding
	embeddings, err := w.embeddingService.GenerateEmbeddingsBatch(texts)
	if err != nil {
		log.Printf("[EmbeddingWorker] 批量 embedding 失败，回退到逐个处理: %v", err)
		// 回退到逐个处理
		w.processBatchSequentially(resources, resourceToTask, modelID)
		return
	}

	// 批量更新数据库
	now := time.Now()
	successCount := 0
	for i, r := range resources {
		if i >= len(embeddings) || embeddings[i] == nil {
			log.Printf("[EmbeddingWorker] 资源 %d 的 embedding 为空", r.ID)
			continue
		}

		vectorStr := VectorToString(embeddings[i])
		result := w.db.Exec(`
			UPDATE resource_index 
			SET embedding = ?::vector, 
			    embedding_text = ?, 
			    embedding_model = ?, 
			    embedding_updated_at = ?
			WHERE id = ?
		`, vectorStr, texts[i], modelID, now, r.ID)

		if result.Error != nil {
			log.Printf("[EmbeddingWorker] 更新资源 %d 的 embedding 失败: %v", r.ID, result.Error)
		} else {
			successCount++
			// 标记任务完成
			if taskID, ok := resourceToTask[r.ID]; ok {
				w.db.Model(&models.EmbeddingTask{}).
					Where("id = ?", taskID).
					Updates(map[string]interface{}{
						"status":       models.EmbeddingTaskStatusCompleted,
						"completed_at": now,
						"updated_at":   now,
					})
			}
		}
	}

	log.Printf("[EmbeddingWorker] 批量处理完成 %d/%d 个资源的 embedding 生成", successCount, len(resources))
}

// processBatchSequentially 逐个处理资源
func (w *EmbeddingWorker) processBatchSequentially(resources []models.ResourceIndex, resourceToTask map[uint]uint, modelID string) {
	successCount := 0
	for _, r := range resources {
		// 构建 embedding 文本
		var text string
		if r.EmbeddingText != "" {
			text = r.EmbeddingText
		} else {
			text = w.embeddingService.BuildEmbeddingText(&r)
		}

		// 生成单个 embedding
		embedding, err := w.embeddingService.GenerateEmbedding(text)
		if err != nil {
			log.Printf("[EmbeddingWorker] 资源 %d 生成 embedding 失败: %v", r.ID, err)
			// 标记该任务失败
			if taskID, ok := resourceToTask[r.ID]; ok {
				w.db.Model(&models.EmbeddingTask{}).
					Where("id = ?", taskID).
					Updates(map[string]interface{}{
						"status":        models.EmbeddingTaskStatusPending,
						"retry_count":   gorm.Expr("retry_count + 1"),
						"error_message": err.Error(),
						"updated_at":    time.Now(),
					})
			}
			continue
		}

		// 立即更新资源的 embedding
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
			log.Printf("[EmbeddingWorker] 更新资源 %d 的 embedding 失败: %v", r.ID, result.Error)
		} else {
			successCount++
			// 立即标记该任务完成
			if taskID, ok := resourceToTask[r.ID]; ok {
				w.db.Model(&models.EmbeddingTask{}).
					Where("id = ?", taskID).
					Updates(map[string]interface{}{
						"status":       models.EmbeddingTaskStatusCompleted,
						"completed_at": now,
						"updated_at":   now,
					})
			}
			log.Printf("[EmbeddingWorker] 资源 %d embedding 生成成功 (%d/%d)", r.ID, successCount, len(resources))
		}
	}

	log.Printf("[EmbeddingWorker] 完成 %d/%d 个资源的 embedding 生成", successCount, len(resources))
}

// SyncAllWorkspaces 同步所有 Workspace 的 embedding（全量同步）
func (w *EmbeddingWorker) SyncAllWorkspaces() error {
	log.Println("[EmbeddingWorker] ========== 开始全量同步 ==========")

	// 1. 获取所有没有 embedding 的资源
	var resources []models.ResourceIndex
	w.db.Where("embedding IS NULL").Find(&resources)

	log.Printf("[EmbeddingWorker] 需要生成 embedding 的资源数: %d", len(resources))

	if len(resources) == 0 {
		log.Println("[EmbeddingWorker] 没有需要同步的资源")
		return nil
	}

	// 2. 批量创建 embedding 任务
	now := time.Now()
	tasks := make([]models.EmbeddingTask, 0, len(resources))
	for _, r := range resources {
		tasks = append(tasks, models.EmbeddingTask{
			ResourceID:  r.ID,
			WorkspaceID: r.WorkspaceID,
			Status:      models.EmbeddingTaskStatusPending,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	// 批量插入，忽略重复
	result := w.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&tasks, 1000)

	log.Printf("[EmbeddingWorker] 创建 %d 个 embedding 任务", result.RowsAffected)
	log.Println("[EmbeddingWorker] ========== 全量同步任务创建完成 ==========")

	return nil
}

// SyncWorkspace 同步指定 Workspace 的 embedding
func (w *EmbeddingWorker) SyncWorkspace(workspaceID string) error {
	log.Printf("[EmbeddingWorker] 同步 Workspace: %s", workspaceID)

	// 检查 embedding 配置是否可用
	configStatus := w.embeddingService.GetConfigStatus()
	log.Printf("[EmbeddingWorker] Embedding 配置状态: configured=%v, hasAPIKey=%v, message=%s",
		configStatus.Configured, configStatus.HasAPIKey, configStatus.Message)

	if !configStatus.Configured {
		log.Printf("[EmbeddingWorker] Embedding 配置不可用: %s", configStatus.Message)
		return nil
	}

	// 获取该 Workspace 没有 embedding 的资源
	var resources []models.ResourceIndex
	w.db.Where("workspace_id = ? AND embedding IS NULL", workspaceID).Find(&resources)

	log.Printf("[EmbeddingWorker] Workspace %s 找到 %d 个没有 embedding 的资源", workspaceID, len(resources))

	if len(resources) == 0 {
		log.Printf("[EmbeddingWorker] Workspace %s 没有需要同步的资源", workspaceID)
		return nil
	}

	// 批量创建 embedding 任务
	now := time.Now()
	tasks := make([]models.EmbeddingTask, 0, len(resources))
	for _, r := range resources {
		tasks = append(tasks, models.EmbeddingTask{
			ResourceID:  r.ID,
			WorkspaceID: r.WorkspaceID,
			Status:      models.EmbeddingTaskStatusPending,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	result := w.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&tasks, 1000)

	log.Printf("[EmbeddingWorker] Workspace %s 创建 %d 个 embedding 任务 (affected: %d)", workspaceID, len(tasks), result.RowsAffected)

	return nil
}

// RebuildWorkspace 重建指定 Workspace 的所有 embedding
func (w *EmbeddingWorker) RebuildWorkspace(workspaceID string) error {
	log.Printf("[EmbeddingWorker] 重建 Workspace: %s", workspaceID)

	// 1. 清空该 Workspace 的所有 embedding，同时更新 last_synced_at
	now := time.Now()
	result := w.db.Exec(`
		UPDATE resource_index 
		SET embedding = NULL, embedding_updated_at = NULL, last_synced_at = ?
		WHERE workspace_id = ?
	`, now, workspaceID)
	log.Printf("[EmbeddingWorker] 清空 %d 个资源的 embedding，更新 last_synced_at", result.RowsAffected)

	// 2. 删除该 Workspace 的所有任务（包括 completed 状态的，以便重新创建）
	result = w.db.Where("workspace_id = ?", workspaceID).Delete(&models.EmbeddingTask{})
	log.Printf("[EmbeddingWorker] 删除 %d 个旧任务", result.RowsAffected)

	// 3. 重新创建任务
	return w.SyncWorkspace(workspaceID)
}

// GetStatus 获取 worker 状态
func (w *EmbeddingWorker) GetStatus() *models.EmbeddingWorkerStatus {
	var pendingCount, processingCount, completedCount, failedCount, expiredCount int64

	expireTime := time.Now().AddDate(0, 0, -w.expireDays)

	w.db.Model(&models.EmbeddingTask{}).
		Where("status = ? AND created_at > ?", models.EmbeddingTaskStatusPending, expireTime).
		Count(&pendingCount)
	w.db.Model(&models.EmbeddingTask{}).
		Where("status = ?", models.EmbeddingTaskStatusProcessing).
		Count(&processingCount)
	w.db.Model(&models.EmbeddingTask{}).
		Where("status = ?", models.EmbeddingTaskStatusCompleted).
		Count(&completedCount)
	w.db.Model(&models.EmbeddingTask{}).
		Where("status = ? AND retry_count >= ?", models.EmbeddingTaskStatusPending, w.maxRetries).
		Count(&failedCount)
	w.db.Model(&models.EmbeddingTask{}).
		Where("created_at < ? AND status IN ?", expireTime, []string{
			models.EmbeddingTaskStatusPending,
			models.EmbeddingTaskStatusProcessing,
		}).
		Count(&expiredCount)

	return &models.EmbeddingWorkerStatus{
		Running:         w.running,
		PendingTasks:    pendingCount,
		ProcessingTasks: processingCount,
		CompletedTasks:  completedCount,
		FailedTasks:     failedCount,
		ExpiredTasks:    expiredCount,
		ExpireDays:      w.expireDays,
	}
}

// GetWorkspaceStatus 获取指定 Workspace 的 embedding 状态
func (w *EmbeddingWorker) GetWorkspaceStatus(workspaceID string) *models.EmbeddingStatus {
	var totalResources, withEmbedding, pendingTasks, processingTasks, failedTasks int64

	// 统计资源总数
	w.db.Model(&models.ResourceIndex{}).
		Where("workspace_id = ?", workspaceID).
		Count(&totalResources)

	// 统计有 embedding 的资源数
	w.db.Model(&models.ResourceIndex{}).
		Where("workspace_id = ? AND embedding IS NOT NULL", workspaceID).
		Count(&withEmbedding)

	// 统计任务状态
	w.db.Model(&models.EmbeddingTask{}).
		Where("workspace_id = ? AND status = ?", workspaceID, models.EmbeddingTaskStatusPending).
		Count(&pendingTasks)
	w.db.Model(&models.EmbeddingTask{}).
		Where("workspace_id = ? AND status = ?", workspaceID, models.EmbeddingTaskStatusProcessing).
		Count(&processingTasks)
	w.db.Model(&models.EmbeddingTask{}).
		Where("workspace_id = ? AND status = ? AND retry_count >= ?",
			workspaceID, models.EmbeddingTaskStatusPending, w.maxRetries).
		Count(&failedTasks)

	// 计算进度
	// 进度 = 已完成的 embedding 数量 / 总资源数量
	// 这样可以实时反映 embedding 生成的进度
	var progress float64
	if totalResources > 0 {
		progress = float64(withEmbedding) / float64(totalResources) * 100
	}

	// 预估剩余时间
	// 对于 Bedrock，每个资源约 5-6 秒（逐个调用）
	// 对于 OpenAI，每批约 2 秒
	remainingTasks := pendingTasks + processingTasks
	var estimatedSeconds int64
	if remainingTasks > 0 {
		// 假设每个资源 5 秒（Bedrock 逐个调用）
		estimatedSeconds = remainingTasks * 5
	}
	estimatedTime := formatDuration(estimatedSeconds)

	return &models.EmbeddingStatus{
		WorkspaceID:     workspaceID,
		TotalResources:  totalResources,
		WithEmbedding:   withEmbedding,
		PendingTasks:    pendingTasks,
		ProcessingTasks: processingTasks,
		FailedTasks:     failedTasks,
		Progress:        progress,
		EstimatedTime:   estimatedTime,
	}
}

// formatDuration 格式化时间
func formatDuration(seconds int64) string {
	if seconds < 60 {
		return "不到 1 分钟"
	} else if seconds < 3600 {
		minutes := seconds / 60
		return fmt.Sprintf("约 %d 分钟", minutes)
	} else {
		hours := seconds / 3600
		minutes := (seconds % 3600) / 60
		if minutes > 0 {
			return fmt.Sprintf("约 %d 小时 %d 分钟", hours, minutes)
		}
		return fmt.Sprintf("约 %d 小时", hours)
	}
}

// CreateEmbeddingTask 创建单个 embedding 任务
func (w *EmbeddingWorker) CreateEmbeddingTask(resourceID uint, workspaceID string) error {
	task := models.EmbeddingTask{
		ResourceID:  resourceID,
		WorkspaceID: workspaceID,
		Status:      models.EmbeddingTaskStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	return w.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&task).Error
}

// CreateEmbeddingTasks 批量创建 embedding 任务
func (w *EmbeddingWorker) CreateEmbeddingTasks(workspaceID string, resourceIDs []uint) error {
	if len(resourceIDs) == 0 {
		return nil
	}

	now := time.Now()
	tasks := make([]models.EmbeddingTask, 0, len(resourceIDs))
	for _, id := range resourceIDs {
		tasks = append(tasks, models.EmbeddingTask{
			ResourceID:  id,
			WorkspaceID: workspaceID,
			Status:      models.EmbeddingTaskStatusPending,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	return w.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&tasks, 1000).Error
}
