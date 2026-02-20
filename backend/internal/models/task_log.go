package models

import (
	"time"
)

// TaskLog 任务执行日志
type TaskLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"not null;index:idx_task_logs_task_id" json:"task_id"`
	Phase     string    `gorm:"type:varchar(20);not null" json:"phase"` // init, plan, apply
	Content   string    `gorm:"type:text" json:"content"`
	Level     string    `gorm:"type:varchar(10);not null" json:"level"` // info, error, warning
	CreatedAt time.Time `gorm:"autoCreateTime;index:idx_task_logs_created_at" json:"created_at"`

	// 关联
	Task WorkspaceTask `gorm:"foreignKey:TaskID" json:"-"`
}

// TableName 指定表名
func (TaskLog) TableName() string {
	return "task_logs"
}
