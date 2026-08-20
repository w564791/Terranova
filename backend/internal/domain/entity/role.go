package entity

import (
	"time"
)

// Role IAM角色
// OrgID=0 表示平台级（系统预置 Role）；自定义 Role 必须绑定租户 org_id。
type Role struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	OrgID       uint      `gorm:"not null;default:0;uniqueIndex:idx_role_org_name" json:"org_id"`
	Name        string    `gorm:"type:varchar(100);not null;uniqueIndex:idx_role_org_name" json:"name"`
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
