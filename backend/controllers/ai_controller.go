package controllers

import (
	"fmt"
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AIController AI 控制器
type AIController struct {
	configService   *services.AIConfigService
	analysisService *services.AIAnalysisService
}

// NewAIController 创建 AI 控制器实例
func NewAIController(db *gorm.DB) *AIController {
	return &AIController{
		configService:   services.NewAIConfigService(db),
		analysisService: services.NewAIAnalysisService(db),
	}
}

// ListConfigs 获取 AI 配置列表
// @Summary 获取 AI 配置列表
// @Description 获取所有AI配置
// @Tags AI
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "成功返回配置列表"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/admin/ai-configs [get]
// @Security Bearer
func (c *AIController) ListConfigs(ctx *gin.Context) {
	configs, err := c.configService.ListConfigs()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取配置列表失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data":    configs,
	})
}

// GetConfig 获取单个 AI 配置
// @Summary 获取AI配置详情
// @Description 根据ID获取AI配置详情
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Success 200 {object} map[string]interface{} "成功返回配置详情"
// @Failure 400 {object} map[string]interface{} "无效的配置ID"
// @Failure 404 {object} map[string]interface{} "配置不存在"
// @Router /api/v1/admin/ai-configs/{id} [get]
// @Security Bearer
func (c *AIController) GetConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的配置 ID",
		})
		return
	}

	config, err := c.configService.GetConfigByID(uint(id))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "配置不存在",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data":    config,
	})
}

// CreateConfig 创建 AI 配置
// @Summary 创建AI配置
// @Description 创建新的AI配置
// @Tags AI
// @Accept json
// @Produce json
// @Param config body models.AIConfig true "AI配置信息"
// @Param force_update query bool false "强制更新（禁用其他配置）"
// @Success 200 {object} map[string]interface{} "配置创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/admin/ai-configs [post]
// @Security Bearer
func (c *AIController) CreateConfig(ctx *gin.Context) {
	var config models.AIConfig
	if err := ctx.ShouldBindJSON(&config); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取 force_update 参数
	forceUpdate := ctx.Query("force_update") == "true"

	if err := c.configService.CreateConfig(&config, forceUpdate); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "配置创建成功",
		"data":    config,
	})
}

// UpdateConfig 更新 AI 配置
// @Summary 更新AI配置
// @Description 更新AI配置信息
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Param config body models.AIConfig true "AI配置信息"
// @Param force_update query bool false "强制更新（禁用其他配置）"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/admin/ai-configs/{id} [put]
// @Security Bearer
func (c *AIController) UpdateConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的配置 ID",
		})
		return
	}

	var config models.AIConfig
	if err := ctx.ShouldBindJSON(&config); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取 force_update 参数
	forceUpdate := ctx.Query("force_update") == "true"

	if err := c.configService.UpdateConfig(uint(id), &config, forceUpdate); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	// 返回更新后的配置
	updatedConfig, _ := c.configService.GetConfigByID(uint(id))
	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "配置更新成功",
		"data":    updatedConfig,
	})
}

// DeleteConfig 删除 AI 配置
// @Summary 删除AI配置
// @Description 删除指定的AI配置
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的配置ID"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/admin/ai-configs/{id} [delete]
// @Security Bearer
func (c *AIController) DeleteConfig(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的配置 ID",
		})
		return
	}

	if err := c.configService.DeleteConfig(uint(id)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "删除配置失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "配置删除成功",
	})
}

// GetAvailableModels 获取可用模型列表
// @Summary 获取可用AI模型列表
// @Description 根据AWS区域获取可用的AI模型列表
// @Tags AI
// @Accept json
// @Produce json
// @Param region query string true "AWS Region"
// @Success 200 {object} map[string]interface{} "成功返回模型列表"
// @Failure 400 {object} map[string]interface{} "缺少region参数"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/admin/ai-config/models [get]
// @Security Bearer
func (c *AIController) GetAvailableModels(ctx *gin.Context) {
	region := ctx.Query("region")
	if region == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "缺少 region 参数",
		})
		return
	}

	models, err := c.configService.GetAvailableModels(region)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "获取模型列表失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"region": region,
			"models": models,
		},
	})
}

// GetAvailableRegions 获取可用区域列表
// @Summary 获取可用AWS区域列表
// @Description 获取支持AI服务的AWS区域列表
// @Tags AI
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "成功返回区域列表"
// @Router /api/v1/admin/ai-config/regions [get]
// @Security Bearer
func (c *AIController) GetAvailableRegions(ctx *gin.Context) {
	regions := c.configService.GetAvailableRegions()
	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"regions": regions,
		},
	})
}

// AnalyzeErrorRequest 分析错误请求
// 安全说明：error_message 从数据库获取，不信任客户端输入，防止 prompt injection 攻击
type AnalyzeErrorRequest struct {
	TaskID uint `json:"task_id" binding:"required"`
}

