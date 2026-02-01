package models

import (
	"time"
)

// AIPrompt AI 助手提示词结构
type AIPrompt struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Prompt    string `json:"prompt"`
	CreatedAt string `json:"created_at"`
}

type Module struct {
	ID               uint        `json:"id" gorm:"primaryKey"`
	Name             string      `json:"name" gorm:"not null"`
	Provider         string      `json:"provider" gorm:"not null"`
	Source           string      `json:"source" gorm:"not null"`
	ModuleSource     string      `json:"module_source" gorm:"column:module_source"`
	Version          string      `json:"version" gorm:"not null"`
	Description      string      `json:"description"`
	Status           string      `json:"status" gorm:"default:active"`
	DefaultVersionID *string     `json:"default_version_id,omitempty" gorm:"type:varchar(30)"` // 指向默认的 ModuleVersion
	VCSProviderID    *uint       `json:"vcs_provider_id"`
	RepositoryURL    string      `json:"repository_url"`
	Branch           string      `json:"branch" gorm:"default:main"`
	Path             string      `json:"path" gorm:"default:/"`
	ModuleFiles      interface{} `json:"module_files" gorm:"type:jsonb"`
	AIPrompts        []AIPrompt  `json:"ai_prompts" gorm:"column:ai_prompts;type:jsonb;serializer:json;default:'[]'"` // AI 助手提示词列表
	SyncStatus       string      `json:"sync_status" gorm:"default:pending"`
	LastSyncAt       *time.Time  `json:"last_sync_at"`
	CreatedBy        *string     `gorm:"type:varchar(20)" json:"created_by"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`

	// 关联（非数据库字段）
	DefaultVersion *ModuleVersion `json:"default_version,omitempty" gorm:"foreignKey:DefaultVersionID"`
}
