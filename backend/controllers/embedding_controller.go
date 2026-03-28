package controllers

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EmbeddingController embedding 控制器
type EmbeddingController struct {
	db               *gorm.DB
	worker           *services.EmbeddingWorker
	embeddingService *services.EmbeddingService
}

// NewEmbeddingController 创建 embedding 控制器
func NewEmbeddingController(db *gorm.DB, worker *services.EmbeddingWorker) *EmbeddingController {
	return &EmbeddingController{
		db:               db,
		worker:           worker,
		embeddingService: services.NewEmbeddingService(db),
	}
}

// GetConfigStatus 获取 embedding 配置状态
// GET /api/ai/embedding/config-status
func (c *EmbeddingController) GetConfigStatus(ctx *gin.Context) {
	status := c.embeddingService.GetConfigStatus()

	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": status,
	})
}

// GetWorkerStatus 获取 worker 状态
// GET /api/admin/embedding/status
func (c *EmbeddingController) GetWorkerStatus(ctx *gin.Context) {
	status := c.worker.GetStatus()

	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": status,
	})
}

// GetWorkspaceEmbeddingStatus 获取 Workspace 的 embedding 状态
// GET /api/workspaces/:id/embedding-status
func (c *EmbeddingController) GetWorkspaceEmbeddingStatus(ctx *gin.Context) {
	workspaceID := ctx.Param("id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "workspace_id 不能为空",
		})
		return
	}

	status := c.worker.GetWorkspaceStatus(workspaceID)

	// 填充 CMDB 同步状态
	var workspace models.Workspace
	if err := c.db.Select("cmdb_sync_status", "cmdb_sync_triggered_by", "cmdb_sync_started_at", "cmdb_sync_completed_at").
		Where("workspace_id = ?", workspaceID).First(&workspace).Error; err == nil {

		status.CMDBSyncStatus = workspace.CMDBSyncStatus
		status.CMDBSyncTriggeredBy = workspace.CMDBSyncTriggeredBy
		if workspace.CMDBSyncStartedAt != nil {
			t := workspace.CMDBSyncStartedAt.Format(time.RFC3339)
			status.CMDBSyncStartedAt = &t
		}
		if workspace.CMDBSyncCompletedAt != nil {
			t := workspace.CMDBSyncCompletedAt.Format(time.RFC3339)
			status.CMDBSyncCompletedAt = &t
		}

		// 自动转换：如果状态是 syncing 但已无活跃任务，则转为 idle
		if workspace.CMDBSyncStatus == models.CMDBSyncStatusSyncing &&
			status.PendingTasks == 0 && status.ProcessingTasks == 0 {
			now := time.Now()
			c.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(map[string]interface{}{
				"cmdb_sync_status":       models.CMDBSyncStatusIdle,
				"cmdb_sync_completed_at": now,
			})
			status.CMDBSyncStatus = models.CMDBSyncStatusIdle
			t := now.Format(time.RFC3339)
			status.CMDBSyncCompletedAt = &t
			log.Printf("[Embedding] Auto-transitioned workspace %s CMDB sync status to idle (no active tasks)", workspaceID)
		}
	}

	// 综合判断是否繁忙
	status.IsBusy = status.CMDBSyncStatus == models.CMDBSyncStatusSyncing ||
		status.PendingTasks > 0 || status.ProcessingTasks > 0

	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": status,
	})
}

// SyncAllWorkspaces 同步所有 Workspace 的 embedding
// POST /api/admin/embedding/sync-all
func (c *EmbeddingController) SyncAllWorkspaces(ctx *gin.Context) {
	// 检查配置状态
	configStatus := c.embeddingService.GetConfigStatus()
	if !configStatus.Configured {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": configStatus.Message,
			"help":    configStatus.Help,
		})
		return
	}

	if !configStatus.HasAPIKey {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "API Key 未配置，请在 AI 配置管理界面填写 OpenAI API Key",
		})
		return
	}

	err := c.worker.SyncAllWorkspaces()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "全量同步任务已创建，后台处理中",
		"data":    c.worker.GetStatus(),
	})
}

