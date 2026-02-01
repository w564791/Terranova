package models

import (
	"time"
)

// NotificationType 通知类型
type NotificationType string

const (
	NotificationTypeWebhook   NotificationType = "webhook"
	NotificationTypeLarkRobot NotificationType = "lark_robot"
)

// NotificationEvent 通知事件
type NotificationEvent string

const (
	NotificationEventTaskCreated      NotificationEvent = "task_created"
	NotificationEventTaskPlanning     NotificationEvent = "task_planning"
	NotificationEventTaskPlanned      NotificationEvent = "task_planned"
	NotificationEventTaskApplying     NotificationEvent = "task_applying"
	NotificationEventTaskCompleted    NotificationEvent = "task_completed"
	NotificationEventTaskFailed       NotificationEvent = "task_failed"
	NotificationEventTaskCancelled    NotificationEvent = "task_cancelled"
	NotificationEventApprovalRequired NotificationEvent = "approval_required"
	NotificationEventApprovalTimeout  NotificationEvent = "approval_timeout"
	NotificationEventDriftDetected    NotificationEvent = "drift_detected"
)

// NotificationLogStatus 通知日志状态
type NotificationLogStatus string

const (
	NotificationLogStatusPending NotificationLogStatus = "pending"
	NotificationLogStatusSending NotificationLogStatus = "sending"
	NotificationLogStatusSuccess NotificationLogStatus = "success"
	NotificationLogStatusFailed  NotificationLogStatus = "failed"
)

// NotificationConfig 通知配置
type NotificationConfig struct {
	// 基础字段
	ID             uint      `json:"id" gorm:"primaryKey"`
	NotificationID string    `json:"notification_id" gorm:"column:notification_id;type:varchar(50);uniqueIndex"` // 语义化ID，如 "notif-lark-ops"
	Name           string    `json:"name" gorm:"type:varchar(100);not null"`                                     // 名称
	Description    string    `json:"description" gorm:"type:text"`                                               // 描述
	CreatedBy      *string   `json:"created_by" gorm:"type:varchar(50)"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// 通知类型
	NotificationType NotificationType `json:"notification_type" gorm:"type:varchar(20);not null"` // webhook, lark_robot

	// Endpoint 配置
	EndpointURL string `json:"endpoint_url" gorm:"type:varchar(500);not null"`

	// 认证配置（加密存储，不返回给前端）
	SecretEncrypted string `json:"-" gorm:"column:secret_encrypted;type:text"`

	// 自定义 Headers
	CustomHeaders JSONB `json:"custom_headers" gorm:"type:jsonb;default:'{\"Content-Type\": \"application/json\"}'"`

	// 状态
	Enabled bool `json:"enabled" gorm:"default:true"`

	// 全局配置
	IsGlobal     bool   `json:"is_global" gorm:"default:false"`                                              // 是否为全局通知
	GlobalEvents string `json:"global_events" gorm:"type:varchar(500);default:'task_completed,task_failed'"` // 全局通知默认触发事件

	// 重试配置
	RetryCount           int `json:"retry_count" gorm:"default:3"`             // 重试次数
	RetryIntervalSeconds int `json:"retry_interval_seconds" gorm:"default:30"` // 重试间隔（秒）

	// 超时配置
	TimeoutSeconds int `json:"timeout_seconds" gorm:"default:30"` // 请求超时（秒）

	// 组织/团队归属
	OrganizationID *string `json:"organization_id" gorm:"type:varchar(50);index"`
	TeamID         *string `json:"team_id" gorm:"type:varchar(50);index"`
}

// TableName 指定表名
func (NotificationConfig) TableName() string {
	return "notification_configs"
}

// NotificationConfigResponse API 响应结构（用于隐藏敏感字段并添加计算字段）
type NotificationConfigResponse struct {
	ID                   uint             `json:"id"`
	NotificationID       string           `json:"notification_id"`
	Name                 string           `json:"name"`
	Description          string           `json:"description"`
	NotificationType     NotificationType `json:"notification_type"`
	EndpointURL          string           `json:"endpoint_url"`
	SecretSet            bool             `json:"secret_set"` // 是否设置了密钥
	CustomHeaders        JSONB            `json:"custom_headers"`
	Enabled              bool             `json:"enabled"`
	IsGlobal             bool             `json:"is_global"`
	GlobalEvents         string           `json:"global_events,omitempty"`
	RetryCount           int              `json:"retry_count"`
	RetryIntervalSeconds int              `json:"retry_interval_seconds"`
	TimeoutSeconds       int              `json:"timeout_seconds"`
	OrganizationID       *string          `json:"organization_id"`
	TeamID               *string          `json:"team_id"`
	WorkspaceCount       int              `json:"workspace_count"` // 关联的 Workspace 数量
	CreatedBy            *string          `json:"created_by"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
}

