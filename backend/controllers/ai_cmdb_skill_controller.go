package controllers

import (
	"encoding/json"
	"fmt"
	"iac-platform/services"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AICMDBSkillController AI + CMDB + Skill 控制器
type AICMDBSkillController struct {
	db      *gorm.DB
	service *services.AICMDBSkillService
}

// NewAICMDBSkillController 创建控制器实例
func NewAICMDBSkillController(db *gorm.DB) *AICMDBSkillController {
	return &AICMDBSkillController{
		db:      db,
		service: services.NewAICMDBSkillService(db),
	}
}

// ResourceInfoItem 资源信息项（用于接收前端传递的完整资源信息）
type ResourceInfoItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	ARN  string `json:"arn,omitempty"`
}

// GenerateConfigWithCMDBSkillRequest 请求结构
type GenerateConfigWithCMDBSkillRequest struct {
	ModuleID        uint                   `json:"module_id" binding:"required"`
	UserDescription string                 `json:"user_description" binding:"required,max=2000"`
	UserSelections  map[string]interface{} `json:"user_selections,omitempty"`   // 支持 string 或 []string
	ResourceInfoMap map[string]interface{} `json:"resource_info_map,omitempty"` // 完整的资源信息（包括 ARN）
	CurrentConfig   map[string]interface{} `json:"current_config,omitempty"`
	Mode            string                 `json:"mode,omitempty"`
	UseOptimized    bool                   `json:"use_optimized,omitempty"` // 是否使用优化版（并行执行 + AI 选择 Skills）
	ContextIDs      struct {
		WorkspaceID    string `json:"workspace_id,omitempty"`
		OrganizationID string `json:"organization_id,omitempty"`
	} `json:"context_ids,omitempty"`
}

// GenerateConfigWithCMDBSkill 使用 Skill 模式生成配置
// @Summary 使用 Skill 模式生成配置
// @Description 使用 Skill 组合模式 + CMDB 查询生成 Terraform 配置
// @Tags AI
// @Accept json
// @Produce json
// @Param request body GenerateConfigWithCMDBSkillRequest true "请求参数"
// @Success 200 {object} services.GenerateConfigWithCMDBResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/ai/form/generate-with-cmdb-skill [post]
func (c *AICMDBSkillController) GenerateConfigWithCMDBSkill(ctx *gin.Context) {
	var req GenerateConfigWithCMDBSkillRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	// 获取用户 ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 设置默认模式
	mode := req.Mode
	if mode == "" {
		mode = "new"
	}

	// 获取 AI 配置，检查是否启用优化版
	useOptimized := req.UseOptimized
	if !useOptimized {
		// 如果请求中没有指定，从 AI 配置中读取
		aiConfigService := services.NewAIConfigService(c.db)
		aiConfig, err := aiConfigService.GetConfigForCapability("form_generation")
		if err == nil && aiConfig != nil {
			useOptimized = aiConfig.UseOptimized
		}
	}

	// 调用服务（根据 use_optimized 参数选择方法）
	var response *services.GenerateConfigWithCMDBResponse
	var err error

	if useOptimized {
		// 使用优化版：并行执行 CMDB 查询 + AI 智能选择 Domain Skills
		response, err = c.service.GenerateConfigWithCMDBSkillOptimized(
			userID.(string),
			req.ModuleID,
			req.UserDescription,
			req.ContextIDs.WorkspaceID,
			req.ContextIDs.OrganizationID,
			req.UserSelections,
			req.CurrentConfig,
			mode,
			req.ResourceInfoMap, // 传递完整的资源信息
		)
	} else {
		// 使用原有版本：串行执行
		response, err = c.service.GenerateConfigWithCMDBSkill(
			userID.(string),
			req.ModuleID,
			req.UserDescription,
			req.ContextIDs.WorkspaceID,
			req.ContextIDs.OrganizationID,
			req.UserSelections,
			req.CurrentConfig,
			mode,
			req.ResourceInfoMap, // 传递完整的资源信息
		)
	}

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "配置生成失败",
			"details": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, response)
}

