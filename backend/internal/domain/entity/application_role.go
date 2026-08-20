package entity

import "time"

// ApplicationRole Application 主体的 Role 赋值（选项 A / D5）
// ApplicationPrincipalID 存 app_key，与 AgentAuth principal_id 一致。
type ApplicationRole struct {
	ID                     uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	ApplicationPrincipalID string     `gorm:"column:application_principal_id;type:varchar(64);not null;index:idx_app_roles_principal" json:"application_principal_id"`
	RoleID                 uint       `gorm:"not null;index:idx_app_roles_role" json:"role_id"`
	ScopeType              string     `gorm:"type:varchar(20);not null;index:idx_app_roles_scope" json:"scope_type"`
	ScopeID                uint       `gorm:"not null;index:idx_app_roles_scope" json:"scope_id"`
	AssignedBy             *string    `gorm:"type:varchar(20)" json:"assigned_by,omitempty"`
	AssignedAt             time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"assigned_at"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	Reason                 string     `gorm:"type:text" json:"reason,omitempty"`

	RoleName        string `gorm:"-" json:"role_name,omitempty"`
	RoleDisplayName string `gorm:"-" json:"role_display_name,omitempty"`
}

func (ApplicationRole) TableName() string { return "iam_application_roles" }

func (ar *ApplicationRole) IsValid() bool {
	if ar.ExpiresAt == nil {
		return true
	}
	return ar.ExpiresAt.After(time.Now())
}
