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

// ManifestAIController manifest 编辑器的 AI 控制器（生成/修复 + 检查）
type ManifestAIController struct {
	db           *gorm.DB
	genService   *services.ManifestAIService
	checkService *services.ManifestCheckService
}

// NewManifestAIController 创建控制器实例
func NewManifestAIController(db *gorm.DB) *ManifestAIController {
	return &ManifestAIController{
		db:           db,
		genService:   services.NewManifestAIService(db),
		checkService: services.NewManifestCheckService(db),
	}
}

// GenerateResourceRequest 生成/修复请求
type GenerateResourceRequest struct {
	Description    string `json:"description" binding:"required,max=2000"`
	CurrentContent string `json:"current_content,omitempty"` // 当前选区或文件内容（修复时提供）
	ContextIDs     struct {
		WorkspaceID    string `json:"workspace_id,omitempty"`
		OrganizationID string `json:"organization_id,omitempty"`
	} `json:"context_ids,omitempty"`
}

// CheckDraftRequest 检查请求
type CheckDraftRequest struct {
	FilePath   string `json:"file_path,omitempty"`
	Content    string `json:"content" binding:"required"` // 选区或当前文件内容
	StartLine  int    `json:"start_line,omitempty"`       // content 在文件中的起始行号(选区时>0,整文件为1或省略)
	ContextIDs struct {
		WorkspaceID    string `json:"workspace_id,omitempty"`
		OrganizationID string `json:"organization_id,omitempty"`
	} `json:"context_ids,omitempty"`
}

// GenerateResourceSSE 生成/修复 manifest 资源（SSE 实时进度）
// @Router /api/v1/ai/manifest/generate-resource-sse [post]
func (c *ManifestAIController) GenerateResourceSSE(ctx *gin.Context) {
	flusher, ok := c.prepareSSE(ctx)
	if !ok {
		return
	}

	userID, exists := ctx.Get("user_id")
	if !exists {
		c.sendSSEError(ctx, flusher, "未授权", 0)
		return
	}

	var req GenerateResourceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.sendSSEError(ctx, flusher, "请求参数错误: "+err.Error(), 0)
		return
	}

	startTime := time.Now()
	progressCallback := func(event services.ProgressEvent) {
		event.ElapsedMs = time.Since(startTime).Milliseconds()
		c.sendSSEEvent(ctx, flusher, event)
	}

	log.Printf("[ManifestAI-SSE] 开始资源生成: user_id=%s", userID)

	result, err := c.genService.GenerateResourceWithProgress(
		userID.(string),
		req.Description,
		req.ContextIDs.WorkspaceID,
		req.ContextIDs.OrganizationID,
		req.CurrentContent,
		progressCallback,
	)
	if err != nil {
		c.sendSSEError(ctx, flusher, err.Error(), time.Since(startTime).Milliseconds())
		return
	}

	if result.Status == "blocked" {
		c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
			Type:      "error",
			StepName:  "已拦截",
			Message:   result.Message,
			Error:     result.Message,
			ElapsedMs: time.Since(startTime).Milliseconds(),
		})
		return
	}

	c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
		Type:       "complete",
		StepName:   "完成",
		Message:    result.Message,
		HCL:        result.HCL,
		UsageLogID: result.UsageLogID,
		ElapsedMs:  time.Since(startTime).Milliseconds(),
	})

	log.Printf("[ManifestAI-SSE] 资源生成完成: user_id=%s, elapsed=%dms", userID, time.Since(startTime).Milliseconds())
}

// CheckDraftSSE 检查 manifest 草稿（SSE 实时进度）
// @Router /api/v1/ai/manifest/check-sse [post]
func (c *ManifestAIController) CheckDraftSSE(ctx *gin.Context) {
	flusher, ok := c.prepareSSE(ctx)
	if !ok {
		return
	}

	userID, exists := ctx.Get("user_id")
	if !exists {
		c.sendSSEError(ctx, flusher, "未授权", 0)
		return
	}

	var req CheckDraftRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.sendSSEError(ctx, flusher, "请求参数错误: "+err.Error(), 0)
		return
	}

	startTime := time.Now()
	progressCallback := func(event services.ProgressEvent) {
		event.ElapsedMs = time.Since(startTime).Milliseconds()
		c.sendSSEEvent(ctx, flusher, event)
	}

	log.Printf("[ManifestAI-SSE] 开始草稿检查: user_id=%s, file=%s", userID, req.FilePath)

	result, err := c.checkService.CheckDraftWithProgress(
		userID.(string),
		req.ContextIDs.WorkspaceID,
		req.FilePath,
		req.Content,
		req.StartLine,
		progressCallback,
	)
	if err != nil {
		c.sendSSEError(ctx, flusher, err.Error(), time.Since(startTime).Milliseconds())
		return
	}

	if result.Status == "blocked" {
		c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
			Type:      "error",
			StepName:  "已拦截",
			Message:   result.Message,
			Error:     result.Message,
			ElapsedMs: time.Since(startTime).Milliseconds(),
		})
		return
	}

	c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
		Type:       "complete",
		StepName:   "完成",
		Message:    result.Message,
		Issues:     result.Issues,
		UsageLogID: result.UsageLogID,
		ElapsedMs:  time.Since(startTime).Milliseconds(),
	})

	log.Printf("[ManifestAI-SSE] 草稿检查完成: user_id=%s, issues=%d, elapsed=%dms",
		userID, len(result.Issues), time.Since(startTime).Milliseconds())
}

// prepareSSE 设置 SSE 响应头并获取 Flusher
func (c *ManifestAIController) prepareSSE(ctx *gin.Context) (http.Flusher, bool) {
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	flusher, ok := ctx.Writer.(http.Flusher)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return nil, false
	}
	return flusher, true
}

// sendSSEEvent 发送 SSE 事件
func (c *ManifestAIController) sendSSEEvent(ctx *gin.Context, flusher http.Flusher, event services.ProgressEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ManifestAI-SSE] JSON 序列化失败: %v", err)
		return
	}
	fmt.Fprintf(ctx.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
	flusher.Flush()
}

// sendSSEError 发送 SSE 错误事件
func (c *ManifestAIController) sendSSEError(ctx *gin.Context, flusher http.Flusher, errorMsg string, elapsedMs int64) {
	c.sendSSEEvent(ctx, flusher, services.ProgressEvent{
		Type:      "error",
		StepName:  "错误",
		Error:     errorMsg,
		ElapsedMs: elapsedMs,
	})
}