// SyncWorkspace 同步指定 Workspace 的 embedding
// POST /api/workspaces/:id/embedding/sync
func (c *EmbeddingController) SyncWorkspace(ctx *gin.Context) {
	workspaceID := ctx.Param("id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "workspace_id 不能为空",
		})
		return
	}

	// 检查配置状态
	configStatus := c.embeddingService.GetConfigStatus()
	if !configStatus.Configured || !configStatus.HasAPIKey {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "embedding 配置不可用",
			"help":    configStatus.Help,
		})
		return
	}

	// 互斥检查：是否有同步任务在运行
	if busy, reason := c.isWorkspaceBusy(workspaceID); busy {
		ctx.JSON(http.StatusConflict, gin.H{
			"code":    409,
			"message": reason,
		})
		return
	}

	// 标记同步状态
	now := time.Now()
	c.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(map[string]interface{}{
		"cmdb_sync_status":       models.CMDBSyncStatusSyncing,
		"cmdb_sync_triggered_by": models.CMDBSyncTriggerManual,
		"cmdb_sync_started_at":   now,
		"cmdb_sync_completed_at": nil,
	})

	err := c.worker.SyncWorkspace(workspaceID)
	if err != nil {
		// 同步失败，重置状态
		completedAt := time.Now()
		c.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(map[string]interface{}{
			"cmdb_sync_status":       models.CMDBSyncStatusIdle,
			"cmdb_sync_completed_at": completedAt,
		})
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "同步任务已创建，后台处理中",
		"data":    c.worker.GetWorkspaceStatus(workspaceID),
	})
}

// RebuildWorkspace 重建指定 Workspace 的 embedding
// POST /api/workspaces/:id/embedding/rebuild
func (c *EmbeddingController) RebuildWorkspace(ctx *gin.Context) {
	workspaceID := ctx.Param("id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "workspace_id 不能为空",
		})
		return
	}

	// 检查配置状态
	configStatus := c.embeddingService.GetConfigStatus()
	if !configStatus.Configured || !configStatus.HasAPIKey {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "embedding 配置不可用",
			"help":    configStatus.Help,
		})
		return
	}

	// 互斥检查：是否有同步任务在运行
	if busy, reason := c.isWorkspaceBusy(workspaceID); busy {
		ctx.JSON(http.StatusConflict, gin.H{
			"code":    409,
			"message": reason,
		})
		return
	}

	// 标记同步状态
	now := time.Now()
	c.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(map[string]interface{}{
		"cmdb_sync_status":       models.CMDBSyncStatusSyncing,
		"cmdb_sync_triggered_by": models.CMDBSyncTriggerRebuild,
		"cmdb_sync_started_at":   now,
		"cmdb_sync_completed_at": nil,
	})

	// 外部 CMDB 资源走 PostSyncWorker 路径
	if workspaceID == "__external__" {
		c.rebuildExternalEmbedding(ctx)
		return
	}

	err := c.worker.RebuildWorkspace(workspaceID)
	if err != nil {
		// 重建失败，重置状态
		completedAt := time.Now()
		c.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(map[string]interface{}{
			"cmdb_sync_status":       models.CMDBSyncStatusIdle,
			"cmdb_sync_completed_at": completedAt,
		})
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "重建任务已创建，后台处理中",
		"data":    c.worker.GetWorkspaceStatus(workspaceID),
	})
}

