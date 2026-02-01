package entity

import (
	"time"
)

// Organization 组织实体（租户边界）
type Organization struct {
	ID          uint                   `json:"id"`
	Name        string                 `json:"name"`         // 唯一标识名称
	DisplayName string                 `json:"display_name"` // 显示名称
	Description string                 `json:"description"`
	IsActive    bool                   `json:"is_active"`
	Settings    map[string]interface{} `json:"settings" gorm:"type:jsonb;serializer:json"` // 组织配置
	CreatedBy   *string                `json:"created_by"`                                 // user_id
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// TableName 指定表名
func (Organization) TableName() string {
	return "organizations"
}

// IsValid 验证组织数据是否有效
func (o *Organization) IsValid() bool {
	return o.Name != "" && len(o.Name) <= 100
}

// Project 项目实体
type Project struct {
	ID          uint                   `json:"id"`
	OrgID       uint                   `json:"org_id"`       // 所属组织ID
	Name        string                 `json:"name"`         // 项目名称
	DisplayName string                 `json:"display_name"` // 显示名称
	Description string                 `json:"description"`
	IsDefault   bool                   `json:"is_default"` // 是否为默认项目
	IsActive    bool                   `json:"is_active"`
	Settings    map[string]interface{} `json:"settings" gorm:"type:jsonb;serializer:json"` // 项目配置
	CreatedBy   *string                `json:"created_by"`                                 // user_id
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`

	// 关联
	Organization *Organization `json:"organization,omitempty" gorm:"foreignKey:OrgID"`
}

// TableName 指定表名
func (Project) TableName() string {
	return "projects"
}

// IsValid 验证项目数据是否有效
func (p *Project) IsValid() bool {
	return p.OrgID > 0 && p.Name != "" && len(p.Name) <= 100
}

// WorkspaceProjectRelation 工作空间-项目关联
type WorkspaceProjectRelation struct {
	ID          uint      `json:"id"`
	WorkspaceID string    `json:"workspace_id" gorm:"type:varchar(50);not null"` // 语义化ID，如 ws-xxx
	ProjectID   uint      `json:"project_id"`
	CreatedAt   time.Time `json:"created_at"`

	// 关联
	Project *Project `json:"project,omitempty" gorm:"foreignKey:ProjectID"`
}

// TableName 指定表名
func (WorkspaceProjectRelation) TableName() string {
	return "workspace_project_relations"
}

// UserOrganization 用户-组织关系
type UserOrganization struct {
	ID       uint      `json:"id"`
	UserID   string    `json:"user_id" gorm:"type:varchar(20);not null"`
	OrgID    uint      `json:"org_id"`
	JoinedAt time.Time `json:"joined_at"`

	// 关联
	Organization *Organization `json:"organization,omitempty" gorm:"foreignKey:OrgID"`
}

// TableName 指定表名
func (UserOrganization) TableName() string {
	return "user_organizations"
}
