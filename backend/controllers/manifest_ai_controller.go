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

// ManifestAIController manifest 编辑器的 AI 控制器（生成/修复 + 检查 + 会话）
type ManifestAIController struct {
	db             *gorm.DB
	genService     *services.ManifestAIService
	checkService   *services.ManifestCheckService
	sessionService *services.ManifestAISessionService
}

// NewManifestAIController 创建控制器实例
func NewManifestAIController(db *gorm.DB) *ManifestAIController {
	return &ManifestAIController{
		db:             db,
		genService:     services.NewManifestAIService(db),
		checkService:   services.NewManifestCheckService(db),
		sessionService: services.NewManifestAISessionService(db),
	}
}

// GenerateResourceRequest 生成/修复请求
type GenerateResourceRequest struct {
	Description    string `json:"description" binding:"required,max=2000"`
	CurrentContent string `json:"current_content,omitempty"` // 当前选区或文件内容（修复时提供）
	SessionID      string `json:"session_id,omitempty"`      // 非空则把本次交互落入该会话(向后兼容:空则不持久化)
	ContextIDs     struct {
		WorkspaceID    string `json:"workspace_id,omitempty"`
		OrganizationID string `json:"organization_id,omitempty"`
	} `json:"context_ids,omitempty"`
}

// CheckFile 单个待检查文件(跨文件检查时多于一个)
type CheckFile struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	StartLine int    `json:"start_line,omitempty"` // content 在文件中的起始行(整文件=1,选区=选区起始行)
}

// CheckDraftRequest 检查请求
type CheckDraftRequest struct {
	Files      []CheckFile `json:"files" binding:"required,min=1"` // 当前文件 + 跨文件关联文件
	SessionID  string      `json:"session_id,omitempty"`           // 非空则把本次检查落入该会话
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
		Warnings:   result.Warnings,
		UsageLogID: result.UsageLogID,
		ElapsedMs:  time.Since(startTime).Milliseconds(),
	})

	// 落会话(session_id 为空则跳过)
	c.sessionService.AppendExchange(
		req.SessionID, userID.(string), "generate",
		services.MarshalJSONContent(map[string]string{"description": req.Description}),
		services.MarshalJSONContent(map[string]interface{}{"hcl": result.HCL, "warnings": result.Warnings}),
	)

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

	log.Printf("[ManifestAI-SSE] 开始草稿检查: user_id=%s, files=%d", userID, len(req.Files))

	files := make([]services.CheckFileInput, 0, len(req.Files))
	for _, f := range req.Files {
		files = append(files, services.CheckFileInput{Path: f.Path, Content: f.Content, StartLine: f.StartLine})
	}

	result, err := c.checkService.CheckDraftWithProgress(
		userID.(string),
		req.ContextIDs.WorkspaceID,
		files,
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
		Type:           "complete",
		StepName:       "完成",
		Message:        result.Message,
		Issues:         result.Issues,
		UsageLogID:     result.UsageLogID,
		CompletedSteps: result.CompletedSteps,
		ElapsedMs:      time.Since(startTime).Milliseconds(),
	})

	// 落会话(session_id 为空则跳过)
	checkedPaths := make([]string, 0, len(req.Files))
	for _, f := range req.Files {
		checkedPaths = append(checkedPaths, f.Path)
	}
	c.sessionService.AppendExchange(
		req.SessionID, userID.(string), "check",
		services.MarshalJSONContent(map[string]interface{}{"files": checkedPaths}),
		services.MarshalJSONContent(map[string]interface{}{"issues": result.Issues, "message": result.Message}),
	)

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

// ========== AI 会话 CRUD(按 manifest + 用户隔离) ==========

func (c *ManifestAIController) userID(ctx *gin.Context) (string, bool) {
	v, ok := ctx.Get("user_id")
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return "", false
	}
	uid, isStr := v.(string) // 防御:user_id 非 string 时不 panic(中间件目前总存 string)
	if !isStr || uid == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return "", false
	}
	return uid, true
}

// ListSessions GET /ai/manifest/sessions?manifest_id=&org_id=
func (c *ManifestAIController) ListSessions(ctx *gin.Context) {
	uid, ok := c.userID(ctx)
	if !ok {
		return
	}
	manifestID := ctx.Query("manifest_id")
	if manifestID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "缺少 manifest_id"})
		return
	}
	sessions, err := c.sessionService.ListSessions(manifestID, uid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

// CreateSession POST /ai/manifest/sessions  body: {manifest_id, org_id, title?}
func (c *ManifestAIController) CreateSession(ctx *gin.Context) {
	uid, ok := c.userID(ctx)
	if !ok {
		return
	}
	var req struct {
		ManifestID string `json:"manifest_id" binding:"required"`
		OrgID      string `json:"org_id"`
		Title      string `json:"title"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if req.Title == "" {
		req.Title = "新会话"
	}
	sess, err := c.sessionService.CreateSession(req.ManifestID, req.OrgID, uid, req.Title)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, sess)
}

// GetSessionMessages GET /ai/manifest/sessions/:sid/messages
func (c *ManifestAIController) GetSessionMessages(ctx *gin.Context) {
	uid, ok := c.userID(ctx)
	if !ok {
		return
	}
	msgs, err := c.sessionService.GetMessages(ctx.Param("sid"), uid)
	if err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"messages": msgs})
}

// DeleteSession DELETE /ai/manifest/sessions/:sid
func (c *ManifestAIController) DeleteSession(ctx *gin.Context) {
	uid, ok := c.userID(ctx)
	if !ok {
		return
	}
	if err := c.sessionService.DeleteSession(ctx.Param("sid"), uid); err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"ok": true})
}