// GenerateConfigWithCMDBSkillSSE 使用 SSE 实时推送进度的配置生成
// @Summary 使用 SSE 实时推送进度的配置生成
// @Description 使用 Skill 组合模式 + CMDB 查询生成 Terraform 配置，通过 SSE 实时推送进度
// @Tags AI
// @Accept json
// @Produce text/event-stream
// @Param request body GenerateConfigWithCMDBSkillRequest true "请求参数"
// @Success 200 {object} services.ProgressEvent
// @Router /api/v1/ai/form/generate-with-cmdb-skill-sse [post]
func (c *AICMDBSkillController) GenerateConfigWithCMDBSkillSSE(ctx *gin.Context) {
	// 设置 SSE 响应头
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no") // 禁用 nginx 缓冲

	// 获取 ResponseWriter 的 Flusher
	flusher, ok := ctx.Writer.(http.Flusher)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	// 获取用户 ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		c.sendSSEError(ctx, flusher, "未授权", 0)
		return
	}

	// 从 body 解析请求参数
	var req GenerateConfigWithCMDBSkillRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.sendSSEError(ctx, flusher, "请求参数错误: "+err.Error(), 0)
		return
	}

	// 设置默认模式
	mode := req.Mode
	if mode == "" {
		mode = "new"
	}

	// 获取 AI 配置，检查是否启用优化版
	useOptimized := req.UseOptimized
	if !useOptimized {
		// 如果请求中没有指定，从 AI 配置中读取
		aiConfigService := services.NewAIConfigService(c.db)
		aiConfig, err := aiConfigService.GetConfigForCapability("form_generation")
		if err == nil && aiConfig != nil {
			useOptimized = aiConfig.UseOptimized
		}
	}

	// 记录开始时间
	startTime := time.Now()

	// 保存最后一个 progress 事件的 CompletedSteps
	var lastCompletedSteps []services.CompletedStep

	// 创建进度回调
	progressCallback := func(event services.ProgressEvent) {
		event.ElapsedMs = time.Since(startTime).Milliseconds()
		// 保存 CompletedSteps（用于最终的 complete 事件）
		if len(event.CompletedSteps) > 0 {
			lastCompletedSteps = event.CompletedSteps
		}
		c.sendSSEEvent(ctx, flusher, event)
	}

	log.Printf("[SSE] 开始配置生成: user_id=%s, module_id=%d, use_optimized=%v", userID, req.ModuleID, useOptimized)

	// 调用服务（带进度回调）
	var response *services.GenerateConfigWithCMDBResponse
	var err error

	if useOptimized {
		response, err = c.service.GenerateConfigWithCMDBSkillOptimizedWithProgress(
			userID.(string),
			req.ModuleID,
			req.UserDescription,
			req.ContextIDs.WorkspaceID,
			req.ContextIDs.OrganizationID,
			req.UserSelections,
			req.CurrentConfig,
			mode,
			req.ResourceInfoMap,
			progressCallback,
		)
	} else {
		response, err = c.service.GenerateConfigWithCMDBSkillWithProgress(
			userID.(string),
			req.ModuleID,
			req.UserDescription,
			req.ContextIDs.WorkspaceID,
			req.ContextIDs.OrganizationID,
			req.UserSelections,
			req.CurrentConfig,
			mode,
			req.ResourceInfoMap,
			progressCallback,
		)
	}

	if err != nil {
		c.sendSSEError(ctx, flusher, err.Error(), time.Since(startTime).Milliseconds())
		return
	}

	// 根据响应状态发送最终事件
	if response.Status == "need_selection" {
		c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
			Type:        "need_selection",
			StepName:    "需要选择",
			Message:     response.Message,
			CMDBLookups: response.CMDBLookups,
			ElapsedMs:   time.Since(startTime).Milliseconds(),
		})
	} else if response.Status == "blocked" {
		c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
			Type:      "error",
			StepName:  "已拦截",
			Message:   response.Message,
			Error:     response.Message,
			ElapsedMs: time.Since(startTime).Milliseconds(),
		})
	} else {
		c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
			Type:           "complete",
			StepName:       "完成",
			Message:        response.Message,
			Config:         response.Config,
			ElapsedMs:      time.Since(startTime).Milliseconds(),
			CompletedSteps: lastCompletedSteps, // 包含所有已完成步骤的耗时
		})
	}

	log.Printf("[SSE] 配置生成完成: user_id=%s, module_id=%d, elapsed=%dms", userID, req.ModuleID, time.Since(startTime).Milliseconds())
}

// sendSSEEvent 发送 SSE 事件
func (c *AICMDBSkillController) sendSSEEvent(ctx *gin.Context, flusher http.Flusher, event services.ProgressEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[SSE] JSON 序列化失败: %v", err)
		return
	}
	fmt.Fprintf(ctx.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
	flusher.Flush()
}

// sendSSEError 发送 SSE 错误事件
func (c *AICMDBSkillController) sendSSEError(ctx *gin.Context, flusher http.Flusher, errorMsg string, elapsedMs int64) {
	event := services.ProgressEvent{
		Type:      "error",
		StepName:  "错误",
		Error:     errorMsg,
		ElapsedMs: elapsedMs,
	}
	c.sendSSEEvent(ctx, flusher, event)
}

// PreviewAssembledPromptRequest 预览请求结构
type PreviewAssembledPromptRequest struct {
	Capability      string `json:"capability" binding:"required"`
	ModuleID        uint   `json:"module_id" binding:"required"`
	UserDescription string `json:"user_description" binding:"required"`
}

// PreviewAssembledPrompt 预览组装后的 Prompt
// @Summary 预览组装后的 Prompt
// @Description 预览 Skill 组装后的完整 Prompt（用于调试）
// @Tags AI
// @Accept json
// @Produce json
// @Param request body PreviewAssembledPromptRequest true "请求参数"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/ai/skill/preview-prompt [post]
func (c *AICMDBSkillController) PreviewAssembledPrompt(ctx *gin.Context) {
	var req PreviewAssembledPromptRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "请求参数错误",
			"details": err.Error(),
		})
		return
	}

	prompt, usedSkills, err := c.service.PreviewAssembledPrompt(
		req.Capability,
		req.ModuleID,
		req.UserDescription,
	)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":   "预览失败",
			"details": err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"prompt":      prompt,
		"used_skills": usedSkills,
		"skill_count": len(usedSkills),
	})
}
