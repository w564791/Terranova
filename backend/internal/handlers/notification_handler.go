package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NotificationHandler 通知配置处理器
type NotificationHandler struct {
	db *gorm.DB
}

// NewNotificationHandler 创建通知配置处理器
func NewNotificationHandler(db *gorm.DB) *NotificationHandler {
	return &NotificationHandler{db: db}
}

// generateNotificationID 生成通知配置ID
func generateNotificationID(name string) string {
	// 将名称转换为小写，替换空格为破折号
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")
	id = strings.ReplaceAll(id, "_", "-")
	// 只保留字母、数字和破折号
	reg := regexp.MustCompile("[^a-z0-9-]")
	id = reg.ReplaceAllString(id, "")
	// 添加前缀
	return fmt.Sprintf("notif-%s", id)
}

// generateNotificationLogID 生成通知日志ID
func generateNotificationLogID() string {
	return fmt.Sprintf("nlog-%d", time.Now().UnixNano())
}

// ListNotifications 获取通知配置列表
// @Summary 获取通知配置列表
// @Tags Notifications
// @Produce json
// @Param organization_id query string false "组织ID"
// @Param team_id query string false "团队ID"
// @Param notification_type query string false "通知类型"
// @Param is_global query bool false "是否全局"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/notifications [get]
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	// 解析查询参数
	organizationID := c.Query("organization_id")
	teamID := c.Query("team_id")
	notificationType := c.Query("notification_type")
	isGlobalStr := c.Query("is_global")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询
	query := h.db.Model(&models.NotificationConfig{})

	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}
	if teamID != "" {
		query = query.Where("team_id = ?", teamID)
	}
	if notificationType != "" {
		query = query.Where("notification_type = ?", notificationType)
	}
	if isGlobalStr != "" {
		isGlobal := isGlobalStr == "true"
		query = query.Where("is_global = ?", isGlobal)
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count notifications"})
		return
	}

	// 获取列表
	var notifications []models.NotificationConfig
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list notifications"})
		return
	}

	// 获取每个通知的 Workspace 数量
	responses := make([]models.NotificationConfigResponse, len(notifications))
	for i, n := range notifications {
		var count int64
		h.db.Model(&models.WorkspaceNotification{}).Where("notification_id = ?", n.NotificationID).Count(&count)
		responses[i] = n.ToResponse(int(count))
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": responses,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// GetNotification 获取单个通知配置
// @Summary 获取单个通知配置
// @Tags Notifications
// @Produce json
// @Param notification_id path string true "通知配置ID"
// @Success 200 {object} models.NotificationConfigResponse
// @Router /api/v1/notifications/{notification_id} [get]
func (h *NotificationHandler) GetNotification(c *gin.Context) {
	notificationID := c.Param("notification_id")

	var notification models.NotificationConfig
	if err := h.db.Where("notification_id = ?", notificationID).First(&notification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification"})
		return
	}

	// 获取 Workspace 数量
	var count int64
	h.db.Model(&models.WorkspaceNotification{}).Where("notification_id = ?", notificationID).Count(&count)

	c.JSON(http.StatusOK, notification.ToResponse(int(count)))
}

// CreateNotification 创建通知配置
// @Summary 创建通知配置
// @Tags Notifications
// @Accept json
// @Produce json
// @Param request body models.CreateNotificationRequest true "创建请求"
// @Success 201 {object} models.NotificationConfigResponse
// @Router /api/v1/notifications [post]
func (h *NotificationHandler) CreateNotification(c *gin.Context) {
	var req models.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证名称格式
	nameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !nameRegex.MatchString(req.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name can only contain letters, numbers, dashes and underscores"})
		return
	}

	// 验证通知类型
	if req.NotificationType != models.NotificationTypeWebhook && req.NotificationType != models.NotificationTypeLarkRobot {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification type. Must be 'webhook' or 'lark_robot'"})
		return
	}

	// 生成 ID
	notificationID := generateNotificationID(req.Name)

	// 检查 ID 是否已存在
	var existing models.NotificationConfig
	if err := h.db.Where("notification_id = ?", notificationID).First(&existing).Error; err == nil {
		// ID 已存在，添加时间戳
		notificationID = fmt.Sprintf("%s-%d", notificationID, time.Now().Unix())
	}

	// 设置默认值
	retryCount := req.RetryCount
	if retryCount <= 0 {
		retryCount = 3
	}
	if retryCount > 10 {
		retryCount = 10
	}

	retryIntervalSeconds := req.RetryIntervalSeconds
	if retryIntervalSeconds <= 0 {
		retryIntervalSeconds = 30
	}

	timeoutSeconds := req.TimeoutSeconds
	if timeoutSeconds < 5 {
		timeoutSeconds = 30
	}
	if timeoutSeconds > 120 {
		timeoutSeconds = 120
	}

	globalEvents := req.GlobalEvents
	if globalEvents == "" {
		globalEvents = "task_completed,task_failed"
	}

	// 获取当前用户
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	// 创建通知配置
	notification := models.NotificationConfig{
		NotificationID:       notificationID,
		Name:                 req.Name,
		Description:          req.Description,
		NotificationType:     req.NotificationType,
		EndpointURL:          req.EndpointURL,
		Enabled:              true,
		IsGlobal:             req.IsGlobal,
		GlobalEvents:         globalEvents,
		RetryCount:           retryCount,
		RetryIntervalSeconds: retryIntervalSeconds,
		TimeoutSeconds:       timeoutSeconds,
		OrganizationID:       req.OrganizationID,
		TeamID:               req.TeamID,
		CreatedBy:            &userIDStr,
	}

	// 加密密钥
	if req.Secret != "" {
		encrypted, err := crypto.EncryptValue(req.Secret)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt secret"})
			return
		}
		notification.SecretEncrypted = encrypted
	}

	// 处理自定义 Headers
	if req.CustomHeaders != nil {
		notification.CustomHeaders = models.JSONB{}
		for k, v := range req.CustomHeaders {
			notification.CustomHeaders[k] = v
		}
	} else {
		notification.CustomHeaders = models.JSONB{"Content-Type": "application/json"}
	}

	// 保存到数据库
	if err := h.db.Create(&notification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create notification"})
		return
	}

	c.JSON(http.StatusCreated, notification.ToResponse(0))
}

