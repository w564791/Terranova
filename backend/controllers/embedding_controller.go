package controllers

import (
	"log"
	"net/http"

	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EmbeddingController embedding 控制器
type EmbeddingController struct {
	db               *gorm.DB
	worker           *services.EmbeddingWorker
	embeddingService *services.EmbeddingService
	aiFormService    *services.AIFormService // 用于意图断言
}

// NewEmbeddingController 创建 embedding 控制器
func NewEmbeddingController(db *gorm.DB, worker *services.EmbeddingWorker) *EmbeddingController {
	return &EmbeddingController{
		db:               db,
		worker:           worker,
		embeddingService: services.NewEmbeddingService(db),
		aiFormService:    services.NewAIFormService(db),
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

	err := c.worker.SyncWorkspace(workspaceID)
	if err != nil {
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

	err := c.worker.RebuildWorkspace(workspaceID)
	if err != nil {
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

	// 获取用户 ID（用于意图断言）
	userID, _ := ctx.Get("user_id")
	userIDStr, _ := userID.(string)

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

	// ========== 意图断言检查（安全守卫）==========
	// 防止恶意用户通过向量搜索接口发送不当内容到 Embedding API
	assertionResult, err := c.aiFormService.AssertIntent(userIDStr, req.Query)
	if err != nil {
		// 意图断言服务不可用，记录警告但继续执行（降级处理）
	} else if assertionResult != nil && !assertionResult.IsSafe {
		// 意图不安全，拦截请求
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": assertionResult.Suggestion,
			"blocked": true,
		})
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