// AnalyzeError 分析错误
// @Summary AI分析错误
// @Description 使用AI分析Terraform任务错误并提供解决方案。安全说明：错误信息从数据库获取，不信任客户端输入，防止 prompt injection 攻击
// @Tags AI
// @Accept json
// @Produce json
// @Param request body AnalyzeErrorRequest true "错误分析请求"
// @Success 200 {object} map[string]interface{} "分析完成"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 401 {object} map[string]interface{} "未授权"
// @Failure 404 {object} map[string]interface{} "任务不存在或无错误信息"
// @Failure 429 {object} map[string]interface{} "请求过于频繁"
// @Failure 500 {object} map[string]interface{} "分析失败"
// @Router /api/v1/ai/analyze-error [post]
// @Security Bearer
func (c *AIController) AnalyzeError(ctx *gin.Context) {
	var req AnalyzeErrorRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取当前用户 ID（从 JWT 或 session 中获取）
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": "未授权",
		})
		return
	}

	// 安全修复：从数据库获取任务信息，不信任客户端传入的 error_message
	// 这可以防止 prompt injection 攻击
	result, duration, err := c.analysisService.AnalyzeErrorByTaskID(
		req.TaskID,
		userID.(string),
	)

	if err != nil {
		// 检查是否是任务不存在或无错误信息
		if err.Error() == "任务不存在" || err.Error() == "任务没有错误信息，无需分析" {
			ctx.JSON(http.StatusNotFound, gin.H{
				"code":    404,
				"message": err.Error(),
			})
			return
		}

		// 检查是否是速率限制错误
		if retryAfter := c.extractRetryAfter(err.Error()); retryAfter > 0 {
			ctx.JSON(http.StatusTooManyRequests, gin.H{
				"code":    429,
				"message": err.Error(),
				"data": gin.H{
					"retry_after": retryAfter,
				},
			})
			return
		}

		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	// 获取使用的配置信息（用于验证）
	usedConfig, _ := c.configService.GetConfigForCapability("error_analysis")
	configInfo := gin.H{
		"id":           0,
		"service_type": "unknown",
		"model_id":     "unknown",
	}
	if usedConfig != nil {
		configInfo = gin.H{
			"id":           usedConfig.ID,
			"service_type": usedConfig.ServiceType,
			"model_id":     usedConfig.ModelID,
			"capabilities": usedConfig.Capabilities,
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "分析完成",
		"data": gin.H{
			"error_type":        result.ErrorType,
			"root_cause":        result.RootCause,
			"solutions":         result.Solutions,
			"prevention":        result.Prevention,
			"severity":          result.Severity,
			"analysis_duration": duration,
			"used_config":       configInfo, // 添加使用的配置信息
		},
	})
}

// GetTaskAnalysis 获取任务的分析结果
// @Summary 获取任务错误分析结果
// @Description 获取指定任务的AI错误分析结果
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "成功返回分析结果"
// @Failure 400 {object} map[string]interface{} "无效的任务ID"
// @Failure 404 {object} map[string]interface{} "未找到分析结果"
// @Failure 500 {object} map[string]interface{} "解析失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/error-analysis [get]
// @Security Bearer
func (c *AIController) GetTaskAnalysis(ctx *gin.Context) {
	taskIDStr := ctx.Param("task_id")
	taskID, err := strconv.ParseUint(taskIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的任务 ID",
		})
		return
	}

	analysis, err := c.configService.GetAnalysis(uint(taskID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "未找到分析结果",
		})
		return
	}

	// 解析 solutions
	result, err := c.configService.GetAnalysisWithSolutions(uint(taskID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "解析分析结果失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"id":                analysis.ID,
			"task_id":           analysis.TaskID,
			"error_type":        result.ErrorType,
			"root_cause":        result.RootCause,
			"solutions":         result.Solutions,
			"prevention":        result.Prevention,
			"severity":          result.Severity,
			"analysis_duration": analysis.AnalysisDuration,
			"created_at":        analysis.CreatedAt,
		},
	})
}

// extractRetryAfter 从错误消息中提取重试时间
func (c *AIController) extractRetryAfter(errMsg string) int {
	// 简单的字符串解析，提取 "请在 X 秒后重试" 中的 X
	var retryAfter int
	_, err := fmt.Sscanf(errMsg, "请求过于频繁，请在 %d 秒后重试", &retryAfter)
	if err == nil {
		return retryAfter
	}
	return 0
}

// BatchUpdatePriorities 批量更新配置优先级
// @Summary 批量更新AI配置优先级
// @Description 批量更新多个AI配置的优先级
// @Tags AI
// @Accept json
// @Produce json
// @Param updates body []services.PriorityUpdate true "优先级更新列表"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/admin/ai-configs/priorities [put]
// @Security Bearer
func (c *AIController) BatchUpdatePriorities(ctx *gin.Context) {
	var updates []services.PriorityUpdate
	if err := ctx.ShouldBindJSON(&updates); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	if err := c.configService.BatchUpdatePriorities(updates); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "更新优先级失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "优先级更新成功",
	})
}

// SetAsDefault 设置为默认配置
// @Summary 设置为默认AI配置
// @Description 将指定配置设置为默认配置（支持所有场景）
// @Tags AI
// @Accept json
// @Produce json
// @Param id path int true "配置ID"
// @Success 200 {object} map[string]interface{} "设置成功"
// @Failure 400 {object} map[string]interface{} "无效的配置ID"
// @Failure 500 {object} map[string]interface{} "设置失败"
// @Router /api/v1/admin/ai-configs/{id}/set-default [put]
// @Security Bearer
func (c *AIController) SetAsDefault(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "无效的配置 ID",
		})
		return
	}

	if err := c.configService.SetAsDefault(uint(id)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "设置默认配置失败: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "默认配置设置成功",
	})
}
