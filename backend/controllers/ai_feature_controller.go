package controllers

import (
	"iac-platform/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AIFeatureController AI 能力开关控制器
type AIFeatureController struct {
	db      *gorm.DB
	service *services.AIFeatureService
}

// NewAIFeatureController 创建控制器
func NewAIFeatureController(db *gorm.DB) *AIFeatureController {
	return &AIFeatureController{
		db:      db,
		service: services.NewAIFeatureService(db),
	}
}

// GetFeatures 获取所有 AI 能力开关状态
func (c *AIFeatureController) GetFeatures(ctx *gin.Context) {
	features := c.service.GetAllFeatures()
	ctx.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": features,
	})
}

// UpdateFeatures 更新 AI 能力开关
func (c *AIFeatureController) UpdateFeatures(ctx *gin.Context) {
	var req map[string]bool
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if err := c.service.UpdateFeatures(req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "AI feature settings updated",
		"data":    c.service.GetAllFeatures(),
	})
}
