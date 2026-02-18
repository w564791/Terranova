package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// NotificationSender 通知发送服务
type NotificationSender struct {
	db         *gorm.DB
	httpClient *http.Client
	baseURL    string
}

// NewNotificationSender 创建通知发送服务
func NewNotificationSender(db *gorm.DB, baseURL string) *NotificationSender {
	return &NotificationSender{
		db:      db,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// generateNotificationLogID 生成通知日志ID
func generateNotificationLogID() string {
	return fmt.Sprintf("nlog-%d", time.Now().UnixNano())
}

// notificationTimePtr 返回时间指针
func notificationTimePtr(t time.Time) *time.Time {
	return &t
}

// SendNotification 发送通知
func (s *NotificationSender) SendNotification(
	ctx context.Context,
	config *models.NotificationConfig,
	event models.NotificationEvent,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
) error {
	// 创建日志记录
	log := &models.NotificationLog{
		LogID:          generateNotificationLogID(),
		NotificationID: config.NotificationID,
		Event:          event,
		Status:         models.NotificationLogStatusPending,
		MaxRetryCount:  config.RetryCount,
	}
	if task != nil {
		log.TaskID = &task.ID
	}
	if workspace != nil {
		log.WorkspaceID = &workspace.WorkspaceID
	}

	if err := s.db.Create(log).Error; err != nil {
		return fmt.Errorf("failed to create notification log: %w", err)
	}

	// 根据通知类型发送
	var err error
	switch config.NotificationType {
	case models.NotificationTypeWebhook:
		err = s.sendWebhook(ctx, config, event, task, workspace, log)
	case models.NotificationTypeLarkRobot:
		err = s.sendLarkRobot(ctx, config, event, task, workspace, log)
	default:
		err = fmt.Errorf("unsupported notification type: %s", config.NotificationType)
	}

	return err
}

// SendTestNotification 发送测试通知
func (s *NotificationSender) SendTestNotification(
	ctx context.Context,
	config *models.NotificationConfig,
	event string,
	testMessage string,
) (*models.TestNotificationResponse, error) {
	startTime := time.Now()

	// 构建测试数据
	var payload map[string]interface{}
	var payloadBytes []byte
	var err error

	if config.NotificationType == models.NotificationTypeLarkRobot {
		payload = s.buildTestLarkPayload(event, testMessage)
		// 添加签名
		if config.SecretEncrypted != "" {
			secret, err := crypto.DecryptValue(config.SecretEncrypted)
			if err != nil {
				return &models.TestNotificationResponse{
					Success:      false,
					ErrorMessage: fmt.Sprintf("Failed to decrypt secret: %v", err),
				}, nil
			}
			timestamp := time.Now().Unix()
			sign, err := s.genLarkSign(secret, timestamp)
			if err != nil {
				return &models.TestNotificationResponse{
					Success:      false,
					ErrorMessage: fmt.Sprintf("Failed to generate signature: %v", err),
				}, nil
			}
			payload["timestamp"] = fmt.Sprintf("%d", timestamp)
			payload["sign"] = sign
		}
	} else {
		payload = s.buildTestWebhookPayload(event, testMessage)
	}

	payloadBytes, err = json.Marshal(payload)
	if err != nil {
		return &models.TestNotificationResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to marshal payload: %v", err),
		}, nil
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "POST", config.EndpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return &models.TestNotificationResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Failed to create request: %v", err),
		}, nil
	}

	// 设置 Headers
	req.Header.Set("Content-Type", "application/json")

	if config.NotificationType == models.NotificationTypeWebhook {
		req.Header.Set("X-IaC-Event", event)

		// 添加自定义 Headers
		if config.CustomHeaders != nil {
			for key, value := range config.CustomHeaders {
				if v, ok := value.(string); ok {
					req.Header.Set(key, v)
				}
			}
		}

		// 添加 HMAC 签名
		if config.SecretEncrypted != "" {
			secret, err := crypto.DecryptValue(config.SecretEncrypted)
			if err == nil {
				signature := s.calculateWebhookSignature(payloadBytes, secret)
				req.Header.Set("X-IaC-Signature", signature)
			}
		}
	}

	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return &models.TestNotificationResponse{
			Success:        false,
			ResponseTimeMs: time.Since(startTime).Milliseconds(),
			ErrorMessage:   fmt.Sprintf("Request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// 读取响应
	responseBody, _ := io.ReadAll(resp.Body)

	responseTimeMs := time.Since(startTime).Milliseconds()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return &models.TestNotificationResponse{
			Success:        true,
			StatusCode:     resp.StatusCode,
			ResponseTimeMs: responseTimeMs,
			Message:        fmt.Sprintf("Test notification sent successfully. Response: %s", string(responseBody)),
		}, nil
	}

	return &models.TestNotificationResponse{
		Success:        false,
		StatusCode:     resp.StatusCode,
		ResponseTimeMs: responseTimeMs,
		ErrorMessage:   fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(responseBody)),
	}, nil
}

