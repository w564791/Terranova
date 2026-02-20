package models

import (
	"time"
)

// AgentAccessLog represents an access log entry for agent operations
type AgentAccessLog struct {
	ID             int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	AgentID        string    `gorm:"column:agent_id;type:varchar(50);not null" json:"agent_id"`
	WorkspaceID    string    `gorm:"column:workspace_id;type:varchar(50);not null" json:"workspace_id"`
	Action         string    `gorm:"column:action;type:varchar(50);not null" json:"action"`
	TaskID         *string   `gorm:"column:task_id;type:varchar(100)" json:"task_id,omitempty"`
	RequestIP      *string   `gorm:"column:request_ip;type:varchar(50)" json:"request_ip,omitempty"`
	RequestPath    *string   `gorm:"column:request_path;type:text" json:"request_path,omitempty"`
	Success        bool      `gorm:"column:success;not null;default:true" json:"success"`
	ErrorMessage   *string   `gorm:"column:error_message;type:text" json:"error_message,omitempty"`
	ResponseTimeMs *int      `gorm:"column:response_time_ms" json:"response_time_ms,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName specifies the table name
func (AgentAccessLog) TableName() string {
	return "agent_access_logs"
}

// AgentAction constants
const (
	AgentActionTaskRun   = "task.run"
	AgentActionTaskQuery = "task.query"
	AgentActionPing      = "ping"
	AgentActionRegister  = "register"
)
