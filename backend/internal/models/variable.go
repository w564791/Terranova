package models

import (
	"fmt"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/infrastructure"

	"gorm.io/gorm"
)

// VariableType 变量类型枚举
type VariableType string

const (
	VariableTypeTerraform   VariableType = "terraform"   // Terraform变量
	VariableTypeEnvironment VariableType = "environment" // 环境变量
)

// ValueFormat 值格式枚举
type ValueFormat string

const (
	ValueFormatString ValueFormat = "string" // 字符串格式
	ValueFormatHCL    ValueFormat = "hcl"    // HCL表达式格式
)

// WorkspaceVariable Workspace变量模型
type WorkspaceVariable struct {
	ID           uint         `json:"id" gorm:"primaryKey"`
	VariableID   string       `json:"variable_id" gorm:"type:varchar(20);not null;uniqueIndex"` // 变量语义化ID
	WorkspaceID  string       `json:"workspace_id" gorm:"type:varchar(50);not null;index"`
	Key          string       `json:"key" gorm:"not null;size:100"`
	Version      int          `json:"version" gorm:"not null;default:1"` // 版本号
	Value        string       `json:"value,omitempty" gorm:"type:text"`  // 敏感变量在响应时会被隐藏
	VariableType VariableType `json:"variable_type" gorm:"not null;default:terraform;size:20"`
	ValueFormat  ValueFormat  `json:"value_format" gorm:"not null;default:string;size:20"`
	Sensitive    bool         `json:"sensitive" gorm:"default:false"`
	Description  string       `json:"description" gorm:"type:text"`
	IsDeleted    bool         `json:"is_deleted" gorm:"default:false"` // 软删除标记
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	CreatedBy    *string      `gorm:"type:varchar(20)" json:"created_by"`

	// 关联
	Workspace *Workspace `json:"workspace,omitempty" gorm:"foreignKey:WorkspaceID"`
}

// TableName 指定表名
func (WorkspaceVariable) TableName() string {
	return "workspace_variables"
}

// BeforeCreate 创建前生成 variable_id 并加密敏感变量
func (v *WorkspaceVariable) BeforeCreate(tx *gorm.DB) error {
	// 只在 variable_id 为空时生成（创建新变量）
	// 如果 variable_id 已存在，说明是创建新版本，不应该重新生成
	if v.VariableID == "" {
		varID, err := infrastructure.GenerateVariableID()
		if err != nil {
			return fmt.Errorf("failed to generate variable_id: %w", err)
		}
		v.VariableID = varID
	}
	
	// 加密敏感变量
	if v.Sensitive && v.Value != "" && !crypto.IsEncrypted(v.Value) {
		encrypted, err := crypto.EncryptValue(v.Value)
		if err != nil {
			return fmt.Errorf("failed to encrypt variable: %w", err)
		}
		v.Value = encrypted
	}
	return nil
}

// BeforeSave 保存前加密敏感变量
func (v *WorkspaceVariable) BeforeSave(tx *gorm.DB) error {
	if v.Sensitive && v.Value != "" && !crypto.IsEncrypted(v.Value) {
		encrypted, err := crypto.EncryptValue(v.Value)
		if err != nil {
			return fmt.Errorf("failed to encrypt variable: %w", err)
		}
		v.Value = encrypted
	}
	return nil
}

// AfterFind 查询后解密敏感变量
func (v *WorkspaceVariable) AfterFind(tx *gorm.DB) error {
	if v.Sensitive && v.Value != "" && crypto.IsEncrypted(v.Value) {
		decrypted, err := crypto.DecryptValue(v.Value)
		if err != nil {
			return fmt.Errorf("failed to decrypt variable: %w", err)
		}
		v.Value = decrypted
	}
	return nil
}

// WorkspaceVariableResponse 变量响应（用于API返回，敏感变量值会被隐藏）
type WorkspaceVariableResponse struct {
	ID           uint         `json:"id"`
	VariableID   string       `json:"variable_id"`     // 变量语义化ID
	WorkspaceID  string       `json:"workspace_id"`
	Key          string       `json:"key"`
	Version      int          `json:"version"`         // 版本号
	Value        string       `json:"value,omitempty"` // 敏感变量时为空
	VariableType VariableType `json:"variable_type"`
	ValueFormat  ValueFormat  `json:"value_format"`
	Sensitive    bool         `json:"sensitive"`
	Description  string       `json:"description"`
	// IsDeleted 字段不返回给客户端（内部实现细节）
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	CreatedBy    *string      `gorm:"type:varchar(20)" json:"created_by"`
}

// ToResponse 转换为响应格式（处理敏感变量）
func (v *WorkspaceVariable) ToResponse() *WorkspaceVariableResponse {
	resp := &WorkspaceVariableResponse{
		ID:           v.ID,
		VariableID:   v.VariableID,
		WorkspaceID:  v.WorkspaceID,
		Key:          v.Key,
		Version:      v.Version,
		VariableType: v.VariableType,
		ValueFormat:  v.ValueFormat,
		Sensitive:    v.Sensitive,
		Description:  v.Description,
		// IsDeleted 不返回给客户端
		CreatedAt:    v.CreatedAt,
		UpdatedAt:    v.UpdatedAt,
		CreatedBy:    v.CreatedBy,
	}

	// 敏感变量不返回值
	if !v.Sensitive {
		resp.Value = v.Value
	}

	return resp
}