// sendWebhook 发送 Webhook 通知
func (s *NotificationSender) sendWebhook(
	ctx context.Context,
	config *models.NotificationConfig,
	event models.NotificationEvent,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
	log *models.NotificationLog,
) error {
	// 构建请求体
	payload := s.buildWebhookPayload(event, task, workspace)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return s.updateLogError(log, err)
	}

	log.RequestPayload = payload
	log.Status = models.NotificationLogStatusSending
	log.SentAt = notificationTimePtr(time.Now())
	s.db.Save(log)

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "POST", config.EndpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.updateLogError(log, err)
	}

	// 设置 Headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-IaC-Event", string(event))

	// 添加自定义 Headers
	if config.CustomHeaders != nil {
		for key, value := range config.CustomHeaders {
			if v, ok := value.(string); ok {
				req.Header.Set(key, v)
			}
		}
	}

	// 添加 HMAC 签名
	if config.SecretEncrypted != "" {
		secret, err := crypto.DecryptValue(config.SecretEncrypted)
		if err == nil {
			signature := s.calculateWebhookSignature(payloadBytes, secret)
			req.Header.Set("X-IaC-Signature", signature)
		}
	}

	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return s.updateLogError(log, err)
	}
	defer resp.Body.Close()

	// 读取响应体
	responseBody, _ := io.ReadAll(resp.Body)
	if len(responseBody) > 1000 {
		responseBody = responseBody[:1000] // 截断保存
	}

	// 更新日志
	log.ResponseStatusCode = &resp.StatusCode
	log.ResponseBody = string(responseBody)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Status = models.NotificationLogStatusSuccess
	} else {
		log.Status = models.NotificationLogStatusFailed
		log.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	log.CompletedAt = notificationTimePtr(time.Now())

	return s.db.Save(log).Error
}

// sendLarkRobot 发送 Lark Robot 通知
func (s *NotificationSender) sendLarkRobot(
	ctx context.Context,
	config *models.NotificationConfig,
	event models.NotificationEvent,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
	log *models.NotificationLog,
) error {
	// 构建 Lark 消息卡片
	payload := s.buildLarkCardPayload(event, task, workspace)

	// 添加签名（如果配置了 secret）
	if config.SecretEncrypted != "" {
		secret, err := crypto.DecryptValue(config.SecretEncrypted)
		if err != nil {
			return s.updateLogError(log, fmt.Errorf("failed to decrypt secret: %w", err))
		}
		timestamp := time.Now().Unix()
		sign, err := s.genLarkSign(secret, timestamp)
		if err != nil {
			return s.updateLogError(log, err)
		}
		payload["timestamp"] = fmt.Sprintf("%d", timestamp)
		payload["sign"] = sign
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return s.updateLogError(log, err)
	}

	log.RequestPayload = payload
	log.Status = models.NotificationLogStatusSending
	log.SentAt = notificationTimePtr(time.Now())
	s.db.Save(log)

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "POST", config.EndpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return s.updateLogError(log, err)
	}

	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return s.updateLogError(log, err)
	}
	defer resp.Body.Close()

	// 读取响应体
	responseBody, _ := io.ReadAll(resp.Body)
	if len(responseBody) > 1000 {
		responseBody = responseBody[:1000]
	}

	// 更新日志
	log.ResponseStatusCode = &resp.StatusCode
	log.ResponseBody = string(responseBody)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Status = models.NotificationLogStatusSuccess
	} else {
		log.Status = models.NotificationLogStatusFailed
		log.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	log.CompletedAt = notificationTimePtr(time.Now())

	return s.db.Save(log).Error
}

// genLarkSign 生成 Lark 签名
func (s *NotificationSender) genLarkSign(secret string, timestamp int64) (string, error) {
	stringToSign := fmt.Sprintf("%v", timestamp) + "\n" + secret

	var data []byte
	h := hmac.New(sha256.New, []byte(stringToSign))
	_, err := h.Write(data)
	if err != nil {
		return "", err
	}

	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return signature, nil
}

