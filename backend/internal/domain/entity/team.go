package entity

import (
	"time"

	"iac-platform/internal/infrastructure"

	"gorm.io/gorm"
)

// Team 团队实体
type Team struct {
	ID          string    `json:"id" gorm:"column:team_id;primaryKey;type:varchar(20)"`
	OrgID       uint      `json:"org_id"`       // 所属组织ID
	Name        string    `json:"name"`         // 团队名称
	DisplayName string    `json:"display_name"` // 显示名称
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`  // 是否为系统预置团队（不可删除）
	CreatedBy   *string   `json:"created_by"` // 创建人user_id
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// 关联
	Organization *Organization `json:"organization,omitempty" gorm:"foreignKey:OrgID"`
	Members      []TeamMember  `json:"members,omitempty" gorm:"foreignKey:TeamID"`
}

// TableName 指定表名
func (Team) TableName() string {
	return "teams"
}

// BeforeCreate GORM hook - 创建前生成ID
func (t *Team) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		id, err := infrastructure.GenerateTeamID()
		if err != nil {
			return err
		}
		t.ID = id
	}
	return nil
}

// IsValid 验证团队数据是否有效
func (t *Team) IsValid() bool {
	return t.OrgID > 0 && t.Name != "" && len(t.Name) <= 100
}

// TeamRole 团队内角色
type TeamRole string

const (
	// TeamRoleMember 普通成员
	TeamRoleMember TeamRole = "MEMBER"
	// TeamRoleMaintainer 维护者（可以管理团队成员）
	TeamRoleMaintainer TeamRole = "MAINTAINER"
)

// String 返回角色的字符串表示
func (r TeamRole) String() string {
	return string(r)
}

// IsValid 验证角色是否有效
func (r TeamRole) IsValid() bool {
	return r == TeamRoleMember || r == TeamRoleMaintainer
}

// TeamMember 团队成员
type TeamMember struct {
	ID       uint      `json:"id"`
	TeamID   string    `json:"team_id" gorm:"type:varchar(20);not null"`
	UserID   string    `json:"user_id" gorm:"type:varchar(20);not null"`
	Role     TeamRole  `json:"role"`      // 团队内角色
	JoinedAt time.Time `json:"joined_at"` // 加入时间
	JoinedBy *string   `json:"joined_by"` // 添加人user_id

	// 关联
	Team *Team `json:"team,omitempty" gorm:"foreignKey:TeamID"`
}

// TableName 指定表名
func (TeamMember) TableName() string {
	return "team_members"
}

// IsValid 验证团队成员数据是否有效
func (tm *TeamMember) IsValid() bool {
	return tm.TeamID != "" && tm.UserID != "" && tm.Role.IsValid()
}

// IsMaintainer 判断是否为维护者
func (tm *TeamMember) IsMaintainer() bool {
	return tm.Role == TeamRoleMaintainer
}
