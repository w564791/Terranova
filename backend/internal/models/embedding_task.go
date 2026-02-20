package models

import (
	"time"
)

// EmbeddingTask Embedding 生成任务队列表
// 用于异步处理资源的 embedding 生成
type EmbeddingTask struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	ResourceID  uint   `gorm:"column:resource_id;not null;uniqueIndex:uk_embedding_task_resource" json:"resource_id"` // 关联的资源 ID
	WorkspaceID string `gorm:"column:workspace_id;type:varchar(50);not null;index:idx_embedding_tasks_workspace" json:"workspace_id"`

	// 任务状态
	Status       string `gorm:"column:status;type:varchar(20);not null;default:'pending';index:idx_embedding_tasks_status" json:"status"` // pending, processing, completed, failed
	RetryCount   int    `gorm:"column:retry_count;default:0" json:"retry_count"`                                                          // 重试次数
	ErrorMessage string `gorm:"column:error_message;type:text" json:"error_message,omitempty"`                                            // 错误信息

	// 时间戳
	CreatedAt   time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP;index:idx_embedding_tasks_created" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updated_at"`
	CompletedAt *time.Time `gorm:"column:completed_at" json:"completed_at,omitempty"`
}

func (EmbeddingTask) TableName() string {
	return "embedding_tasks"
}

// EmbeddingTaskStatus 任务状态常量
const (
	EmbeddingTaskStatusPending    = "pending"    // 待处理
	EmbeddingTaskStatusProcessing = "processing" // 处理中
	EmbeddingTaskStatusCompleted  = "completed"  // 已完成
	EmbeddingTaskStatusFailed     = "failed"     // 失败
)

// EmbeddingTaskConfig 任务配置常量
const (
	EmbeddingTaskMaxRetries    = 3   // 最大重试次数
	EmbeddingTaskExpireDays    = 3   // 任务过期天数
	EmbeddingTaskBatchSize     = 100 // 每批处理数量
	EmbeddingTaskBatchInterval = 2   // 批次间隔（秒）
)

// EmbeddingStatus Embedding 状态统计
type EmbeddingStatus struct {
	WorkspaceID     string  `json:"workspace_id"`
	TotalResources  int64   `json:"total_resources"`
	WithEmbedding   int64   `json:"with_embedding"`
	PendingTasks    int64   `json:"pending_tasks"`
	ProcessingTasks int64   `json:"processing_tasks"`
	FailedTasks     int64   `json:"failed_tasks"`
	Progress        float64 `json:"progress"`       // 0-100
	EstimatedTime   string  `json:"estimated_time"` // 预估剩余时间
}

// EmbeddingWorkerStatus Worker 状态
type EmbeddingWorkerStatus struct {
	Running         bool  `json:"running"`
	PendingTasks    int64 `json:"pending_tasks"`
	ProcessingTasks int64 `json:"processing_tasks"`
	CompletedTasks  int64 `json:"completed_tasks"`
	FailedTasks     int64 `json:"failed_tasks"`
	ExpiredTasks    int64 `json:"expired_tasks"`
	ExpireDays      int   `json:"expire_days"`
}

// EmbeddingConfigStatus Embedding 配置状态
type EmbeddingConfigStatus struct {
	Configured  bool   `json:"configured"`
	HasAPIKey   bool   `json:"has_api_key"`
	ModelID     string `json:"model_id,omitempty"`
	ServiceType string `json:"service_type,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	Message     string `json:"message"`
	Help        string `json:"help,omitempty"`
}
