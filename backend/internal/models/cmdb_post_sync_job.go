package models

import "time"

const (
	PostSyncJobTypeSummary             = "summary"
	PostSyncJobTypeEmbedding           = "embedding"
	PostSyncJobTypeSummaryAssessment   = "summary_assessment"
)

const (
	PostSyncJobStatusPending    = "pending"
	PostSyncJobStatusProcessing = "processing"
	PostSyncJobStatusCompleted  = "completed"
	PostSyncJobStatusFailed     = "failed"
)

// PostSyncJob CMDB 同步后处理任务队列
type PostSyncJob struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	SourceID     string     `gorm:"column:source_id;type:varchar(50);not null;index:idx_post_sync_jobs_source" json:"source_id"`
	JobType      string     `gorm:"column:job_type;type:varchar(20);not null" json:"job_type"`
	Status       string     `gorm:"column:status;type:varchar(20);not null;default:'pending';index:idx_post_sync_jobs_status" json:"status"`
	DependsOn    *uint      `gorm:"column:depends_on" json:"depends_on"`
	ErrorMessage string     `gorm:"column:error_message;type:text" json:"error_message"`
	RetryCount   int        `gorm:"column:retry_count;default:0" json:"retry_count"`
	CreatedAt    time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
	StartedAt    *time.Time `gorm:"column:started_at" json:"started_at"`
	CompletedAt  *time.Time `gorm:"column:completed_at" json:"completed_at"`
}

func (PostSyncJob) TableName() string { return "cmdb_post_sync_jobs" }

const (
	PostSyncJobMaxRetries = 3
	PostSyncJobExpireDays = 3
)
