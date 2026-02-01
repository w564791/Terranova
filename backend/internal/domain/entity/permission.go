package entity

import (
	"time"

	"iac-platform/internal/domain/valueobject"
)

// PermissionDefinition 权限定义
type PermissionDefinition struct {
	ID           string                   `gorm:"primaryKey;type:varchar(32)" json:"id"` // 业务语义ID: orgpm-xxx, wspm-xxx
	Name         string                   `gorm:"uniqueIndex" json:"name"`               // 权限名称（唯一）
	ResourceType valueobject.ResourceType `json:"resource_type"`                         // 资源类型
	ScopeLevel   valueobject.ScopeType    `json:"scope_level"`                           // 适用作用域层级
	DisplayName  string                   `json:"display_name"`                          // 显示名称
	Description  string                   `json:"description"`                           // 描述
	IsSystem     bool                     `json:"is_system"`                             // 是否为系统内置
	CreatedAt    time.Time                `json:"created_at"`
}

// TableName 指定表名
func (PermissionDefinition) TableName() string {
	return "permission_definitions"
}

// PermissionGrant 权限授予记录（通用结构）
type PermissionGrant struct {
	ID              uint                        `json:"id"`
	ScopeType       valueobject.ScopeType       `json:"scope_type"`                            // 作用域类型
	ScopeID         uint                        `json:"scope_id"`                              // 作用域ID（数字）
	ScopeIDStr      string                      `json:"scope_id_str,omitempty"`                // 作用域ID（语义化，如workspace_id）
	PrincipalType   valueobject.PrincipalType   `json:"principal_type"`                        // 主体类型
	PrincipalID     string                      `gorm:"type:varchar(20)" json:"principal_id"`  // 主体ID
	PermissionID    string                      `gorm:"type:varchar(32)" json:"permission_id"` // 权限定义ID（业务语义ID）
	PermissionLevel valueobject.PermissionLevel `json:"permission_level"`                      // 权限等级
	GrantedBy       *string                     `json:"granted_by"`                            // 授权人user_id
	GrantedAt       time.Time                   `json:"granted_at"`                            // 授权时间
	ExpiresAt       *time.Time                  `json:"expires_at"`                            // 过期时间
	Reason          string                      `json:"reason"`                                // 授权原因
	Source          string                      `json:"source"`                                // 来源（direct/team/inherited）

	// 关联
	Permission *PermissionDefinition `json:"permission,omitempty" gorm:"foreignKey:PermissionID"`
}

// IsExpired 判断权限是否过期
func (p *PermissionGrant) IsExpired() bool {
	if p.ExpiresAt == nil {
		return false
	}
	return p.ExpiresAt.Before(time.Now())
}

// IsValid 判断权限是否有效（未过期）
func (p *PermissionGrant) IsValid() bool {
	return !p.IsExpired()
}

// OrgPermission 组织级权限分配
type OrgPermission struct {
	ID              uint                        `json:"id"`
	OrgID           uint                        `json:"org_id"`                                // 组织ID
	PrincipalType   valueobject.PrincipalType   `json:"principal_type"`                        // 主体类型
	PrincipalID     string                      `gorm:"type:varchar(20)" json:"principal_id"`  // 主体ID
	PermissionID    string                      `gorm:"type:varchar(32)" json:"permission_id"` // 权限定义ID（业务语义ID）
	PermissionLevel valueobject.PermissionLevel `json:"permission_level"`                      // 权限等级
	GrantedBy       *string                     `json:"granted_by"`                            // 授权人user_id
	GrantedAt       time.Time                   `json:"granted_at"`                            // 授权时间
	ExpiresAt       *time.Time                  `json:"expires_at"`                            // 过期时间
	Reason          string                      `json:"reason"`                                // 授权原因

	// 关联
	Organization *Organization         `json:"organization,omitempty" gorm:"foreignKey:OrgID"`
	Permission   *PermissionDefinition `json:"permission,omitempty" gorm:"foreignKey:PermissionID"`
}

// TableName 指定表名
func (OrgPermission) TableName() string {
	return "org_permissions"
}

// ToPermissionGrant 转换为通用权限授予记录
func (op *OrgPermission) ToPermissionGrant() *PermissionGrant {
	return &PermissionGrant{
		ID:              op.ID,
		ScopeType:       valueobject.ScopeTypeOrganization,
		ScopeID:         op.OrgID,
		PrincipalType:   op.PrincipalType,
		PrincipalID:     op.PrincipalID,
		PermissionID:    op.PermissionID,
		PermissionLevel: op.PermissionLevel,
		GrantedBy:       op.GrantedBy,
		GrantedAt:       op.GrantedAt,
		ExpiresAt:       op.ExpiresAt,
		Reason:          op.Reason,
		Source:          "direct",
		Permission:      op.Permission,
	}
}