// rebuildExternalEmbedding 重建外部 CMDB 资源的 embedding
// 不清空现有 embedding，而是清空 embedding_text 触发 PostSyncWorker 重新生成
// 旧 embedding 在被覆盖前仍可搜索
func (c *EmbeddingController) rebuildExternalEmbedding(ctx *gin.Context) {
	// 清空 embedding_text 让 PostSyncWorker 检测到变更并覆盖（保留 embedding 本身）
	result := c.db.Exec(`
		UPDATE resource_index
		SET embedding_text = ''
		WHERE workspace_id = '__external__'
	`)
	log.Printf("[EmbeddingController] 标记 %d 个外部资源待重建 embedding", result.RowsAffected)

	// 为每个外部 source 入队 embedding job（不需要 summary，只重建 embedding）
	var sourceIDs []string
	c.db.Model(&models.ResourceIndex{}).
		Where("workspace_id = '__external__' AND external_source_id != ''").
		Distinct("external_source_id").
		Pluck("external_source_id", &sourceIDs)

	now := time.Now()
	jobCount := 0
	for _, sourceID := range sourceIDs {
		job := models.PostSyncJob{
			SourceID: sourceID, JobType: models.PostSyncJobTypeEmbedding,
			Status: models.PostSyncJobStatusPending, CreatedAt: now,
		}
		if err := c.db.Create(&job).Error; err == nil {
			jobCount++
		}
	}

	log.Printf("[EmbeddingController] 为 %d 个外部 source 创建 embedding rebuild job", jobCount)

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": fmt.Sprintf("重建任务已创建，%d 个数据源将后台处理", jobCount),
	})
}