// calculateWebhookSignature 计算 Webhook HMAC 签名
func (s *NotificationSender) calculateWebhookSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// buildWebhookPayload 构建 Webhook 请求体
func (s *NotificationSender) buildWebhookPayload(
	event models.NotificationEvent,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
) map[string]interface{} {
	payload := map[string]interface{}{
		"event":     event,
		"timestamp": time.Now().Format(time.RFC3339),
	}

	if task != nil {
		taskData := map[string]interface{}{
			"id":         task.ID,
			"type":       task.TaskType,
			"status":     task.Status,
			"created_by": task.CreatedBy,
			"created_at": task.CreatedAt,
			"app_url":    fmt.Sprintf("%s/workspaces/%s/tasks/%d", s.baseURL, task.WorkspaceID, task.ID),
		}
		if task.Description != "" {
			taskData["description"] = task.Description
		}
		payload["task"] = taskData
	}

	if workspace != nil {
		payload["workspace"] = map[string]interface{}{
			"id":                workspace.WorkspaceID,
			"name":              workspace.Name,
			"terraform_version": workspace.TerraformVersion,
			"app_url":           fmt.Sprintf("%s/workspaces/%s", s.baseURL, workspace.WorkspaceID),
		}
	}

	return payload
}

// buildTestWebhookPayload 构建测试 Webhook 请求体
func (s *NotificationSender) buildTestWebhookPayload(event string, testMessage string) map[string]interface{} {
	return map[string]interface{}{
		"event":        event,
		"timestamp":    time.Now().Format(time.RFC3339),
		"test":         true,
		"test_message": testMessage,
		"task": map[string]interface{}{
			"id":          0,
			"type":        "test",
			"status":      "completed",
			"description": testMessage,
			"created_by":  "system",
			"created_at":  time.Now().Format(time.RFC3339),
		},
		"workspace": map[string]interface{}{
			"id":                "test-workspace",
			"name":              "Test Workspace",
			"terraform_version": "1.5.0",
		},
	}
}

// buildLarkCardPayload 构建 Lark 消息卡片
func (s *NotificationSender) buildLarkCardPayload(
	event models.NotificationEvent,
	task *models.WorkspaceTask,
	workspace *models.Workspace,
) map[string]interface{} {
	// 根据事件类型选择主题颜色和标题
	var title, template string
	switch event {
	case models.NotificationEventTaskCompleted:
		title = "✅ Task Completed"
		template = "green"
	case models.NotificationEventTaskFailed:
		title = "❌ Task Failed"
		template = "red"
	case models.NotificationEventApprovalRequired:
		title = "⏳ Approval Required"
		template = "orange"
	case models.NotificationEventTaskPlanning, models.NotificationEventTaskApplying:
		title = "🔄 Task In Progress"
		template = "blue"
	case models.NotificationEventTaskCreated:
		title = "📝 Task Created"
		template = "blue"
	case models.NotificationEventTaskCancelled:
		title = "🚫 Task Cancelled"
		template = "grey"
	case models.NotificationEventDriftDetected:
		title = " Drift Detected"
		template = "orange"
	default:
		title = "📢 IaC Platform Notification"
		template = "blue"
	}

	// 构建内容
	var contentParts []string
	if workspace != nil {
		contentParts = append(contentParts, fmt.Sprintf("**Workspace:** %s", workspace.Name))
	}
	if task != nil {
		contentParts = append(contentParts, fmt.Sprintf("**Task:** #%d", task.ID))
		if task.Description != "" {
			contentParts = append(contentParts, fmt.Sprintf("**Description:** %s", task.Description))
		}
		contentParts = append(contentParts, fmt.Sprintf("**Status:** %s", task.Status))
		// 获取用户真实名字
		createdByName := "Unknown"
		if task.CreatedBy != nil && s.db != nil {
			var user models.User
			if err := s.db.Where("user_id = ?", *task.CreatedBy).First(&user).Error; err == nil {
				createdByName = user.Username
			} else {
				createdByName = *task.CreatedBy // 如果查询失败，使用 user_id
			}
		} else if task.CreatedBy != nil {
			createdByName = *task.CreatedBy
		}
		contentParts = append(contentParts, fmt.Sprintf("**Created by:** %s", createdByName))
		// 添加时间（使用本地时区）
		contentParts = append(contentParts, fmt.Sprintf("**Time:** %s", time.Now().Local().Format("2006-01-02 15:04:05")))
	}

	content := strings.Join(contentParts, "\n")

	// 构建卡片元素
	elements := []interface{}{
		map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"content": content,
				"tag":     "lark_md",
			},
		},
	}

	// 添加查看详情按钮
	if task != nil && workspace != nil {
		elements = append(elements,
			map[string]interface{}{
				"tag": "hr",
			},
			map[string]interface{}{
				"tag": "action",
				"actions": []interface{}{
					map[string]interface{}{
						"tag": "button",
						"text": map[string]interface{}{
							"content": "View Details",
							"tag":     "lark_md",
						},
						"url":  fmt.Sprintf("%s/workspaces/%s/tasks/%d", s.baseURL, workspace.WorkspaceID, task.ID),
						"type": "primary",
					},
				},
			},
		)
	}

	// 构建卡片
	card := map[string]interface{}{
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"content": title,
				"tag":     "plain_text",
			},
			"template": template,
		},
		"elements": elements,
	}

	return map[string]interface{}{
		"msg_type": "interactive",
		"card":     card,
	}
}

