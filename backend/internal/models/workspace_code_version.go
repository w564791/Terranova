package models

import (
	"time"
)

// WorkspaceCodeVersion 工作空间代码版本
type WorkspaceCodeVersion struct {
	ID             uint                   `gorm:"primaryKey" json:"id"`
	WorkspaceID    uint                   `gorm:"not null;index:idx_workspace_code_versions_workspace" json:"workspace_id"`
	Version        int                    `gorm:"not null" json:"version"`
	TFCode         map[string]interface{} `gorm:"type:jsonb;not null" json:"tf_code"`
	ProviderConfig map[string]interface{} `gorm:"type:jsonb;not null" json:"provider_config"`
	StateVersionID *uint                  `json:"state_version_id,omitempty"`
	ChangeSummary  string                 `gorm:"type:text" json:"change_summary"`
	CreatedBy     *string `gorm:"type:varchar(20)" json:"created_by,omitempty"`
	CreatedAt      time.Time              `gorm:"autoCreateTime" json:"created_at"`

	// 关联
	Workspace    Workspace              `gorm:"foreignKey:WorkspaceID" json:"-"`
	StateVersion *WorkspaceStateVersion `gorm:"foreignKey:StateVersionID" json:"state_version,omitempty"`
	Creator      *User                  `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

// TableName 指定表名
func (WorkspaceCodeVersion) TableName() string {
	return "workspace_code_versions"
}
