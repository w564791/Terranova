package models

import "time"

// ManifestAISession manifest 编辑器 AI 助手的会话(按 manifest + 用户隔离)。
// 一个用户在一个 manifest 下可有多条会话,可新建/切换/查看历史。
type ManifestAISession struct {
	ID         string    `json:"id" gorm:"primaryKey;size:64"` // 格式: mas-{uuid}(前缀+uuid 约 40 字符)
	ManifestID string    `json:"manifest_id" gorm:"size:36;not null;index:idx_mas_lookup,priority:1"`
	OrgID      string    `json:"org_id" gorm:"size:50;not null"`                                  // 组织(字符串,兼容多种 org id 形态)
	UserID     string    `json:"user_id" gorm:"size:64;not null;index:idx_mas_lookup,priority:2"` // 所属用户,隔离边界
	Title      string    `json:"title" gorm:"size:255"`                                           // 会话标题(首条消息摘要,可改)
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime;index:idx_mas_lookup,priority:3,sort:desc"`
}

func (ManifestAISession) TableName() string { return "manifest_ai_sessions" }

// ManifestAIMessage 会话内的单条消息。生成/检查的用户输入与 AI 产出都入会话。
type ManifestAIMessage struct {
	ID        string    `json:"id" gorm:"primaryKey;size:64"` // 格式: mam-{uuid}
	SessionID string    `json:"session_id" gorm:"size:64;not null;index:idx_mam_session,priority:1"`
	Role      string    `json:"role" gorm:"size:16;not null"`       // user | assistant
	Kind      string    `json:"kind" gorm:"size:16;not null"`       // generate | check
	Content   string    `json:"content" gorm:"type:jsonb;not null"` // JSON:生成 {description,hcl};检查 {trigger,issues}
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime;index:idx_mam_session,priority:2"`
}

func (ManifestAIMessage) TableName() string { return "manifest_ai_messages" }