// buildTestLarkPayload 构建测试 Lark 消息卡片
func (s *NotificationSender) buildTestLarkPayload(event string, testMessage string) map[string]interface{} {
	return map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"content": "🧪 Test Notification",
					"tag":     "plain_text",
				},
				"template": "blue",
			},
			"elements": []interface{}{
				map[string]interface{}{
					"tag": "div",
					"text": map[string]interface{}{
						"content": fmt.Sprintf("**Event:** %s\n**Message:** %s\n**Time:** %s",
							event, testMessage, time.Now().Format(time.RFC3339)),
						"tag": "lark_md",
					},
				},
				map[string]interface{}{
					"tag": "hr",
				},
				map[string]interface{}{
					"tag": "note",
					"elements": []interface{}{
						map[string]interface{}{
							"tag":     "plain_text",
							"content": "This is a test notification from IaC Platform",
						},
					},
				},
			},
		},
	}
}

// updateLogError 更新日志错误状态
func (s *NotificationSender) updateLogError(log *models.NotificationLog, err error) error {
	log.Status = models.NotificationLogStatusFailed
	log.ErrorMessage = err.Error()
	log.CompletedAt = notificationTimePtr(time.Now())
	s.db.Save(log)
	return err
}

// TriggerNotifications 触发 Workspace 的所有通知
// 这个方法会被任务执行流程调用
func (s *NotificationSender) TriggerNotifications(
	ctx context.Context,
	workspaceID string,
	event models.NotificationEvent,
	task *models.WorkspaceTask,
) error {
	// 获取 Workspace
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	// 获取 Workspace 关联的通知配置
	var workspaceNotifications []models.WorkspaceNotification
	if err := s.db.Preload("Notification").
		Where("workspace_id = ? AND enabled = ?", workspaceID, true).
		Find(&workspaceNotifications).Error; err != nil {
		return fmt.Errorf("failed to get workspace notifications: %w", err)
	}

	// 获取全局通知配置
	var globalNotifications []models.NotificationConfig
	if err := s.db.Where("is_global = ? AND enabled = ?", true, true).
		Find(&globalNotifications).Error; err != nil {
		return fmt.Errorf("failed to get global notifications: %w", err)
	}

	eventStr := string(event)

	// 发送 Workspace 关联的通知
	for _, wn := range workspaceNotifications {
		if wn.Notification == nil || !wn.Notification.Enabled {
			continue
		}
		// 检查事件是否匹配
		if !s.eventMatches(wn.Events, eventStr) {
			continue
		}
		// 异步发送通知
		go func(config *models.NotificationConfig) {
			if err := s.SendNotification(ctx, config, event, task, &workspace); err != nil {
				// 记录错误但不阻塞
				fmt.Printf("Failed to send notification %s: %v\n", config.NotificationID, err)
			}
		}(wn.Notification)
	}

	// 发送全局通知
	for _, gn := range globalNotifications {
		// 检查是否已经在 Workspace 通知中（避免重复发送）
		alreadySent := false
		for _, wn := range workspaceNotifications {
			if wn.NotificationID == gn.NotificationID {
				alreadySent = true
				break
			}
		}
		if alreadySent {
			continue
		}
		// 检查事件是否匹配
		if !s.eventMatches(gn.GlobalEvents, eventStr) {
			continue
		}
		// 异步发送通知
		go func(config models.NotificationConfig) {
			if err := s.SendNotification(ctx, &config, event, task, &workspace); err != nil {
				fmt.Printf("Failed to send global notification %s: %v\n", config.NotificationID, err)
			}
		}(gn)
	}

	return nil
}

// eventMatches 检查事件是否匹配
func (s *NotificationSender) eventMatches(events string, event string) bool {
	eventList := strings.Split(events, ",")
	for _, e := range eventList {
		if strings.TrimSpace(e) == event {
			return true
		}
	}
	return false
}
