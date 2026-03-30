package controllers

import (
	"iac-platform/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// EmbeddingCacheController 向量缓存控制器
type EmbeddingCacheController struct {
	db           *gorm.DB
	cacheService *services.EmbeddingCacheService
}

// NewEmbeddingCacheController 创建控制器实例
func NewEmbeddingCacheController(db *gorm.DB) *EmbeddingCacheController {
	embeddingService := services.NewEmbeddingService(db)
	return &EmbeddingCacheController{
		db:           db,
		cacheService: services.NewEmbeddingCacheService(db, embeddingService),
	}
}

// WarmUp 预热缓存
// @Summary Warm up embedding cache
// @Description Pre-generate vector embeddings for common keywords to improve search performance
// @Tags Embedding Cache
// @Accept json
// @Produce json
// @Param force query bool false "Force regenerate all vectors" default(false)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/embedding-cache/warmup [post]
func (c *EmbeddingCacheController) WarmUp(ctx *gin.Context) {
	// 检查是否已在运行
	if c.cacheService.IsWarmupRunning() {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "预热任务已在运行中",
		})
		return
	}

	// 获取 force 参数
	force := ctx.Query("force") == "true"

	// 异步执行预热
	go func() {
		if err := c.cacheService.WarmUpWithForce(force); err != nil {
			// 记录错误日志
			println("[EmbeddingCache] 预热失败:", err.Error())
		}
	}()

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "预热任务已启动，将在后台执行",
		"force":   force,
	})
}

// GetWarmupProgress 获取预热进度
// @Summary Get warmup progress
// @Description Get the current warmup task progress
// @Tags Embedding Cache
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/embedding-cache/warmup/progress [get]
func (c *EmbeddingCacheController) GetWarmupProgress(ctx *gin.Context) {
	progress := c.cacheService.GetWarmupProgress()

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    progress,
	})
}

// GetStats 获取缓存统计
// @Summary Get embedding cache statistics
// @Description Get statistics for the embedding cache including count and hit rate
// @Tags Embedding Cache
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/embedding-cache/stats [get]
func (c *EmbeddingCacheController) GetStats(ctx *gin.Context) {
	stats, err := c.cacheService.GetStats()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// ClearCache 清空缓存
// @Summary Clear embedding cache
// @Description Clear all embedding cache entries (use with caution)
// @Tags Embedding Cache
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/embedding-cache/clear [delete]
func (c *EmbeddingCacheController) ClearCache(ctx *gin.Context) {
	if err := c.cacheService.ClearCache(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "缓存已清空",
	})
}

// CleanupLowHit 清理低命中缓存
// @Summary Clean up low-hit cache entries
// @Description Remove cache entries with hit count below threshold and older than specified days
// @Tags Embedding Cache
// @Produce json
// @Param min_hit_count query int false "Minimum hit count threshold" default(5)
// @Param older_than_days query int false "Older than days" default(30)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /api/v1/admin/embedding-cache/cleanup [post]
func (c *EmbeddingCacheController) CleanupLowHit(ctx *gin.Context) {
	minHitCount := 5
	olderThanDays := 30

	if v := ctx.Query("min_hit_count"); v != "" {
		var val int
		if _, err := ctx.GetQuery("min_hit_count"); err {
			minHitCount = val
		}
	}

	if v := ctx.Query("older_than_days"); v != "" {
		var val int
		if _, err := ctx.GetQuery("older_than_days"); err {
			olderThanDays = val
		}
	}

	deleted, err := c.cacheService.CleanupLowHitCache(minHitCount, olderThanDays)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "清理完成",
		"deleted": deleted,
	})
}
