package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WorkspaceNotificationHandler Workspace 通知处理器
type WorkspaceNotificationHandler struct {
	db *gorm.DB
}

// NewWorkspaceNotificationHandler 创建 Workspace 通知处理器
func NewWorkspaceNotificationHandler(db *gorm.DB) *WorkspaceNotificationHandler {
	return &WorkspaceNotificationHandler{db: db}
}

// ListWorkspaceNotifications 获取 Workspace 的通知列表
// @Summary 获取 Workspace 的通知列表
// @Tags Workspace Notifications
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/notifications [get]
func (h *WorkspaceNotificationHandler) ListWorkspaceNotifications(c *gin.Context) {
	workspaceID := c.Param("id")

	// 验证 Workspace 存在
	var workspace models.Workspace
	if err := h.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get workspace"})
		return
	}

	// 获取 Workspace 关联的通知
	var workspaceNotifications []models.WorkspaceNotification
	if err := h.db.Preload("Notification").
		Where("workspace_id = ?", workspaceID).
		Find(&workspaceNotifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list workspace notifications"})
		return
	}

	// 获取全局通知
	var globalNotifications []models.NotificationConfig
	if err := h.db.Where("is_global = ? AND enabled = ?", true, true).Find(&globalNotifications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list global notifications"})
		return
	}

	// 构建 Workspace 通知响应
	var workspaceResponses []models.WorkspaceNotificationResponse
	for _, wn := range workspaceNotifications {
		workspaceResponses = append(workspaceResponses, wn.ToResponse())
	}

	// 构建全局通知响应
	var globalResponses []gin.H
	for _, gn := range globalNotifications {
		globalResponses = append(globalResponses, gin.H{
			"notification_id":   gn.NotificationID,
			"name":              gn.Name,
			"description":       gn.Description,
			"notification_type": gn.NotificationType,
			"endpoint_url":      gn.EndpointURL,
			"enabled":           gn.Enabled,
			"is_global":         true,
			"global_events":     gn.GlobalEvents,
			"created_at":        gn.CreatedAt,
			"updated_at":        gn.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"workspace_notifications": workspaceResponses,
		"global_notifications":    globalResponses,
	})
}

// AddWorkspaceNotification 为 Workspace 添加通知
// @Summary 为 Workspace 添加通知
// @Tags Workspace Notifications
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param request body models.CreateWorkspaceNotificationRequest true "创建请求"
// @Success 201 {object} models.WorkspaceNotificationResponse
// @Router /api/v1/workspaces/{workspace_id}/notifications [post]
func (h *WorkspaceNotificationHandler) AddWorkspaceNotification(c *gin.Context) {
	workspaceID := c.Param("id")

	// 验证 Workspace 存在
	var workspace models.Workspace
	if err := h.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get workspace"})
		return
	}

	var req models.CreateWorkspaceNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证 Notification 存在
	var notification models.NotificationConfig
	if err := h.db.Where("notification_id = ?", req.NotificationID).First(&notification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification"})
		return
	}

	// 检查是否已关联
	var existing models.WorkspaceNotification
	if err := h.db.Where("workspace_id = ? AND notification_id = ?", workspaceID, req.NotificationID).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Notification already added to this workspace"})
		return
	}

	// 设置默认事件
	events := req.Events
	if events == "" {
		events = "task_completed,task_failed"
	}

	// 获取当前用户
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	// 创建关联
	workspaceNotification := models.WorkspaceNotification{
		WorkspaceNotificationID: generateWorkspaceNotificationID(workspaceID, req.NotificationID),
		WorkspaceID:             workspaceID,
		NotificationID:          req.NotificationID,
		Events:                  events,
		Enabled:                 true,
		CreatedBy:               &userIDStr,
	}

	if err := h.db.Create(&workspaceNotification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add notification to workspace"})
		return
	}

	// 加载关联的 Notification
	workspaceNotification.Notification = &notification

	c.JSON(http.StatusCreated, workspaceNotification.ToResponse())
}

