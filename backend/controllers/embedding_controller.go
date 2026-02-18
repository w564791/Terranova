package controllers

import (
	"log"
	"net/http"
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

// VectorSearchRequest 向量搜索请求
type VectorSearchRequest struct {
	Query        string   `json:"query" binding:"required"`
	ResourceType string   `json:"resource_type,omitempty"`
	WorkspaceIDs []string `json:"workspace_ids,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

// VectorSearch 向量搜索（支持自动降级到关键字搜索）
// POST /api/cmdb/vector-search
func (c *EmbeddingController) VectorSearch(ctx *gin.Context) {
	var req VectorSearchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 设置默认值
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}

	// 检查配置状态
	configStatus := c.embeddingService.GetConfigStatus()

	// 如果 embedding 配置不可用，降级到关键字搜索
	if !configStatus.Configured || !configStatus.HasAPIKey {
		c.fallbackToKeywordSearch(ctx, req, "embedding 配置未就绪")
		return
	}

	// 生成查询向量
	queryVector, err := c.embeddingService.GenerateEmbedding(req.Query)
	if err != nil {
		// 生成向量失败，降级到关键字搜索
		c.fallbackToKeywordSearch(ctx, req, "生成查询向量失败: "+err.Error())
		return
	}

	// 构建 SQL 查询
	vectorStr := services.VectorToString(queryVector)

	// 从 embedding 配置中获取 topK 和 similarityThreshold
	embeddingConfig, _ := c.embeddingService.GetConfigService().GetConfigForCapability("embedding")
	topK := 50
	similarityThreshold := 0.3
	if embeddingConfig != nil {
		log.Printf("[VectorSearch] 读取 embedding 配置: ID=%d, TopK=%d, SimilarityThreshold=%.2f",
			embeddingConfig.ID, embeddingConfig.TopK, embeddingConfig.SimilarityThreshold)
		if embeddingConfig.TopK > 0 {
			topK = embeddingConfig.TopK
		}
		if embeddingConfig.SimilarityThreshold > 0 {
			similarityThreshold = embeddingConfig.SimilarityThreshold
		}
	} else {
		log.Printf("[VectorSearch] 未找到 embedding 配置，使用默认值: TopK=%d, SimilarityThreshold=%.2f", topK, similarityThreshold)
	}
	log.Printf("[VectorSearch] 最终使用: TopK=%d, SimilarityThreshold=%.2f", topK, similarityThreshold)

	// 如果请求中指定了 limit，使用请求中的值（但不超过配置的 topK）
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
			AND (ri.root_module_name LIKE '%' || wr.resource_name || '%' OR wr.resource_name LIKE '%' || ri.root_module_name || '%') 
			AND wr.is_active = true
		WHERE ri.embedding IS NOT NULL
		  AND ri.resource_mode = 'managed'
		  AND 1 - (ri.embedding <=> $1::vector) >= $2
	`

	args := []interface{}{vectorStr, similarityThreshold}
	argIndex := 3

	// 添加资源类型过滤
	if req.ResourceType != "" {
		sql += " AND ri.resource_type = $" + string(rune('0'+argIndex))
		args = append(args, req.ResourceType)
		argIndex++
	}

	// 添加 workspace 过滤
	if len(req.WorkspaceIDs) > 0 {
		sql += " AND ri.workspace_id = ANY($" + string(rune('0'+argIndex)) + ")"
		args = append(args, req.WorkspaceIDs)
		argIndex++
	}

	sql += " ORDER BY similarity DESC LIMIT $" + string(rune('0'+argIndex))
	args = append(args, topK)

	// 执行查询
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
		Similarity         float64 `json:"similarity"`
	}

	var results []SearchResult
	if err := c.db.Raw(sql, args...).Scan(&results).Error; err != nil {
		// 查询失败，降级到关键字搜索
		c.fallbackToKeywordSearch(ctx, req, "向量搜索查询失败: "+err.Error())
		return
	}

	// 如果向量搜索没有结果，降级到关键字搜索
	if len(results) == 0 {
		c.fallbackToKeywordSearch(ctx, req, "向量搜索无结果")
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"query":         req.Query,
			"results":       results,
			"count":         len(results),
			"search_method": "vector",
		},
	})
}

// fallbackToKeywordSearch 降级到关键字搜索
func (c *EmbeddingController) fallbackToKeywordSearch(ctx *gin.Context, req VectorSearchRequest, reason string) {
	cmdbService := services.NewCMDBService(c.db)

	workspaceID := ""
	if len(req.WorkspaceIDs) > 0 {
		workspaceID = req.WorkspaceIDs[0]
	}

	results, err := cmdbService.SearchResources(req.Query, workspaceID, req.ResourceType, req.Limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "搜索失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"query":           req.Query,
			"results":         results,
			"count":           len(results),
			"search_method":   "keyword",
			"fallback_reason": reason,
		},
	})
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