// UpdateNotification 更新通知配置
// @Summary 更新通知配置
// @Tags Notifications
// @Accept json
// @Produce json
// @Param notification_id path string true "通知配置ID"
// @Param request body models.UpdateNotificationRequest true "更新请求"
// @Success 200 {object} models.NotificationConfigResponse
// @Router /api/v1/notifications/{notification_id} [put]
func (h *NotificationHandler) UpdateNotification(c *gin.Context) {
	notificationID := c.Param("notification_id")

	var notification models.NotificationConfig
	if err := h.db.Where("notification_id = ?", notificationID).First(&notification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification"})
		return
	}

	var req models.UpdateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新字段
	if req.Name != nil {
		nameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
		if !nameRegex.MatchString(*req.Name) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Name can only contain letters, numbers, dashes and underscores"})
			return
		}
		notification.Name = *req.Name
	}
	if req.Description != nil {
		notification.Description = *req.Description
	}
	if req.EndpointURL != nil {
		notification.EndpointURL = *req.EndpointURL
	}
	if req.Secret != nil {
		if *req.Secret == "" {
			// 清除密钥
			notification.SecretEncrypted = ""
		} else {
			// 加密新密钥
			encrypted, err := crypto.EncryptValue(*req.Secret)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt secret"})
				return
			}
			notification.SecretEncrypted = encrypted
		}
	}
	if req.CustomHeaders != nil {
		notification.CustomHeaders = models.JSONB{}
		for k, v := range *req.CustomHeaders {
			notification.CustomHeaders[k] = v
		}
	}
	if req.Enabled != nil {
		notification.Enabled = *req.Enabled
	}
	if req.IsGlobal != nil {
		notification.IsGlobal = *req.IsGlobal
	}
	if req.GlobalEvents != nil {
		notification.GlobalEvents = *req.GlobalEvents
	}
	if req.RetryCount != nil {
		retryCount := *req.RetryCount
		if retryCount < 0 {
			retryCount = 0
		}
		if retryCount > 10 {
			retryCount = 10
		}
		notification.RetryCount = retryCount
	}
	if req.RetryIntervalSeconds != nil {
		notification.RetryIntervalSeconds = *req.RetryIntervalSeconds
	}
	if req.TimeoutSeconds != nil {
		timeoutSeconds := *req.TimeoutSeconds
		if timeoutSeconds < 5 {
			timeoutSeconds = 5
		}
		if timeoutSeconds > 120 {
			timeoutSeconds = 120
		}
		notification.TimeoutSeconds = timeoutSeconds
	}

	notification.UpdatedAt = time.Now()

	// 保存更新
	if err := h.db.Save(&notification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update notification"})
		return
	}

	// 获取 Workspace 数量
	var count int64
	h.db.Model(&models.WorkspaceNotification{}).Where("notification_id = ?", notificationID).Count(&count)

	c.JSON(http.StatusOK, notification.ToResponse(int(count)))
}