// ToResponse 将 NotificationConfig 转换为 API 响应结构
func (n *NotificationConfig) ToResponse(workspaceCount int) NotificationConfigResponse {
	return NotificationConfigResponse{
		ID:                   n.ID,
		NotificationID:       n.NotificationID,
		Name:                 n.Name,
		Description:          n.Description,
		NotificationType:     n.NotificationType,
		EndpointURL:          n.EndpointURL,
		SecretSet:            n.SecretEncrypted != "",
		CustomHeaders:        n.CustomHeaders,
		Enabled:              n.Enabled,
		IsGlobal:             n.IsGlobal,
		GlobalEvents:         n.GlobalEvents,
		RetryCount:           n.RetryCount,
		RetryIntervalSeconds: n.RetryIntervalSeconds,
		TimeoutSeconds:       n.TimeoutSeconds,
		OrganizationID:       n.OrganizationID,
		TeamID:               n.TeamID,
		WorkspaceCount:       workspaceCount,
		CreatedBy:            n.CreatedBy,
		CreatedAt:            n.CreatedAt,
		UpdatedAt:            n.UpdatedAt,
	}
}

// WorkspaceNotification Workspace 通知关联
type WorkspaceNotification struct {
	// 基础字段
	ID                      uint      `json:"id" gorm:"primaryKey"`
	WorkspaceNotificationID string    `json:"workspace_notification_id" gorm:"column:workspace_notification_id;type:varchar(50);uniqueIndex"` // 语义化ID
	WorkspaceID             string    `json:"workspace_id" gorm:"type:varchar(50);not null;index"`
	NotificationID          string    `json:"notification_id" gorm:"type:varchar(50);not null;index"`
	CreatedBy               *string   `json:"created_by" gorm:"type:varchar(50)"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`

	// 触发事件配置（逗号分隔）
	Events string `json:"events" gorm:"type:varchar(500);not null;default:'task_completed,task_failed'"`

	// 状态
	Enabled bool `json:"enabled" gorm:"default:true"`

	// 关联
	Notification *NotificationConfig `json:"notification,omitempty" gorm:"foreignKey:NotificationID;references:NotificationID"`
	Workspace    *Workspace          `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID;references:WorkspaceID"`
}

// TableName 指定表名
func (WorkspaceNotification) TableName() string {
	return "workspace_notifications"
}

// WorkspaceNotificationResponse Workspace Notification API 响应结构
type WorkspaceNotificationResponse struct {
	ID                      uint             `json:"id"`
	WorkspaceNotificationID string           `json:"workspace_notification_id"`
	WorkspaceID             string           `json:"workspace_id"`
	NotificationID          string           `json:"notification_id"`
	NotificationName        string           `json:"notification_name"`        // Notification 名称
	NotificationDescription string           `json:"notification_description"` // Notification 描述
	NotificationType        NotificationType `json:"notification_type"`        // Notification 类型
	Events                  string           `json:"events"`
	Enabled                 bool             `json:"enabled"`
	IsGlobal                bool             `json:"is_global"` // 是否为全局通知
	CreatedBy               *string          `json:"created_by"`
	CreatedAt               time.Time        `json:"created_at"`
	UpdatedAt               time.Time        `json:"updated_at"`
}

