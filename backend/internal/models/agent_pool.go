package models

import (
	"time"
)

// AgentPool represents an agent pool for managing agent instances
type AgentPool struct {
	PoolID                 string     `gorm:"column:pool_id;primaryKey;type:varchar(50)" json:"pool_id"`
	Name                   string     `gorm:"column:name;type:varchar(100);not null" json:"name"`
	Description            *string    `gorm:"column:description;type:text" json:"description,omitempty"`
	PoolType               string     `gorm:"column:pool_type;type:varchar(20);not null" json:"pool_type"`
	K8sConfig              *string    `gorm:"column:k8s_config;type:jsonb" json:"k8s_config,omitempty"`
	OneTimeUnfreezeUntil   *time.Time `gorm:"column:one_time_unfreeze_until" json:"one_time_unfreeze_until,omitempty"`
	OneTimeUnfreezeBy      *string    `gorm:"column:one_time_unfreeze_by;type:varchar(50)" json:"one_time_unfreeze_by,omitempty"`
	OneTimeUnfreezeAt      *time.Time `gorm:"column:one_time_unfreeze_at" json:"one_time_unfreeze_at,omitempty"`
	CreatedBy              *string    `gorm:"column:created_by;type:varchar(50)" json:"created_by,omitempty"`
	UpdatedBy              *string    `gorm:"column:updated_by;type:varchar(50)" json:"updated_by,omitempty"`
	CreatedAt              time.Time  `gorm:"column:created_at;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt              time.Time  `gorm:"column:updated_at;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName specifies the table name for AgentPool model
func (AgentPool) TableName() string {
	return "agent_pools"
}

// AgentPoolType constants
const (
	AgentPoolTypeStatic = "static"
	AgentPoolTypeK8s    = "k8s"
)

// AgentPoolStatus constants
const (
	AgentPoolStatusActive      = "active"
	AgentPoolStatusInactive    = "inactive"
	AgentPoolStatusMaintenance = "maintenance"
)

// ===== Agent Pool Request/Response Models =====

// CreateAgentPoolRequest represents the request to create an agent pool
type CreateAgentPoolRequest struct {
	Name        string  `json:"name" binding:"required,max=100"`
	Description *string `json:"description"`
	PoolType    string  `json:"pool_type" binding:"required,oneof=static k8s"`
}

// UpdateAgentPoolRequest represents the request to update an agent pool
type UpdateAgentPoolRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=100"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}

// AgentPoolListResponse represents the response for listing agent pools
type AgentPoolListResponse struct {
	Pools interface{} `json:"pools"` // Will be []PoolWithCount
	Total int         `json:"total"`
}

// AgentPoolDetailResponse represents the response for agent pool details
type AgentPoolDetailResponse struct {
	Pool   AgentPool `json:"pool"`
	Agents []Agent   `json:"agents"`
	Total  int       `json:"total"`
}
