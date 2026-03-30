package handlers

import (
	"log"
	"net/http"
	"strconv"

	"iac-platform/internal/websocket"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TakeoverHandler 接管请求处理器
type TakeoverHandler struct {
	editingService *services.ResourceEditingService
	wsHub          *websocket.Hub
	db             *gorm.DB
}

// NewTakeoverHandler 创建接管请求处理器
func NewTakeoverHandler(db *gorm.DB, wsHub *websocket.Hub) *TakeoverHandler {
	return &TakeoverHandler{
		editingService: services.NewResourceEditingService(db),
		wsHub:          wsHub,
		db:             db,
	}
}

// RequestTakeover 请求接管
// @Summary 请求接管编辑
// @Description 请求接管其他用户的编辑会话
// @Tags 资源编辑
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param body body object true "请求体"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/takeover-request [post]
// @Security BearerAuth
func (h *TakeoverHandler) RequestTakeover(c *gin.Context) {
	resourceID, err := strconv.ParseUint(c.Param("resource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的资源ID"})
		return
	}

	var req struct {
		TargetSessionID    string `json:"target_session_id" binding:"required"`
		RequesterSessionID string `json:"requester_session_id" binding:"required"` // 请求方的编辑session_id
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取当前用户信息
	userID, _ := c.Get("user_id")
	userName, _ := c.Get("username")
	// 使用前端传递的编辑session_id，而不是登录session_id
	requesterSessionID := req.RequesterSessionID

	// 创建接管请求
	request, err := h.editingService.RequestTakeover(
		uint(resourceID),
		userID.(string),
		userName.(string),
		requesterSessionID,
		req.TargetSessionID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 通过WebSocket通知被接管方
	log.Printf("🔔 准备发送接管请求通知: target_session=%s", req.TargetSessionID)
	log.Printf("🔔 当前已连接的sessions: %v", h.wsHub.GetConnectedSessions())

	h.wsHub.SendToSessionOrBroadcast(req.TargetSessionID, websocket.Message{
		Type:      "takeover_request",
		SessionID: req.TargetSessionID,
		Data:      request,
	})

	c.JSON(http.StatusOK, gin.H{
		"request_id": request.ID,
		"status":     request.Status,
		"expires_at": request.ExpiresAt,
	})
}

// RespondToTakeover 响应接管请求
// @Summary 响应接管请求
// @Description 同意或拒绝接管请求
// @Tags 资源编辑
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param body body object true "请求体"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/takeover-response [post]
// @Security BearerAuth
func (h *TakeoverHandler) RespondToTakeover(c *gin.Context) {
	var req struct {
		RequestID uint `json:"request_id" binding:"required"`
		Approved  bool `json:"approved"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取请求详情（用于WebSocket通知）
	request, err := h.editingService.GetRequestStatus(req.RequestID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return
	}

	// 响应接管请求
	if err := h.editingService.RespondToTakeover(req.RequestID, req.Approved); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 通过WebSocket通知请求方
	messageType := "takeover_rejected"
	if req.Approved {
		messageType = "takeover_approved"
	}

	h.wsHub.SendToSessionOrBroadcast(request.RequesterSession, websocket.Message{
		Type:      messageType,
		SessionID: request.RequesterSession,
		Data:      request,
	})

	c.JSON(http.StatusOK, gin.H{
		"status": map[bool]string{true: "approved", false: "rejected"}[req.Approved],
	})
}

// GetPendingRequests 获取待处理请求
// @Summary 获取待处理的接管请求
// @Description 获取当前session的待处理接管请求
// @Tags 资源编辑
// @Produce json
// @Param id path string true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param target_session query string true "目标session ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/pending-requests [get]
// @Security BearerAuth
func (h *TakeoverHandler) GetPendingRequests(c *gin.Context) {
	targetSession := c.Query("target_session")
	if targetSession == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少target_session参数"})
		return
	}

	requests, err := h.editingService.GetPendingRequests(targetSession)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": requests,
	})
}

// GetRequestStatus 获取请求状态
// @Summary 获取接管请求状态
// @Description 获取指定接管请求的当前状态
// @Tags 资源编辑
// @Produce json
// @Param id path string true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request_id path int true "请求ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "请求不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/request-status/{request_id} [get]
// @Security BearerAuth
func (h *TakeoverHandler) GetRequestStatus(c *gin.Context) {
	requestID, err := strconv.ParseUint(c.Param("request_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求ID"})
		return
	}

	// 获取请求状态前的原始状态
	var originalStatus string
	var originalRequest struct {
		Status        string
		TargetSession string
	}
	h.db.Table("takeover_requests").Select("status, target_session").Where("id = ?", requestID).First(&originalRequest)
	originalStatus = originalRequest.Status

	request, err := h.editingService.GetRequestStatus(uint(requestID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "请求不存在"})
		return
	}

	// 如果状态从pending变为approved（超时自动接管），通知被接管方
	if originalStatus == "pending" && request.Status == "approved" {
		h.wsHub.SendToSessionOrBroadcast(request.TargetSession, websocket.Message{
			Type:      "force_takeover",
			SessionID: request.TargetSession,
			Data: map[string]interface{}{
				"message": "接管请求已超时，您的编辑会话已被接管",
			},
		})
	}

	c.JSON(http.StatusOK, request)
}

// ForceTakeover 强制接管
// @Summary 强制接管编辑
// @Description 强制接管其他用户的编辑会话，无需等待确认
// @Tags 资源编辑
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param body body object true "请求体"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/force-takeover [post]
// @Security BearerAuth
func (h *TakeoverHandler) ForceTakeover(c *gin.Context) {
	resourceID, err := strconv.ParseUint(c.Param("resource_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的资源ID"})
		return
	}

	var req struct {
		TargetSessionID    string `json:"target_session_id" binding:"required"`
		RequesterSessionID string `json:"requester_session_id" binding:"required"` // 请求方的编辑session_id
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}

	// 获取当前用户信息
	userID, _ := c.Get("user_id")
	// 使用前端传递的编辑session_id，而不是登录session_id
	requesterSessionID := req.RequesterSessionID

	// 直接执行接管，不需要等待确认
	if err := h.editingService.TakeoverEditing(
		uint(resourceID),
		userID.(string),
		requesterSessionID,
		req.TargetSessionID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 通过WebSocket通知被接管方
	h.wsHub.SendToSessionOrBroadcast(req.TargetSessionID, websocket.Message{
		Type:      "force_takeover",
		SessionID: req.TargetSessionID,
		Data: map[string]interface{}{
			"message": "您的编辑会话已被强制接管",
		},
	})

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "强制接管成功",
	})
}