// UpdateWorkspaceNotification 更新 Workspace 通知配置
// @Summary 更新 Workspace 通知配置
// @Tags Workspace Notifications
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param workspace_notification_id path string true "Workspace Notification ID"
// @Param request body models.UpdateWorkspaceNotificationRequest true "更新请求"
// @Success 200 {object} models.WorkspaceNotificationResponse
// @Router /api/v1/workspaces/{workspace_id}/notifications/{workspace_notification_id} [put]
func (h *WorkspaceNotificationHandler) UpdateWorkspaceNotification(c *gin.Context) {
	workspaceID := c.Param("id")
	workspaceNotificationID := c.Param("workspace_notification_id")

	// 查找 Workspace Notification
	var workspaceNotification models.WorkspaceNotification
	if err := h.db.Preload("Notification").
		Where("workspace_notification_id = ? AND workspace_id = ?", workspaceNotificationID, workspaceID).
		First(&workspaceNotification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Workspace notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get workspace notification"})
		return
	}

	var req models.UpdateWorkspaceNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 更新字段
	if req.Events != nil {
		workspaceNotification.Events = *req.Events
	}
	if req.Enabled != nil {
		workspaceNotification.Enabled = *req.Enabled
	}

	workspaceNotification.UpdatedAt = time.Now()

	// 保存更新
	if err := h.db.Save(&workspaceNotification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update workspace notification"})
		return
	}

	c.JSON(http.StatusOK, workspaceNotification.ToResponse())
}

// DeleteWorkspaceNotification 删除 Workspace 通知关联
// @Summary 删除 Workspace 通知关联
// @Tags Workspace Notifications
// @Param workspace_id path string true "Workspace ID"
// @Param workspace_notification_id path string true "Workspace Notification ID"
// @Success 204
// @Router /api/v1/workspaces/{workspace_id}/notifications/{workspace_notification_id} [delete]
func (h *WorkspaceNotificationHandler) DeleteWorkspaceNotification(c *gin.Context) {
	workspaceID := c.Param("id")
	workspaceNotificationID := c.Param("workspace_notification_id")

	// 查找 Workspace Notification
	var workspaceNotification models.WorkspaceNotification
	if err := h.db.Where("workspace_notification_id = ? AND workspace_id = ?", workspaceNotificationID, workspaceID).
		First(&workspaceNotification).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Workspace notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get workspace notification"})
		return
	}

	// 删除关联
	if err := h.db.Delete(&workspaceNotification).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete workspace notification"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListNotificationLogs 获取 Workspace 的通知日志
// @Summary 获取 Workspace 的通知日志
// @Tags Workspace Notifications
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param event query string false "事件类型"
// @Param status query string false "状态"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/notification-logs [get]
func (h *WorkspaceNotificationHandler) ListNotificationLogs(c *gin.Context) {
	workspaceID := c.Param("id")
	event := c.Query("event")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询
	query := h.db.Model(&models.NotificationLog{}).
		Preload("Notification").
		Where("workspace_id = ?", workspaceID)

	if event != "" {
		query = query.Where("event = ?", event)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count notification logs"})
		return
	}

	// 获取列表
	var logs []models.NotificationLog
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list notification logs"})
		return
	}

	// 转换为响应格式
	responses := make([]models.NotificationLogResponse, len(logs))
	for i, log := range logs {
		responses[i] = log.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"logs": responses,
		"pagination": gin.H{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	})
}

// GetTaskNotificationLogs 获取任务的通知日志
// @Summary 获取任务的通知日志
// @Tags Workspace Notifications
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/tasks/{task_id}/notification-logs [get]
func (h *WorkspaceNotificationHandler) GetTaskNotificationLogs(c *gin.Context) {
	workspaceID := c.Param("id")
	taskIDStr := c.Param("task_id")

	taskID, err := strconv.ParseUint(taskIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取任务的通知日志
	var logs []models.NotificationLog
	if err := h.db.Preload("Notification").
		Where("workspace_id = ? AND task_id = ?", workspaceID, taskID).
		Order("created_at DESC").
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list notification logs"})
		return
	}

	// 转换为响应格式
	responses := make([]models.NotificationLogResponse, len(logs))
	for i, log := range logs {
		responses[i] = log.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"logs": responses,
	})
}

// GetNotificationLogDetail 获取通知日志详情
// @Summary 获取通知日志详情
// @Tags Workspace Notifications
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param log_id path string true "Log ID"
// @Success 200 {object} models.NotificationLog
// @Router /api/v1/workspaces/{workspace_id}/notification-logs/{log_id} [get]
func (h *WorkspaceNotificationHandler) GetNotificationLogDetail(c *gin.Context) {
	workspaceID := c.Param("id")
	logID := c.Param("log_id")

	var log models.NotificationLog
	if err := h.db.Preload("Notification").
		Where("log_id = ? AND workspace_id = ?", logID, workspaceID).
		First(&log).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Notification log not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get notification log"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// generateWorkspaceNotificationID 生成 Workspace 通知关联ID（本地函数）
func generateWorkspaceNotificationID(workspaceID, notificationID string) string {
	return fmt.Sprintf("wn-%s-%s-%d", workspaceID, notificationID, time.Now().Unix())
}
