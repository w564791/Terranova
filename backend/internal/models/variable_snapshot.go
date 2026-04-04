package models

import "time"

// VariableSnapshot 变量快照记录（同一 vsnap_id 多行，每行一个变量引用）
type VariableSnapshot struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	VsnapID      string    `json:"vsnap_id" gorm:"column:vsnap_id;type:varchar(30);not null;index"`
	WorkspaceID  string    `json:"workspace_id" gorm:"type:varchar(50);not null;index"`
	VariableID   string    `json:"variable_id" gorm:"type:varchar(20);not null"`
	Version      int       `json:"version" gorm:"not null"`
	VariableType string    `json:"variable_type" gorm:"type:varchar(20);not null"`
	SourceType   string    `json:"source_type" gorm:"type:varchar(20);not null"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    *string   `json:"created_by" gorm:"type:varchar(20)"`
}

func (VariableSnapshot) TableName() string {
	return "variable_snapshots"
}
