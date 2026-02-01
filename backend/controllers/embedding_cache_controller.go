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
// @Summary 预热向量缓存
// @Description 预热常用关键词的向量缓存，提升搜索性能
// @Tags AI-Cache
// @Accept json
// @Produce json
// @Param force query bool false "是否强制重新生成所有向量" default(false)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/ai/embedding-cache/warmup [post]
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
// @Summary 获取预热进度
// @Description 获取当前预热任务的进度信息
// @Tags AI-Cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/ai/embedding-cache/warmup/progress [get]
func (c *EmbeddingCacheController) GetWarmupProgress(ctx *gin.Context) {
	progress := c.cacheService.GetWarmupProgress()

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    progress,
	})
}

// GetStats 获取缓存统计
// @Summary 获取向量缓存统计
// @Description 获取向量缓存的统计信息，包括缓存数量、命中率等
// @Tags AI-Cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/ai/embedding-cache/stats [get]
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
// @Summary 清空向量缓存
// @Description 清空所有向量缓存（谨慎使用）
// @Tags AI-Cache
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/ai/embedding-cache/clear [delete]
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
// @Summary 清理低命中率缓存
// @Description 清理命中次数低于阈值的缓存条目
// @Tags AI-Cache
// @Accept json
// @Produce json
// @Param min_hit_count query int false "最小命中次数" default(5)
// @Param older_than_days query int false "创建时间超过多少天" default(30)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/ai/embedding-cache/cleanup [post]
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
