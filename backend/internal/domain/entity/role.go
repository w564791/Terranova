package entity

import (
	"time"
)

// Role IAM角色
type Role struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null;uniqueIndex" json:"name"`
	DisplayName string    `gorm:"type:varchar(200);not null" json:"display_name"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	IsSystem    bool      `gorm:"not null;default:false" json:"is_system"`
	IsActive    bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedBy   *string   `json:"created_by,omitempty"`
	CreatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// 关联字段（不存储在数据库）
	Policies []*RolePolicy `gorm:"-" json:"policies,omitempty"`
}

// TableName 指定表名
func (Role) TableName() string {
	return "iam_roles"
}
