package models

import (
	"time"

	"gorm.io/gorm"
)

type SystemConfig struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	Key         string         `json:"key" gorm:"uniqueIndex;not null"`
	Value       string         `json:"value" gorm:"type:jsonb;not null"`
	Description string         `json:"description"`
	UpdatedBy   *uint          `json:"updated_by"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

type AuditLog struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	UserID       *string        `json:"user_id" gorm:"type:varchar(20)"` // 语义化用户 ID (e.g. user-xxxx);DB 早就是 varchar(20)
	Action       string         `json:"action" gorm:"not null"`
	ResourceType string         `json:"resource_type" gorm:"not null"`
	ResourceID   *uint          `json:"resource_id"`
	OldValues    string         `json:"old_values" gorm:"type:jsonb"`
	NewValues    string         `json:"new_values" gorm:"type:jsonb"`
	IPAddress    string         `json:"ip_address"`
	UserAgent    string         `json:"user_agent"`
	CreatedAt    time.Time      `json:"created_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}