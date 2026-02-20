package models

import (
	"encoding/json"
	"time"

	"gorm.io/datatypes"
)

// ResourceType 资源类型枚举
type ResourceType string

const (
	ResourceTypeAgentPool ResourceType = "agent_pool"
	ResourceTypeWorkspace ResourceType = "workspace"
	ResourceTypeModule    ResourceType = "module"
	ResourceTypeSystem    ResourceType = "system"
	ResourceTypeTeam      ResourceType = "team"
	ResourceTypeUser      ResourceType = "user"
)

// SecretType 密文类型枚举
type SecretType string

const (
	SecretTypeHCP SecretType = "hcp" // HashiCorp Cloud Platform
)

// Secret 通用密文存储模型
type Secret struct {
	ID           uint           `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
	SecretID     string         `gorm:"column:secret_id;type:varchar(50);uniqueIndex;not null" json:"secret_id"`
	SecretType   SecretType     `gorm:"column:secret_type;type:varchar(20);not null;default:'hcp'" json:"secret_type"`
	ValueHash    string         `gorm:"column:value_hash;type:text;not null" json:"-"` // 永不序列化
	ResourceType ResourceType   `gorm:"column:resource_type;type:varchar(50);not null;index:idx_resource" json:"resource_type"`
	ResourceID   *string        `gorm:"column:resource_id;type:varchar(50);index:idx_resource" json:"resource_id,omitempty"`
	CreatedBy    *string        `gorm:"column:created_by;type:varchar(50);index" json:"created_by,omitempty"`
	UpdatedBy    *string        `gorm:"column:updated_by;type:varchar(50)" json:"updated_by,omitempty"`
	CreatedAt    time.Time      `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updated_at"`
	LastUsedAt   *time.Time     `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time     `gorm:"column:expires_at;index" json:"expires_at,omitempty"`
	IsActive     bool           `gorm:"column:is_active;default:true;index" json:"is_active"`
	Metadata     datatypes.JSON `gorm:"column:metadata;type:jsonb" json:"metadata,omitempty"`
}

// TableName 指定表名
func (Secret) TableName() string {
	return "secrets"
}

// SecretMetadata 密文元数据结构
type SecretMetadata struct {
	Key            string          `json:"key"`
	Description    string          `json:"description,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	RotationPolicy *RotationPolicy `json:"rotation_policy,omitempty"`
	AccessCount    int             `json:"access_count,omitempty"`
	LastRotationAt *time.Time      `json:"last_rotation_at,omitempty"`
}

// RotationPolicy 轮转策略
type RotationPolicy struct {
	Enabled      bool `json:"enabled"`
	IntervalDays int  `json:"interval_days"`
}

// ===== Request/Response Models =====

// CreateSecretRequest 创建密文请求
type CreateSecretRequest struct {
	Key          string     `json:"key" binding:"required,max=100"`
	Value        string     `json:"value" binding:"required"`
	SecretType   SecretType `json:"secret_type" binding:"omitempty,oneof=hcp"`
	Description  string     `json:"description"`
	Tags         []string   `json:"tags"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

// CreateSecretResponse 创建密文响应（仅此时返回明文value）
type CreateSecretResponse struct {
	SecretID     string       `json:"secret_id"`
	SecretType   SecretType   `json:"secret_type"`
	ResourceType ResourceType `json:"resource_type"`
	ResourceID   *string      `json:"resource_id,omitempty"`
	Key          string       `json:"key"`
	Value        string       `json:"value"` // 仅创建时返回
	Description  string       `json:"description,omitempty"`
	Tags         []string     `json:"tags,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
}

// SecretResponse 密文响应（不包含value）
type SecretResponse struct {
	SecretID     string       `json:"secret_id"`
	SecretType   SecretType   `json:"secret_type"`
	ResourceType ResourceType `json:"resource_type"`
	ResourceID   *string      `json:"resource_id,omitempty"`
	Key          string       `json:"key"`
	Description  string       `json:"description,omitempty"`
	Tags         []string     `json:"tags,omitempty"`
	CreatedBy    *string      `json:"created_by,omitempty"`
	UpdatedBy    *string      `json:"updated_by,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	LastUsedAt   *time.Time   `json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	IsActive     bool         `json:"is_active"`
}

// UpdateSecretRequest 更新密文请求
type UpdateSecretRequest struct {
	Value       *string  `json:"value"` // 可选：更新value
	Description *string  `json:"description"`
	Tags        []string `json:"tags"`
}

// SecretListResponse 密文列表响应
type SecretListResponse struct {
	Secrets []SecretResponse `json:"secrets"`
	Total   int              `json:"total"`
}

// ToResponse 转换为响应格式（不包含value）
func (s *Secret) ToResponse() *SecretResponse {
	var metadata SecretMetadata
	if s.Metadata != nil {
		_ = json.Unmarshal(s.Metadata, &metadata)
	}

	return &SecretResponse{
		SecretID:     s.SecretID,
		SecretType:   s.SecretType,
		ResourceType: s.ResourceType,
		ResourceID:   s.ResourceID,
		Key:          metadata.Key,
		Description:  metadata.Description,
		Tags:         metadata.Tags,
		CreatedBy:    s.CreatedBy,
		UpdatedBy:    s.UpdatedBy,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		LastUsedAt:   s.LastUsedAt,
		ExpiresAt:    s.ExpiresAt,
		IsActive:     s.IsActive,
	}
}
