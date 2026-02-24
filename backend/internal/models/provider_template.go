package models

import "time"

// ProviderTemplate 全局Provider配置模板
type ProviderTemplate struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"type:varchar(100);not null;uniqueIndex"`
	Type         string    `json:"type" gorm:"type:varchar(50);not null;index"`
	Source       string    `json:"source" gorm:"type:varchar(200);not null"`
	Alias        string    `json:"alias" gorm:"type:varchar(50)"`
	Config       JSONB     `json:"config" gorm:"type:jsonb;not null;default:'{}'"`
	Version      string    `json:"version" gorm:"type:varchar(50)"`
	ConstraintOp string    `json:"constraint_op" gorm:"column:constraint_op;type:varchar(10)"`
	IsDefault    bool      `json:"is_default" gorm:"default:false"`
	Enabled      bool      `json:"enabled" gorm:"default:true"`
	Description  string    `json:"description" gorm:"type:text"`
	CreatedBy    *uint     `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ProviderTemplate) TableName() string {
	return "provider_templates"
}

// CreateProviderTemplateRequest 创建Provider模板请求
type CreateProviderTemplateRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Type         string                 `json:"type" binding:"required"`
	Source       string                 `json:"source" binding:"required"`
	Alias        string                 `json:"alias"`
	Config       map[string]interface{} `json:"config" binding:"required"`
	Version      string                 `json:"version"`
	ConstraintOp string                 `json:"constraint_op"`
	Enabled      bool                   `json:"enabled"`
	Description  string                 `json:"description"`
}

// UpdateProviderTemplateRequest 更新Provider模板请求
type UpdateProviderTemplateRequest struct {
	Name         *string                `json:"name"`
	Type         *string                `json:"type"`
	Source       *string                `json:"source"`
	Alias        *string                `json:"alias"`
	Config       map[string]interface{} `json:"config"`
	Version      *string                `json:"version"`
	ConstraintOp *string                `json:"constraint_op"`
	Enabled      *bool                  `json:"enabled"`
	Description  *string                `json:"description"`
}

// ProviderTemplateListResponse Provider模板列表响应
type ProviderTemplateListResponse struct {
	Items []ProviderTemplate `json:"items"`
	Total int                `json:"total"`
}
