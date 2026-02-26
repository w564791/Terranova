package models

import (
	"time"
)

// ModuleVersion 模块版本（对应 Terraform Module 的不同版本）
type ModuleVersion struct {
	ID                     string    `json:"id" gorm:"primaryKey;type:varchar(30)"` // modv-xxx 语义化 ID
	ModuleID               uint      `json:"module_id" gorm:"not null;index:idx_module_versions_module"`
	Version                string    `json:"version" gorm:"type:varchar(50);not null"` // Terraform Module 版本 (如 6.1.5)
	Source                 string    `json:"source" gorm:"type:varchar(500)"`          // Module source (可覆盖)
	ModuleSource           string    `json:"module_source" gorm:"type:varchar(500)"`   // 完整 source URL
	IsDefault              bool      `json:"is_default" gorm:"default:false;index:idx_module_versions_default"`
	Status                 string    `json:"status" gorm:"type:varchar(20);default:active"`                             // active, deprecated, archived
	// Deprecated: 不再用于 Schema 解析。系统自动使用最新 Schema (created_at DESC)。
	// 保留字段以避免迁移，但不应再读写此字段。
	ActiveSchemaID *uint `json:"active_schema_id,omitempty" gorm:"index:idx_module_versions_active_schema"`
	InheritedFromVersionID *string   `json:"inherited_from_version_id,omitempty" gorm:"type:varchar(30)"`
	CreatedBy              *string   `json:"created_by,omitempty" gorm:"type:varchar(20)"`
	CreatedAt              time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt              time.Time `json:"updated_at" gorm:"autoUpdateTime"`

	// 关联
	Module       *Module `json:"module,omitempty" gorm:"foreignKey:ModuleID"`
	ActiveSchema *Schema `json:"active_schema,omitempty" gorm:"foreignKey:ActiveSchemaID"`

	// 非数据库字段（用于 API 响应）
	SchemaCount         int    `json:"schema_count,omitempty" gorm:"-"`
	ActiveSchemaVersion string `json:"active_schema_version,omitempty" gorm:"-"`
	DemoCount           int    `json:"demo_count,omitempty" gorm:"-"`
	CreatedByName       string `json:"created_by_name,omitempty" gorm:"-"`
}

// TableName 指定表名
func (ModuleVersion) TableName() string {
	return "module_versions"
}

// ModuleVersionStatus 版本状态常量
const (
	ModuleVersionStatusActive     = "active"
	ModuleVersionStatusDeprecated = "deprecated"
	ModuleVersionStatusArchived   = "archived"
)
