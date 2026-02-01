package models

import (
	"time"
)

// ModuleVersionSkill Module 版本关联的 AI Skill
type ModuleVersionSkill struct {
	ID              string `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ModuleVersionID string `gorm:"type:varchar(36);not null;uniqueIndex" json:"module_version_id"`

	// Schema 生成的 Skill（AI 自动生成）
	SchemaGeneratedContent string     `gorm:"type:text" json:"schema_generated_content,omitempty"`
	SchemaGeneratedAt      *time.Time `json:"schema_generated_at,omitempty"`
	SchemaVersionUsed      *int       `gorm:"type:integer" json:"schema_version_used,omitempty"`

	// 用户自定义的 Skill
	CustomContent string `gorm:"type:text" json:"custom_content,omitempty"`

	// 继承信息
	InheritedFromVersionID *string `gorm:"type:varchar(36)" json:"inherited_from_version_id,omitempty"`

	// 元数据
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedBy string    `gorm:"type:varchar(20)" json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// 关联（非数据库字段）
	ModuleVersion        *ModuleVersion `gorm:"foreignKey:ModuleVersionID" json:"module_version,omitempty"`
	InheritedFromVersion *ModuleVersion `gorm:"foreignKey:InheritedFromVersionID" json:"inherited_from_version,omitempty"`
}

// TableName 指定表名
func (ModuleVersionSkill) TableName() string {
	return "module_version_skills"
}

// GetCombinedContent 获取组合后的完整 Skill 内容
// 组合逻辑：Schema 生成的内容 + 用户自定义内容
func (s *ModuleVersionSkill) GetCombinedContent() string {
	combined := ""

	if s.SchemaGeneratedContent != "" {
		combined += s.SchemaGeneratedContent
	}

	if s.CustomContent != "" {
		if combined != "" {
			combined += "\n\n---\n\n## 用户自定义补充\n\n"
		}
		combined += s.CustomContent
	}

	return combined
}

// HasSchemaGenerated 是否已生成 Schema Skill
func (s *ModuleVersionSkill) HasSchemaGenerated() bool {
	return s.SchemaGeneratedContent != "" && s.SchemaGeneratedAt != nil
}

// HasCustomContent 是否有用户自定义内容
func (s *ModuleVersionSkill) HasCustomContent() bool {
	return s.CustomContent != ""
}

// IsInherited 是否是继承的
func (s *ModuleVersionSkill) IsInherited() bool {
	return s.InheritedFromVersionID != nil && *s.InheritedFromVersionID != ""
}

// ========== 请求/响应结构 ==========

// ModuleVersionSkillResponse 响应结构
type ModuleVersionSkillResponse struct {
	ID                     string     `json:"id"`
	ModuleVersionID        string     `json:"module_version_id"`
	SchemaGeneratedContent string     `json:"schema_generated_content,omitempty"`
	SchemaGeneratedAt      *time.Time `json:"schema_generated_at,omitempty"`
	SchemaVersionUsed      *int       `json:"schema_version_used,omitempty"`
	CustomContent          string     `json:"custom_content,omitempty"`
	InheritedFromVersionID *string    `json:"inherited_from_version_id,omitempty"`
	CombinedContent        string     `json:"combined_content"`
	IsActive               bool       `json:"is_active"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// ToResponse 转换为响应结构
func (s *ModuleVersionSkill) ToResponse() *ModuleVersionSkillResponse {
	return &ModuleVersionSkillResponse{
		ID:                     s.ID,
		ModuleVersionID:        s.ModuleVersionID,
		SchemaGeneratedContent: s.SchemaGeneratedContent,
		SchemaGeneratedAt:      s.SchemaGeneratedAt,
		SchemaVersionUsed:      s.SchemaVersionUsed,
		CustomContent:          s.CustomContent,
		InheritedFromVersionID: s.InheritedFromVersionID,
		CombinedContent:        s.GetCombinedContent(),
		IsActive:               s.IsActive,
		CreatedAt:              s.CreatedAt,
		UpdatedAt:              s.UpdatedAt,
	}
}

// UpdateCustomContentRequest 更新自定义内容请求
type UpdateCustomContentRequest struct {
	CustomContent string `json:"custom_content"`
}

// InheritSkillRequest 继承 Skill 请求
type InheritSkillRequest struct {
	FromVersionID string `json:"from_version_id" binding:"required"`
}