// ToResponse 将 WorkspaceNotification 转换为 API 响应结构
func (w *WorkspaceNotification) ToResponse() WorkspaceNotificationResponse {
	resp := WorkspaceNotificationResponse{
		ID:                      w.ID,
		WorkspaceNotificationID: w.WorkspaceNotificationID,
		WorkspaceID:             w.WorkspaceID,
		NotificationID:          w.NotificationID,
		Events:                  w.Events,
		Enabled:                 w.Enabled,
		IsGlobal:                false,
		CreatedBy:               w.CreatedBy,
		CreatedAt:               w.CreatedAt,
		UpdatedAt:               w.UpdatedAt,
	}
	if w.Notification != nil {
		resp.NotificationName = w.Notification.Name
		resp.NotificationDescription = w.Notification.Description
		resp.NotificationType = w.Notification.NotificationType
	}
	return resp
}

// NotificationLog 通知发送记录
type NotificationLog struct {
	// 基础字段
	ID        uint      `json:"id" gorm:"primaryKey"`
	LogID     string    `json:"log_id" gorm:"column:log_id;type:varchar(50);uniqueIndex"` // 语义化ID
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联
	TaskID                  *uint   `json:"task_id" gorm:"index"`
	WorkspaceID             *string `json:"workspace_id" gorm:"type:varchar(50);index"`
	NotificationID          string  `json:"notification_id" gorm:"type:varchar(50);not null;index"`
	WorkspaceNotificationID *string `json:"workspace_notification_id" gorm:"type:varchar(50);index"`

	// 事件信息
	Event NotificationEvent `json:"event" gorm:"type:varchar(50);not null"`

	// 发送状态
	Status NotificationLogStatus `json:"status" gorm:"type:varchar(20);not null;default:pending"`

	// 请求/响应
	RequestPayload     JSONB  `json:"request_payload" gorm:"type:jsonb"`
	RequestHeaders     JSONB  `json:"request_headers" gorm:"type:jsonb"`
	ResponseStatusCode *int   `json:"response_status_code"`
	ResponseBody       string `json:"response_body" gorm:"type:text"`
	ErrorMessage       string `json:"error_message" gorm:"type:text"`

	// 重试信息
	RetryCount    int        `json:"retry_count" gorm:"default:0"`
	MaxRetryCount int        `json:"max_retry_count" gorm:"default:3"`
	NextRetryAt   *time.Time `json:"next_retry_at"`

	// 时间
	SentAt      *time.Time `json:"sent_at"`
	CompletedAt *time.Time `json:"completed_at"`

	// 关联
	Notification *NotificationConfig `json:"notification,omitempty" gorm:"foreignKey:NotificationID;references:NotificationID"`
}

// TableName 指定表名
func (NotificationLog) TableName() string {
	return "notification_logs"
}

// NotificationLogResponse Notification Log API 响应结构
type NotificationLogResponse struct {
	ID                 uint                  `json:"id"`
	LogID              string                `json:"log_id"`
	TaskID             *uint                 `json:"task_id"`
	WorkspaceID        *string               `json:"workspace_id"`
	NotificationID     string                `json:"notification_id"`
	NotificationName   string                `json:"notification_name"`
	NotificationType   NotificationType      `json:"notification_type"`
	Event              NotificationEvent     `json:"event"`
	Status             NotificationLogStatus `json:"status"`
	ResponseStatusCode *int                  `json:"response_status_code"`
	ErrorMessage       string                `json:"error_message,omitempty"`
	RetryCount         int                   `json:"retry_count"`
	SentAt             *time.Time            `json:"sent_at"`
	CompletedAt        *time.Time            `json:"completed_at"`
	CreatedAt          time.Time             `json:"created_at"`
}

