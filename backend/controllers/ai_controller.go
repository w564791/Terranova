package controllers

import (
	"fmt"
	appconfig "iac-platform/internal/config"
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
// @Summary List AI configurations
// @Description Get all AI configurations
// @Tags AI Config
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Configuration list"
// @Failure 500 {object} map[string]interface{} "Server error"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-configs [get]
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
// @Summary Get AI configuration detail
// @Description Get AI configuration detail by ID
// @Tags AI Config
// @Accept json
// @Produce json
// @Param id path int true "Configuration ID"
// @Success 200 {object} map[string]interface{} "Configuration detail"
// @Failure 400 {object} map[string]interface{} "Invalid configuration ID"
// @Failure 404 {object} map[string]interface{} "Configuration not found"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-configs/{id} [get]
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
// @Summary Create AI configuration
// @Description Create a new AI configuration
// @Tags AI Config
// @Accept json
// @Produce json
// @Param config body models.AIConfig true "AI configuration"
// @Param force_update query bool false "Force update (disable other configs)"
// @Success 200 {object} map[string]interface{} "Configuration created"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Creation failed"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-configs [post]
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
// @Summary Update AI configuration
// @Description Update AI configuration by ID
// @Tags AI Config
// @Accept json
// @Produce json
// @Param id path int true "Configuration ID"
// @Param config body models.AIConfig true "AI configuration"
// @Param force_update query bool false "Force update (disable other configs)"
// @Success 200 {object} map[string]interface{} "Update successful"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Update failed"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-configs/{id} [put]
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
// @Summary Delete AI configuration
// @Description Delete an AI configuration by ID
// @Tags AI Config
// @Accept json
// @Produce json
// @Param id path int true "Configuration ID"
// @Success 200 {object} map[string]interface{} "Deletion successful"
// @Failure 400 {object} map[string]interface{} "Invalid configuration ID"
// @Failure 500 {object} map[string]interface{} "Deletion failed"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-configs/{id} [delete]
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
// @Summary Get available AI models
// @Description Get available AI models for a given AWS region
// @Tags AI Config
// @Accept json
// @Produce json
// @Param region query string true "AWS Region"
// @Success 200 {object} map[string]interface{} "Model list"
// @Failure 400 {object} map[string]interface{} "Missing region parameter"
// @Failure 500 {object} map[string]interface{} "Fetch failed"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-config/models [get]
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

// ListOpenAIModels 获取 OpenAI 兼容 API 的模型列表（POST，避免 API Key 暴露在 URL）
// @Summary List OpenAI-compatible models
// @Description List available models from an OpenAI-compatible API endpoint. API Key priority: request body > DB config > DASHSCOPE_API_KEY env var.
// @Tags AI Config
// @Accept json
// @Produce json
// @Param request body object true "Request with base_url, api_key, config_id"
// @Success 200 {object} map[string]interface{} "Model list"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Fetch failed"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-config/openai-models [post]
func (c *AIController) ListOpenAIModels(ctx *gin.Context) {
	var req struct {
		BaseURL  string `json:"base_url"`
		APIKey   string `json:"api_key"`
		ConfigID *uint  `json:"config_id"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	baseURL := req.BaseURL
	apiKey := req.APIKey

	// 编辑模式：从 DB 补全
	if req.ConfigID != nil && (baseURL == "" || apiKey == "") {
		if existing, err := c.configService.GetConfigByID(*req.ConfigID); err == nil {
			if baseURL == "" {
				baseURL = existing.BaseURL
			}
			if apiKey == "" {
				apiKey = existing.APIKey
			}
		}
	}

	// 兜底：环境变量
	if apiKey == "" {
		cfg := appconfig.Load()
		apiKey = cfg.AI.DashScopeAPIKey
	}

	if baseURL == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "缺少 base_url"})
		return
	}

	models, err := c.configService.ListOpenAICompatibleModels(baseURL, apiKey)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取模型列表失败: " + err.Error()})
		return
	}

	// 成功获取模型列表 = key 有效，如果是用户新填的 key + 编辑模式，顺便持久化
	if req.APIKey != "" && req.ConfigID != nil {
		c.configService.UpdateAPIKey(*req.ConfigID, req.APIKey)
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 200, "data": models})
}

// GetAvailableRegions 获取可用区域列表
// @Summary Get available AWS regions
// @Description Get AWS regions that support AI services
// @Tags AI Config
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Region list"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-config/regions [get]
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
// @Summary Analyze task error with AI
// @Description Use AI to analyze Terraform task errors and provide solutions. Error messages are fetched from DB, not from client input, to prevent prompt injection.
// @Tags AI Analysis
// @Accept json
// @Produce json
// @Param request body AnalyzeErrorRequest true "Error analysis request"
// @Success 200 {object} map[string]interface{} "Analysis complete"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "Task not found or no error"
// @Failure 429 {object} map[string]interface{} "Too many requests"
// @Failure 500 {object} map[string]interface{} "Analysis failed"
// @Security BearerAuth
// @Router /api/v1/ai/analyze-error [post]
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
// @Summary Get task error analysis result
// @Description Get AI error analysis result for a specific task
// @Tags AI Analysis
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Analysis result"
// @Failure 400 {object} map[string]interface{} "Invalid task ID"
// @Failure 404 {object} map[string]interface{} "Analysis not found"
// @Failure 500 {object} map[string]interface{} "Parse failed"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/error-analysis [get]
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
// @Summary Batch update AI configuration priorities
// @Description Batch update priorities for multiple AI configurations
// @Tags AI Config
// @Accept json
// @Produce json
// @Param updates body []services.PriorityUpdate true "Priority update list"
// @Success 200 {object} map[string]interface{} "Update successful"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 500 {object} map[string]interface{} "Update failed"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-configs/priorities [put]
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
// @Summary Set default AI configuration
// @Description Set a configuration as the default for all scenarios
// @Tags AI Config
// @Accept json
// @Produce json
// @Param id path int true "Configuration ID"
// @Success 200 {object} map[string]interface{} "Set successful"
// @Failure 400 {object} map[string]interface{} "Invalid configuration ID"
// @Failure 500 {object} map[string]interface{} "Set failed"
// @Security BearerAuth
// @Router /api/v1/global/settings/ai-configs/{id}/set-default [put]
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
