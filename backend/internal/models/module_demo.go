package models

import (
	"time"
)

// ModuleDemo 模块演示配置
type ModuleDemo struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	ModuleID            uint      `gorm:"not null;index:idx_module_demos_module" json:"module_id"`
	ModuleVersionID     *string   `gorm:"type:varchar(30);index:idx_module_demos_version" json:"module_version_id,omitempty"` // 关联到 ModuleVersion
	Name                string    `gorm:"type:varchar(200);not null" json:"name"`
	Description         string    `gorm:"type:text" json:"description"`
	CurrentVersionID    *uint     `json:"current_version_id,omitempty"`
	IsActive            bool      `gorm:"default:true;index:idx_module_demos_active" json:"is_active"`
	UsageNotes          string    `gorm:"type:text" json:"usage_notes"`
	InheritedFromDemoID *uint     `json:"inherited_from_demo_id,omitempty"` // 继承自哪个 Demo（用于追溯）
	CreatedBy           *string   `json:"created_by,omitempty" gorm:"type:varchar(20)"`
	CreatedAt           time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt           time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// 关联
	Module         Module             `gorm:"foreignKey:ModuleID" json:"-"`
	ModuleVersion  *ModuleVersion     `gorm:"foreignKey:ModuleVersionID" json:"module_version,omitempty"`
	CurrentVersion *ModuleDemoVersion `gorm:"foreignKey:CurrentVersionID;references:ID" json:"current_version,omitempty"`
	Creator        *User              `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

// TableName 指定表名
func (ModuleDemo) TableName() string {
	return "module_demos"
}

// ModuleDemoVersion 模块演示版本
type ModuleDemoVersion struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	DemoID           uint      `gorm:"not null;index:idx_module_demo_versions_demo" json:"demo_id"`
	Version          int       `gorm:"not null" json:"version"`
	IsLatest         bool      `gorm:"default:false;index:idx_module_demo_versions_latest" json:"is_latest"`
	ConfigData       JSONB     `gorm:"type:jsonb;not null" json:"config_data"`
	ChangeSummary    string    `gorm:"type:text" json:"change_summary"`
	ChangeType       string    `gorm:"type:varchar(20)" json:"change_type"` // create, update, rollback
	DiffFromPrevious string    `gorm:"type:text" json:"diff_from_previous"`
	CreatedBy        *string   `json:"created_by,omitempty" gorm:"type:varchar(20)"`
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`

	// 关联
	Demo    ModuleDemo `gorm:"foreignKey:DemoID" json:"-"`
	Creator *User      `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

// TableName 指定表名
func (ModuleDemoVersion) TableName() string {
	return "module_demo_versions"
}
