package entity

import (
	"time"
)

// UserRole 用户角色分配
type UserRole struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     string     `gorm:"type:varchar(20);not null;index:idx_user_roles_user_id" json:"user_id"`
	RoleID     uint       `gorm:"not null;index:idx_user_roles_role_id" json:"role_id"`
	ScopeType  string     `gorm:"type:varchar(20);not null;index:idx_user_roles_scope" json:"scope_type"`
	ScopeID    uint       `gorm:"not null;index:idx_user_roles_scope" json:"scope_id"`
	AssignedBy *string    `gorm:"type:varchar(20)" json:"assigned_by,omitempty"`
	AssignedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"assigned_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Reason     string     `gorm:"type:text" json:"reason,omitempty"`

	// 关联字段（不存储在数据库）
	RoleName        string `gorm:"-" json:"role_name,omitempty"`
	RoleDisplayName string `gorm:"-" json:"role_display_name,omitempty"`
}

// TableName 指定表名
func (UserRole) TableName() string {
	return "iam_user_roles"
}

// IsValid 检查角色分配是否有效（未过期）
func (ur *UserRole) IsValid() bool {
	if ur.ExpiresAt == nil {
		return true
	}
	return ur.ExpiresAt.After(time.Now())
}