// ProjectPermission 项目级权限分配
type ProjectPermission struct {
	ID              uint                        `json:"id"`
	ProjectID       uint                        `json:"project_id"`                            // 项目ID
	PrincipalType   valueobject.PrincipalType   `json:"principal_type"`                        // 主体类型
	PrincipalID     string                      `gorm:"type:varchar(20)" json:"principal_id"`  // 主体ID
	PermissionID    string                      `gorm:"type:varchar(32)" json:"permission_id"` // 权限定义ID（业务语义ID）
	PermissionLevel valueobject.PermissionLevel `json:"permission_level"`                      // 权限等级
	GrantedBy       *string                     `json:"granted_by"`                            // 授权人user_id
	GrantedAt       time.Time                   `json:"granted_at"`                            // 授权时间
	ExpiresAt       *time.Time                  `json:"expires_at"`                            // 过期时间
	Reason          string                      `json:"reason"`                                // 授权原因

	// 关联
	Project    *Project              `json:"project,omitempty" gorm:"foreignKey:ProjectID"`
	Permission *PermissionDefinition `json:"permission,omitempty" gorm:"foreignKey:PermissionID"`
}

// TableName 指定表名
func (ProjectPermission) TableName() string {
	return "project_permissions"
}

// ToPermissionGrant 转换为通用权限授予记录
func (pp *ProjectPermission) ToPermissionGrant() *PermissionGrant {
	return &PermissionGrant{
		ID:              pp.ID,
		ScopeType:       valueobject.ScopeTypeProject,
		ScopeID:         pp.ProjectID,
		PrincipalType:   pp.PrincipalType,
		PrincipalID:     pp.PrincipalID,
		PermissionID:    pp.PermissionID,
		PermissionLevel: pp.PermissionLevel,
		GrantedBy:       pp.GrantedBy,
		GrantedAt:       pp.GrantedAt,
		ExpiresAt:       pp.ExpiresAt,
		Reason:          pp.Reason,
		Source:          "direct",
		Permission:      pp.Permission,
	}
}

// WorkspacePermission 工作空间级权限分配
type WorkspacePermission struct {
	ID              uint                        `json:"id"`
	WorkspaceID     string                      `gorm:"type:varchar(50)" json:"workspace_id"`  // 工作空间ID（语义化ID）
	PrincipalType   valueobject.PrincipalType   `json:"principal_type"`                        // 主体类型
	PrincipalID     string                      `gorm:"type:varchar(20)" json:"principal_id"`  // 主体ID
	PermissionID    string                      `gorm:"type:varchar(32)" json:"permission_id"` // 权限定义ID（业务语义ID）
	PermissionLevel valueobject.PermissionLevel `json:"permission_level"`                      // 权限等级
	GrantedBy       *string                     `json:"granted_by"`                            // 授权人user_id
	GrantedAt       time.Time                   `json:"granted_at"`                            // 授权时间
	ExpiresAt       *time.Time                  `json:"expires_at"`                            // 过期时间
	Reason          string                      `json:"reason"`                                // 授权原因

	// 关联
	Permission *PermissionDefinition `json:"permission,omitempty" gorm:"foreignKey:PermissionID"`
}

// TableName 指定表名
func (WorkspacePermission) TableName() string {
	return "workspace_permissions"
}

// ToPermissionGrant 转换为通用权限授予记录
// 注意：需要在调用此方法后，手动设置 ScopeID 为 workspace 的数字 ID
func (wp *WorkspacePermission) ToPermissionGrant() *PermissionGrant {
	return &PermissionGrant{
		ID:              wp.ID,
		ScopeType:       valueobject.ScopeTypeWorkspace,
		ScopeID:         0, // 需要在repository层查询并设置workspace的数字ID
		PrincipalType:   wp.PrincipalType,
		PrincipalID:     wp.PrincipalID,
		PermissionID:    wp.PermissionID,
		PermissionLevel: wp.PermissionLevel,
		GrantedBy:       wp.GrantedBy,
		GrantedAt:       wp.GrantedAt,
		ExpiresAt:       wp.ExpiresAt,
		Reason:          wp.Reason,
		Source:          "direct",
		Permission:      wp.Permission,
	}
}

// PermissionPreset 权限预设
type PermissionPreset struct {
	ID          uint                  `json:"id"`
	Name        string                `json:"name"`         // 预设名称（READ/WRITE/ADMIN）
	ScopeLevel  valueobject.ScopeType `json:"scope_level"`  // 适用层级
	DisplayName string                `json:"display_name"` // 显示名称
	Description string                `json:"description"`  // 描述
	IsSystem    bool                  `json:"is_system"`    // 是否为系统预置
	CreatedAt   time.Time             `json:"created_at"`
}

// TableName 指定表名
func (PermissionPreset) TableName() string {
	return "permission_presets"
}

// PresetPermission 权限预设详情
type PresetPermission struct {
	ID              uint                        `json:"id"`
	PresetID        uint                        `json:"preset_id"`                             // 预设ID
	PermissionID    string                      `gorm:"type:varchar(32)" json:"permission_id"` // 权限ID（业务语义ID）
	PermissionLevel valueobject.PermissionLevel `json:"permission_level"`                      // 权限等级

	// 关联
	Preset     *PermissionPreset     `json:"preset,omitempty" gorm:"foreignKey:PresetID"`
	Permission *PermissionDefinition `json:"permission,omitempty" gorm:"foreignKey:PermissionID"`
}

// TableName 指定表名
func (PresetPermission) TableName() string {
	return "preset_permissions"
}
