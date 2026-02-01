package models

import (
	"time"
)

// PoolAllowedWorkspace represents the workspaces that a pool is allowed to access
type PoolAllowedWorkspace struct {
	ID          int        `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	PoolID      string     `gorm:"column:pool_id;type:varchar(50);not null" json:"pool_id"`
	WorkspaceID string     `gorm:"column:workspace_id;type:varchar(50);not null" json:"workspace_id"`
	Status      string     `gorm:"column:status;type:varchar(20);not null;default:'active'" json:"status"`
	AllowedBy   *string    `gorm:"column:allowed_by;type:varchar(50)" json:"allowed_by,omitempty"`
	AllowedAt   time.Time  `gorm:"column:allowed_at;not null;default:CURRENT_TIMESTAMP" json:"allowed_at"`
	RevokedBy   *string    `gorm:"column:revoked_by;type:varchar(50)" json:"revoked_by,omitempty"`
	RevokedAt   *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName specifies the table name
func (PoolAllowedWorkspace) TableName() string {
	return "pool_allowed_workspaces"
}

// AllowanceStatus constants
const (
	AllowanceStatusActive  = "active"
	AllowanceStatusRevoked = "revoked"
)

// IsActive checks if the allowance is currently active
func (p *PoolAllowedWorkspace) IsActive() bool {
	return p.Status == AllowanceStatusActive
}

// PoolAllowWorkspacesRequest represents the request to allow workspaces
type PoolAllowWorkspacesRequest struct {
	WorkspaceIDs []string `json:"workspace_ids" binding:"required,min=1"`
}

// PoolAllowedWorkspacesResponse represents the response
type PoolAllowedWorkspacesResponse struct {
	PoolID     string                 `json:"pool_id"`
	Workspaces []PoolAllowedWorkspace `json:"workspaces"`
	Total      int                    `json:"total"`
}
