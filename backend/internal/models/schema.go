package models

import (
	"time"
)

// Schema 表示模块的Schema定义
type Schema struct {
	ID                    uint      `json:"id" gorm:"primaryKey"`
	ModuleID              uint      `json:"module_id"`
	ModuleVersionID       *string   `json:"module_version_id,omitempty" gorm:"type:varchar(30);index:idx_schemas_module_version"` // 关联到 ModuleVersion
	Version               string    `json:"version"`
	Status                string    `json:"status"` // active, draft, deprecated
	SchemaData            string    `json:"schema_data" gorm:"type:jsonb"`
	AIGenerated           bool      `json:"ai_generated"`
	SourceType            string    `json:"source_type"`    // json_import, tf_parse, ai_generate, openapi_import
	SchemaVersion         string    `json:"schema_version"` // v1, v2
	OpenAPISchema         JSONB     `json:"openapi_schema" gorm:"column:openapi_schema;type:jsonb"`
	VariablesTF           string    `json:"variables_tf" gorm:"column:variables_tf"`
	UIConfig              JSONB     `json:"ui_config" gorm:"column:ui_config;type:jsonb"`
	InheritedFromSchemaID *uint     `json:"inherited_from_schema_id,omitempty"` // 继承自哪个 Schema（用于追溯）
	CreatedBy             *string   `json:"created_by" gorm:"type:varchar(20)"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`

	// 关联（非数据库字段）
	ModuleVersion *ModuleVersion `json:"module_version,omitempty" gorm:"foreignKey:ModuleVersionID"`
}

// CreateSchemaRequest 创建Schema的请求结构
type CreateSchemaRequest struct {
	Version    string      `json:"version" binding:"required"`
	SchemaData interface{} `json:"schema_data" binding:"required"`
	Status     string      `json:"status"`
	SourceType string      `json:"source_type"` // json_import, tf_parse, ai_generate
}

// CreateSchemaV2Request 创建V2 Schema的请求结构
type CreateSchemaV2Request struct {
	Version       string      `json:"version" binding:"required"`
	OpenAPISchema interface{} `json:"openapi_schema" binding:"required"`
	VariablesTF   string      `json:"variables_tf"`
	Status        string      `json:"status"`
	SourceType    string      `json:"source_type"` // tf_parse, openapi_import
}

// UpdateSchemaRequest 更新Schema的请求结构
type UpdateSchemaRequest struct {
	SchemaData interface{} `json:"schema_data,omitempty"`
	Status     string      `json:"status,omitempty"`
}

// UpdateSchemaV2Request 更新V2 Schema的请求结构
type UpdateSchemaV2Request struct {
	OpenAPISchema interface{} `json:"openapi_schema,omitempty"`
	UIConfig      interface{} `json:"ui_config,omitempty"`
	VariablesTF   string      `json:"variables_tf,omitempty"`
	Status        string      `json:"status,omitempty"`
}

// ParseTFRequest 解析 Terraform variables.tf 的请求结构
type ParseTFRequest struct {
	VariablesTF string `json:"variables_tf"` // 移除 required，允许只解析 outputs
	OutputsTF   string `json:"outputs_tf"`   // 新增 outputs.tf 支持
	ModuleName  string `json:"module_name"`
	Provider    string `json:"provider"`
	Version     string `json:"version"`
	Layout      string `json:"layout"` // top, left
}

// ParseTFResponse 解析 Terraform variables.tf 的响应结构
type ParseTFResponse struct {
	OpenAPISchema  interface{} `json:"openapi_schema"`
	FieldCount     int         `json:"field_count"`
	BasicFields    int         `json:"basic_fields"`
	AdvancedFields int         `json:"advanced_fields"`
	Warnings       []string    `json:"warnings,omitempty"`
}

// SchemaFieldUpdate 单个字段更新请求
type SchemaFieldUpdate struct {
	FieldName string      `json:"field_name" binding:"required"`
	Property  string      `json:"property" binding:"required"` // label, group, widget, help, etc.
	Value     interface{} `json:"value"`
}

// ValidateModuleInputRequest 验证模块输入的请求结构
type ValidateModuleInputRequest struct {
	Values map[string]interface{} `json:"values" binding:"required"` // 输入值
	Mode   string                 `json:"mode"`                      // create, update
}

// ValidateModuleInputResponse 验证模块输入的响应结构
type ValidateModuleInputResponse struct {
	Valid  bool              `json:"valid"`  // 是否验证通过
	Errors []ValidationError `json:"errors"` // 验证错误列表
}

// ValidationError 验证错误
type ValidationError struct {
	Field   string `json:"field"`   // 字段名，跨字段验证时为空
	Type    string `json:"type"`    // 错误类型：required, type, pattern, minimum, maximum, minLength, maxLength, enum, conflicts, requiredWith, exactlyOneOf, atLeastOneOf
	Message string `json:"message"` // 错误消息
}

// SchemaDiffResponse Schema 对比响应
type SchemaDiffResponse struct {
	OldVersion string                   `json:"old_version"`
	NewVersion string                   `json:"new_version"`
	OldID      uint                     `json:"old_id"`
	NewID      uint                     `json:"new_id"`
	OldData    interface{}              `json:"old_data"`
	NewData    interface{}              `json:"new_data"`
	Diffs      []map[string]interface{} `json:"diffs"`
	Stats      DiffStats                `json:"stats"`
}

// DiffStats 差异统计
type DiffStats struct {
	Total    int `json:"total"`
	Added    int `json:"added"`
	Removed  int `json:"removed"`
	Modified int `json:"modified"`
}
