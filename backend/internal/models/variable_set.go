package models

import (
	"fmt"
	"time"

	"iac-platform/internal/crypto"
	"iac-platform/internal/infrastructure"

	"gorm.io/gorm"
)

// VariableSet 变量集模型
type VariableSet struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	VarsetID    string    `json:"varset_id" gorm:"column:varset_id;type:varchar(30);not null;uniqueIndex"`
	Name        string    `json:"name" gorm:"type:varchar(100);not null"`
	Description string    `json:"description" gorm:"type:text"`
	Scope       string    `json:"scope" gorm:"type:varchar(20);not null;default:specific"`
	IsDeleted   bool      `json:"is_deleted" gorm:"default:false"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   *string   `json:"created_by" gorm:"type:varchar(20)"`
}

// TableName 指定表名
func (VariableSet) TableName() string {
	return "variable_sets"
}

// BeforeCreate 创建前生成 varset_id
func (v *VariableSet) BeforeCreate(tx *gorm.DB) error {
	if v.VarsetID == "" {
		varsetID, err := infrastructure.GenerateVarsetID()
		if err != nil {
			return fmt.Errorf("failed to generate varset_id: %w", err)
		}
		v.VarsetID = varsetID
	}
	return nil
}

// VarsetVariable 变量集变量模型
type VarsetVariable struct {
	ID           uint         `json:"id" gorm:"primaryKey"`
	VariableID   string       `json:"variable_id" gorm:"type:varchar(20);not null;uniqueIndex"`
	VarsetID     string       `json:"varset_id" gorm:"column:varset_id;type:varchar(30);not null;index"`
	Key          string       `json:"key" gorm:"not null;size:100"`
	Value        string       `json:"value,omitempty" gorm:"type:text"`
	VariableType VariableType `json:"variable_type" gorm:"not null;default:terraform;size:20"`
	ValueFormat  ValueFormat  `json:"value_format" gorm:"not null;default:string;size:20"`
	Sensitive    bool         `json:"sensitive" gorm:"default:false"`
	Description  string       `json:"description" gorm:"type:text"`
	IsDeleted    bool         `json:"is_deleted" gorm:"default:false"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	CreatedBy    *string      `json:"created_by" gorm:"type:varchar(20)"`
}

// TableName 指定表名
func (VarsetVariable) TableName() string {
	return "varset_variables"
}

// BeforeCreate 创建前生成 variable_id 并加密敏感变量
func (v *VarsetVariable) BeforeCreate(tx *gorm.DB) error {
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
func (v *VarsetVariable) BeforeSave(tx *gorm.DB) error {
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
func (v *VarsetVariable) AfterFind(tx *gorm.DB) error {
	if v.Sensitive && v.Value != "" && crypto.IsEncrypted(v.Value) {
		decrypted, err := crypto.DecryptValue(v.Value)
		if err != nil {
			return fmt.Errorf("failed to decrypt variable: %w", err)
		}
		v.Value = decrypted
	}
	return nil
}

// ToResponse 转换为响应格式（处理敏感变量）
func (v *VarsetVariable) ToResponse() map[string]interface{} {
	value := v.Value
	if v.Sensitive {
		value = ""
	}

	return map[string]interface{}{
		"id":            v.ID,
		"variable_id":   v.VariableID,
		"varset_id":     v.VarsetID,
		"key":           v.Key,
		"value":         value,
		"variable_type": v.VariableType,
		"value_format":  v.ValueFormat,
		"sensitive":     v.Sensitive,
		"description":   v.Description,
		"is_deleted":    v.IsDeleted,
		"created_at":    v.CreatedAt,
		"updated_at":    v.UpdatedAt,
		"created_by":    v.CreatedBy,
	}
}

// VarsetAssignment 变量集分配模型
type VarsetAssignment struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	VarsetID    string    `json:"varset_id" gorm:"column:varset_id;type:varchar(30);not null;index"`
	ScopeType   string    `json:"scope_type" gorm:"type:varchar(20);not null"`
	ProjectID   *int      `json:"project_id" gorm:"type:integer"`
	WorkspaceID *string   `json:"workspace_id" gorm:"type:varchar(50)"`
	AttachedAt  time.Time `json:"attached_at" gorm:"autoCreateTime"`
	AttachedBy  *string   `json:"attached_by" gorm:"type:varchar(20)"`
}

// TableName 指定表名
func (VarsetAssignment) TableName() string {
	return "varset_assignments"
}