// ToResponse 将 NotificationLog 转换为 API 响应结构
func (l *NotificationLog) ToResponse() NotificationLogResponse {
	resp := NotificationLogResponse{
		ID:                 l.ID,
		LogID:              l.LogID,
		TaskID:             l.TaskID,
		WorkspaceID:        l.WorkspaceID,
		NotificationID:     l.NotificationID,
		Event:              l.Event,
		Status:             l.Status,
		ResponseStatusCode: l.ResponseStatusCode,
		ErrorMessage:       l.ErrorMessage,
		RetryCount:         l.RetryCount,
		SentAt:             l.SentAt,
		CompletedAt:        l.CompletedAt,
		CreatedAt:          l.CreatedAt,
	}
	if l.Notification != nil {
		resp.NotificationName = l.Notification.Name
		resp.NotificationType = l.Notification.NotificationType
	}
	return resp
}

// ============================================================================
// 请求结构体
// ============================================================================

// CreateNotificationRequest 创建通知配置请求
type CreateNotificationRequest struct {
	Name                 string            `json:"name" binding:"required"`              // 名称
	Description          string            `json:"description"`                          // 描述
	NotificationType     NotificationType  `json:"notification_type" binding:"required"` // 类型
	EndpointURL          string            `json:"endpoint_url" binding:"required"`      // Endpoint URL
	Secret               string            `json:"secret"`                               // 密钥（可选）
	CustomHeaders        map[string]string `json:"custom_headers"`                       // 自定义 Headers
	IsGlobal             bool              `json:"is_global"`                            // 是否为全局通知
	GlobalEvents         string            `json:"global_events"`                        // 全局通知默认触发事件
	RetryCount           int               `json:"retry_count"`                          // 重试次数
	RetryIntervalSeconds int               `json:"retry_interval_seconds"`               // 重试间隔（秒）
	TimeoutSeconds       int               `json:"timeout_seconds"`                      // 超时时间（秒）
	OrganizationID       *string           `json:"organization_id"`                      // 组织ID
	TeamID               *string           `json:"team_id"`                              // 团队ID
}

// UpdateNotificationRequest 更新通知配置请求
type UpdateNotificationRequest struct {
	Name                 *string            `json:"name"`                   // 名称
	Description          *string            `json:"description"`            // 描述
	EndpointURL          *string            `json:"endpoint_url"`           // Endpoint URL
	Secret               *string            `json:"secret"`                 // 密钥（空字符串表示清除）
	CustomHeaders        *map[string]string `json:"custom_headers"`         // 自定义 Headers
	Enabled              *bool              `json:"enabled"`                // 是否启用
	IsGlobal             *bool              `json:"is_global"`              // 是否为全局通知
	GlobalEvents         *string            `json:"global_events"`          // 全局通知默认触发事件
	RetryCount           *int               `json:"retry_count"`            // 重试次数
	RetryIntervalSeconds *int               `json:"retry_interval_seconds"` // 重试间隔（秒）
	TimeoutSeconds       *int               `json:"timeout_seconds"`        // 超时时间（秒）
}

// CreateWorkspaceNotificationRequest 创建 Workspace 通知请求
type CreateWorkspaceNotificationRequest struct {
	NotificationID string `json:"notification_id" binding:"required"` // Notification ID
	Events         string `json:"events"`                             // 触发事件（逗号分隔）
}

// UpdateWorkspaceNotificationRequest 更新 Workspace 通知请求
type UpdateWorkspaceNotificationRequest struct {
	Events  *string `json:"events"`  // 触发事件（逗号分隔）
	Enabled *bool   `json:"enabled"` // 是否启用
}

// TestNotificationRequest 测试通知请求
type TestNotificationRequest struct {
	Event       string `json:"event"`        // 测试事件类型
	TestMessage string `json:"test_message"` // 测试消息
}

// TestNotificationResponse 测试通知响应
type TestNotificationResponse struct {
	Success        bool   `json:"success"`
	StatusCode     int    `json:"status_code"`
	ResponseTimeMs int64  `json:"response_time_ms"`
	Message        string `json:"message"`
	ErrorMessage   string `json:"error_message,omitempty"`
}