// DeleteNotification 删除通知配置
// @Summary 删除通知配置
// @Tags Notifications
// @Param notification_id path string true "通知配置ID"
// @Success 204
// @Router /api/v1/notifications/{notification_id} [delete]
func (h *NotificationHandler) DeleteNotification(c *gin.Context) {
	notificationID := c.Param("notification_id")

	var notification models.NotificationConfig
	if err := h.db.Where("notification_id = ?", notificationID).First(&notification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification"})
		return
	}

	// 删除通知配置（关联的 workspace_notifications 和 notification_logs 会级联删除）
	if err := h.db.Delete(&notification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete notification"})
		return
	}

	c.Status(http.StatusNoContent)
}

// TestNotification 测试通知配置
// @Summary 测试通知配置
// @Tags Notifications
// @Accept json
// @Produce json
// @Param notification_id path string true "通知配置ID"
// @Param request body models.TestNotificationRequest true "测试请求"
// @Success 200 {object} models.TestNotificationResponse
// @Router /api/v1/notifications/{notification_id}/test [post]
func (h *NotificationHandler) TestNotification(c *gin.Context) {
	notificationID := c.Param("notification_id")

	var notification models.NotificationConfig
	if err := h.db.Where("notification_id = ?", notificationID).First(&notification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification"})
		return
	}

	var req models.TestNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 使用默认值
		req.Event = "task_completed"
		req.TestMessage = "This is a test notification from IaC Platform"
	}

	// 设置默认值
	if req.Event == "" {
		req.Event = "task_completed"
	}
	if req.TestMessage == "" {
		req.TestMessage = "This is a test notification from IaC Platform"
	}

	// 调用 NotificationSender 发送测试通知
	sender := h.getNotificationSender()
	result, err := sender.SendTestNotification(c.Request.Context(), &notification, req.Event, req.TestMessage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send test notification: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// getNotificationSender 获取通知发送服务
func (h *NotificationHandler) getNotificationSender() *services.NotificationSender {
	// 从环境变量获取 baseURL，默认为空
	baseURL := ""
	return services.NewNotificationSender(h.db, baseURL)
}

// GetAvailableNotifications 获取可用的通知配置（用于 Workspace 添加通知时选择）
// @Summary 获取可用的通知配置
// @Tags Notifications
// @Produce json
// @Param workspace_id query string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/notifications/available [get]
func (h *NotificationHandler) GetAvailableNotifications(c *gin.Context) {
	workspaceID := c.Query("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	// 获取所有启用的非全局通知
	var notifications []models.NotificationConfig
	if err := h.db.Where("enabled = ? AND is_global = ?", true, false).Find(&notifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list notifications"})
		return
	}

	// 获取该 Workspace 已关联的通知
	var existingIDs []string
	h.db.Model(&models.WorkspaceNotification{}).
		Where("workspace_id = ?", workspaceID).
		Pluck("notification_id", &existingIDs)

	existingMap := make(map[string]bool)
	for _, id := range existingIDs {
		existingMap[id] = true
	}

	// 过滤掉已关联的通知
	var available []models.NotificationConfigResponse
	for _, n := range notifications {
		if !existingMap[n.NotificationID] {
			available = append(available, n.ToResponse(0))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": available,
	})
}
