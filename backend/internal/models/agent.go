package models

import (
	"time"
)

// Agent represents an agent instance in the system
type Agent struct {
	AgentID       string     `gorm:"column:agent_id;primaryKey;type:varchar(50)" json:"agent_id"`
	ApplicationID int        `gorm:"column:application_id;not null" json:"application_id"`
	PoolID        *string    `gorm:"column:pool_id;type:varchar(50)" json:"pool_id,omitempty"`
	Name          string     `gorm:"column:name;type:varchar(100)" json:"name"`
	TokenHash     string     `gorm:"column:token_hash;type:varchar(255);not null" json:"-"` // 不返回给客户端
	Status        string     `gorm:"column:status;type:varchar(20);default:'idle'" json:"status"`
	IPAddress     *string    `gorm:"column:ip_address;type:varchar(50)" json:"ip_address,omitempty"`
	Version       *string    `gorm:"column:version;type:varchar(50)" json:"version,omitempty"`
	LastPingAt    *time.Time `gorm:"column:last_ping_at" json:"last_ping_at,omitempty"`
	Capabilities  *string    `gorm:"column:capabilities;type:jsonb" json:"capabilities,omitempty"`
	Metadata      *string    `gorm:"column:metadata;type:jsonb" json:"metadata,omitempty"`
	RegisteredAt  time.Time  `gorm:"column:registered_at;not null;default:CURRENT_TIMESTAMP" json:"registered_at"`
	CreatedBy     *string    `gorm:"column:created_by;type:varchar(50)" json:"created_by,omitempty"`
	UpdatedBy     *string    `gorm:"column:updated_by;type:varchar(50)" json:"updated_by,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName specifies the table name for Agent model
func (Agent) TableName() string {
	return "agents"
}

// AgentStatus constants
const (
	AgentStatusIdle    = "idle"
	AgentStatusBusy    = "busy"
	AgentStatusOffline = "offline"
)

// IsOnline checks if the agent is online (last ping within 2 minutes)
func (a *Agent) IsOnline() bool {
	if a.LastPingAt == nil {
		return false
	}
	return time.Since(*a.LastPingAt) < 2*time.Minute
}

// AgentRegisterRequest represents the request body for agent registration
type AgentRegisterRequest struct {
	Name    string `json:"name" binding:"omitempty,max=100"`
	Version string `json:"version" binding:"omitempty,max=50"`
}

// AgentRegisterResponse represents the response for agent registration
type AgentRegisterResponse struct {
	AgentID      string    `json:"agent_id"`
	Status       string    `json:"status"`
	RegisteredAt time.Time `json:"registered_at"`
}

// AgentPingRequest represents the request body for agent ping
type AgentPingRequest struct {
	Status       string        `json:"status" binding:"required,oneof=idle busy"`
	CPUUsage     float64       `json:"cpu_usage"`      // CPU使用率 0-100
	MemoryUsage  float64       `json:"memory_usage"`   // 内存使用率 0-100
	RunningTasks []RunningTask `json:"running_tasks"`  // 当前运行的任务
}

// RunningTask 运行中的任务信息
type RunningTask struct {
	TaskID      uint   `json:"task_id"`
	TaskType    string `json:"task_type"`
	WorkspaceID string `json:"workspace_id"`
	StartedAt   string `json:"started_at"`
}

// AgentPingResponse represents the response for agent ping
type AgentPingResponse struct {
	Message    string    `json:"message"`
	LastPingAt time.Time `json:"last_ping_at"`
}

// Note: Agent-level authorization has been deprecated and migrated to Pool-level authorization.
// All agent authorization related types have been removed.
// Please use Pool-level authorization APIs instead.
