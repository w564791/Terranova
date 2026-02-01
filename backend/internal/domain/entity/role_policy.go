package entity

import (
	"time"
)

// RolePolicy 角色权限策略
type RolePolicy struct {
	ID              uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	RoleID          uint      `gorm:"not null;index:idx_role_policies_role_id" json:"role_id"`
	PermissionID    string    `gorm:"type:varchar(32);not null;index:idx_role_policies_permission_id" json:"permission_id"` // 业务语义ID
	PermissionLevel string    `gorm:"type:varchar(20);not null" json:"permission_level"`
	ScopeType       string    `gorm:"type:varchar(20);not null" json:"scope_type"`
	CreatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// 关联字段（不存储在数据库）
	PermissionName        string `gorm:"-" json:"permission_name,omitempty"`
	PermissionDisplayName string `gorm:"-" json:"permission_display_name,omitempty"`
	ResourceType          string `gorm:"-" json:"resource_type,omitempty"`
}

// TableName 指定表名
func (RolePolicy) TableName() string {
	return "iam_role_policies"
}