// VectorSearchRequest 向量搜索请求
type VectorSearchRequest struct {
	Query        string   `json:"query" binding:"required"`
	ResourceType string   `json:"resource_type,omitempty"`
	WorkspaceIDs []string `json:"workspace_ids,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

// SearchResult 搜索结果（向量搜索和关键词搜索的统一结构）
type SearchResult struct {
	ID                 uint    `json:"id"`
	WorkspaceID        string  `json:"workspace_id"`
	WorkspaceName      string  `json:"workspace_name"`
	TerraformAddress   string  `json:"terraform_address"`
	ResourceType       string  `json:"resource_type"`
	ResourceName       string  `json:"resource_name"`
	CloudResourceID    string  `json:"cloud_resource_id"`
	CloudResourceName  string  `json:"cloud_resource_name"`
	CloudResourceARN   string  `json:"cloud_resource_arn"`
	Description        string  `json:"description"`
	ModulePath         string  `json:"module_path"`
	RootModuleName     string  `json:"root_module_name"`
	SourceType         string  `json:"source_type"`
	ExternalSourceID   string  `json:"external_source_id"`
	ExternalSourceName string  `json:"external_source_name"`
	CloudProvider      string  `json:"cloud_provider"`
	CloudAccountID     string  `json:"cloud_account_id"`
	CloudAccountName   string  `json:"cloud_account_name"`
	CloudRegion        string  `json:"cloud_region"`
	PlatformResourceID *uint   `json:"platform_resource_id"`
	JumpURL            string  `json:"jump_url"`
	ResourceSummary    string  `json:"resource_summary,omitempty"`
	Similarity         float64 `json:"similarity"`
}

// VectorSearch 混合搜索（向量搜索 + 关键词搜索并行，合并去重）
// POST /api/cmdb/vector-search
func (c *EmbeddingController) VectorSearch(ctx *gin.Context) {
	var req VectorSearchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}

	var vectorResults, keywordResults []SearchResult
	var vectorErr, keywordErr error
	searchMethod := "hybrid"

	var wg sync.WaitGroup

	// 并行：向量搜索（仅当配置可用时）
	configStatus := c.embeddingService.GetConfigStatus()
	if configStatus.Configured && configStatus.HasAPIKey {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vectorResults, vectorErr = c.doVectorSearch(req)
		}()
	}

	// 并行：关键词搜索
	wg.Add(1)
	go func() {
		defer wg.Done()
		keywordResults, keywordErr = c.doKeywordSearch(req)
	}()

	wg.Wait()

	// 合并去重：向量结果优先（有 similarity score）
	// 用 TerraformAddress 做去重 key（关键词搜索结果无 ID 字段）
	seen := make(map[string]bool)
	var merged []SearchResult

	if vectorErr == nil {
		for _, r := range vectorResults {
			seen[r.TerraformAddress] = true
			merged = append(merged, r)
		}
	}
	if keywordErr == nil {
		for _, r := range keywordResults {
			if !seen[r.TerraformAddress] {
				seen[r.TerraformAddress] = true
				merged = append(merged, r)
			}
		}
	}

	if vectorErr != nil && keywordErr != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "搜索失败"})
		return
	}

	if vectorErr != nil || len(vectorResults) == 0 {
		searchMethod = "keyword"
	} else if keywordErr != nil || len(keywordResults) == 0 {
		searchMethod = "vector"
	}

	if len(merged) > req.Limit {
		merged = merged[:req.Limit]
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"query":         req.Query,
			"results":       merged,
			"count":         len(merged),
			"search_method": searchMethod,
		},
	})
}

// doVectorSearch 执行向量搜索
func (c *EmbeddingController) doVectorSearch(req VectorSearchRequest) ([]SearchResult, error) {
	queryVector, err := c.embeddingService.GenerateEmbedding(req.Query)
	if err != nil {
		return nil, fmt.Errorf("生成查询向量失败: %w", err)
	}

	vectorStr := services.VectorToString(queryVector)

	embeddingConfig, _ := c.embeddingService.GetConfigService().GetConfigForCapability("embedding")
	topK := 50
	similarityThreshold := 0.3
	if embeddingConfig != nil {
		if embeddingConfig.TopK > 0 {
			topK = embeddingConfig.TopK
		}
		if embeddingConfig.SimilarityThreshold > 0 {
			similarityThreshold = embeddingConfig.SimilarityThreshold
		}
	}
	if req.Limit > 0 && req.Limit < topK {
		topK = req.Limit
	}

	sql := `
		SELECT
			ri.id,
			ri.workspace_id,
			w.name as workspace_name,
			ri.terraform_address,
			ri.resource_type,
			ri.resource_name,
			ri.cloud_resource_id,
			ri.cloud_resource_name,
			ri.cloud_resource_arn,
			ri.description,
			ri.module_path,
			ri.root_module_name,
			ri.source_type,
			ri.external_source_id,
			es.name as external_source_name,
			ri.cloud_provider,
			ri.cloud_account_id,
			ri.cloud_account_name,
			ri.cloud_region,
			ri.resource_summary,
			wr.id as platform_resource_id,
			CASE
				WHEN ri.source_type = 'external' THEN NULL
				WHEN wr.id IS NOT NULL THEN CONCAT('/workspaces/', ri.workspace_id, '/resources/', wr.id)
				ELSE NULL
			END as jump_url,
			1 - (ri.embedding <=> $1::vector) as similarity
		FROM resource_index ri
		LEFT JOIN workspaces w ON ri.workspace_id = w.workspace_id
		LEFT JOIN cmdb_external_sources es ON ri.external_source_id = es.source_id
		LEFT JOIN workspace_resources wr ON ri.workspace_id = wr.workspace_id
			AND ri.source_type = 'terraform'
			AND ri.root_module_name LIKE '%\_' || wr.resource_name
			AND wr.is_active = true
		WHERE ri.embedding IS NOT NULL
		  AND ri.resource_mode = 'managed'
		  AND 1 - (ri.embedding <=> $1::vector) >= $2
	`

	args := []interface{}{vectorStr, similarityThreshold}
	argIndex := 3

	if req.ResourceType != "" {
		sql += fmt.Sprintf(" AND ri.resource_type = $%d", argIndex)
		args = append(args, req.ResourceType)
		argIndex++
	}

	if len(req.WorkspaceIDs) > 0 {
		sql += fmt.Sprintf(" AND ri.workspace_id = ANY($%d)", argIndex)
		args = append(args, req.WorkspaceIDs)
		argIndex++
	}

	sql += fmt.Sprintf(" ORDER BY similarity DESC LIMIT $%d", argIndex)
	args = append(args, topK)

	var rawResults []SearchResult
	if err := c.db.Raw(sql, args...).Scan(&rawResults).Error; err != nil {
		return nil, fmt.Errorf("向量搜索失败: %w", err)
	}

	// 按 ri.id 去重（LEFT JOIN workspace_resources 可能产生多行）
	seen := make(map[uint]bool)
	results := make([]SearchResult, 0, len(rawResults))
	for _, r := range rawResults {
		if !seen[r.ID] {
			seen[r.ID] = true
			results = append(results, r)
		}
	}
	return results, nil
}

// doKeywordSearch 执行关键词搜索
func (c *EmbeddingController) doKeywordSearch(req VectorSearchRequest) ([]SearchResult, error) {
	cmdbService := services.NewCMDBService(c.db)

	workspaceID := ""
	if len(req.WorkspaceIDs) > 0 {
		workspaceID = req.WorkspaceIDs[0]
	}

	resources, err := cmdbService.SearchResources(req.Query, workspaceID, req.ResourceType, req.Limit)
	if err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(resources))
	for _, r := range resources {
		results = append(results, SearchResult{
			WorkspaceID:        r.WorkspaceID,
			WorkspaceName:      r.WorkspaceName,
			TerraformAddress:   r.TerraformAddress,
			ResourceType:       r.ResourceType,
			ResourceName:       r.ResourceName,
			CloudResourceID:    r.CloudResourceID,
			CloudResourceName:  r.CloudResourceName,
			CloudResourceARN:   r.CloudResourceARN,
			Description:        r.Description,
			ModulePath:         r.ModulePath,
			RootModuleName:     r.RootModuleName,
			SourceType:         r.SourceType,
			ExternalSourceID:   r.ExternalSourceID,
			ExternalSourceName: r.ExternalSourceName,
			CloudProvider:      r.CloudProvider,
			CloudAccountID:     r.CloudAccountID,
			CloudAccountName:   r.CloudAccountName,
			CloudRegion:        r.CloudRegion,
			PlatformResourceID: r.PlatformResourceID,
			JumpURL:            r.JumpURL,
			ResourceSummary:    r.ResourceSummary,
			Similarity:         0,
		})
	}
	return results, nil
}

// isWorkspaceBusy 检查 workspace 是否有同步任务在运行
// 返回 (busy, reason)
func (c *EmbeddingController) isWorkspaceBusy(workspaceID string) (bool, string) {
	// 1. 检查 CMDB 同步状态
	var workspace models.Workspace
	if err := c.db.Select("cmdb_sync_status", "cmdb_sync_triggered_by").
		Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		log.Printf("[Embedding] Failed to check workspace %s sync status: %v", workspaceID, err)
		return false, ""
	}

	if workspace.CMDBSyncStatus == models.CMDBSyncStatusSyncing {
		triggerDesc := map[string]string{
			models.CMDBSyncTriggerAuto:    "apply 后自动同步",
			models.CMDBSyncTriggerManual:  "手动同步",
			models.CMDBSyncTriggerRebuild: "重建",
		}
		desc := triggerDesc[workspace.CMDBSyncTriggeredBy]
		if desc == "" {
			desc = "同步"
		}
		return true, "当前有" + desc + "任务正在运行，请等待完成后再操作"
	}

	// 2. 检查是否有正在处理的 embedding 任务
	var activeTasks int64
	c.db.Model(&models.EmbeddingTask{}).
		Where("workspace_id = ? AND status IN ?", workspaceID, []string{
			models.EmbeddingTaskStatusPending,
			models.EmbeddingTaskStatusProcessing,
		}).Count(&activeTasks)

	if activeTasks > 0 {
		return true, "当前有 embedding 任务正在处理中，请等待完成后再操作"
	}

	return false, ""
}
